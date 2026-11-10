package clients

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"golang.org/x/sync/singleflight"
)

// configureTTL is how long the provider reuses the API client of one configuration. Upjet
// configures the Terraform provider for every reconcile of every managed resource, and the
// Terraform provider signs in to Superset each time. Many reconciles at once, for example
// after the provider starts, then exceed the rate limit of Superset's login endpoint
// (HTTP 429 "Too Many Requests"). With the cache, the provider signs in once per configuration
// in this time. The client signs in again by itself when Superset rejects an expired token. The
// time is short, so that the caches inside the client (the permissions of view menus and the
// list of database connections) follow changes in Superset.
const configureTTL = 10 * time.Minute

// configureCache keeps the result of the Terraform provider's Configure for each configuration.
type configureCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[[sha256.Size]byte]configureEntry
	// group makes concurrent configures of one configuration sign in once.
	group singleflight.Group
}

type configureEntry struct {
	resourceData   any
	dataSourceData any
	expires        time.Time
}

func newConfigureCache() *configureCache {
	return &configureCache{now: time.Now, entries: map[[sha256.Size]byte]configureEntry{}}
}

// configure answers from the cache when it has an entry for the configuration, and otherwise
// calls configure and caches a result without errors. The key is a hash of the whole
// configuration, so a changed password or URL gets a new client at once. Concurrent calls for one
// configuration that is not cached wait for a single configure.
func (c *configureCache) configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse, configure func(context.Context, fwprovider.ConfigureRequest, *fwprovider.ConfigureResponse)) {
	key, err := configurationKey(req)
	if err != nil {
		// Without a key the configuration is not cached.
		configure(ctx, req, resp)
		return
	}
	if entry, ok := c.lookup(key); ok {
		resp.ResourceData, resp.DataSourceData = entry.resourceData, entry.dataSourceData
		return
	}

	v, _, _ := c.group.Do(string(key[:]), func() (any, error) {
		if entry, ok := c.lookup(key); ok {
			return &fwprovider.ConfigureResponse{ResourceData: entry.resourceData, DataSourceData: entry.dataSourceData}, nil
		}
		var inner fwprovider.ConfigureResponse
		configure(ctx, req, &inner)
		if !inner.Diagnostics.HasError() && inner.ResourceData != nil {
			c.mu.Lock()
			c.entries[key] = configureEntry{resourceData: inner.ResourceData, dataSourceData: inner.DataSourceData, expires: c.now().Add(configureTTL)}
			c.mu.Unlock()
		}
		return &inner, nil
	})
	result := v.(*fwprovider.ConfigureResponse) //nolint:forcetypeassert // the function above always returns this type
	resp.Diagnostics.Append(result.Diagnostics...)
	resp.ResourceData, resp.DataSourceData = result.ResourceData, result.DataSourceData
}

// lookup returns the entry of a key unless it has expired, and drops expired entries, so that old
// passwords do not stay in memory.
func (c *configureCache) lookup(key [sha256.Size]byte) (configureEntry, bool) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if ok && now.Before(entry.expires) {
		return entry, true
	}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	return configureEntry{}, false
}

// configurationKey hashes the configuration in its plugin protocol encoding, which holds every
// value without ambiguity.
func configurationKey(req fwprovider.ConfigureRequest) ([sha256.Size]byte, error) {
	dv, err := tfprotov6.NewDynamicValue(req.Config.Raw.Type(), req.Config.Raw)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(dv.MsgPack), nil
}
