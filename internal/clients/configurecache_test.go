package clients

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func configureRequest(password string) fwprovider.ConfigureRequest {
	typ := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "password": tftypes.String}}
	return fwprovider.ConfigureRequest{Config: tfsdk.Config{Raw: tftypes.NewValue(typ, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "https://superset.example.com"),
		"password": tftypes.NewValue(tftypes.String, password),
	})}}
}

// The provider must sign in once per configuration, not for every reconcile, and must sign in
// again when the configuration changes or the cached client is old.
func TestConfigureCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := newConfigureCache()
	cache.now = func() time.Time { return now }
	calls := 0
	configure := func(_ context.Context, _ fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
		calls++
		resp.ResourceData = calls
		resp.DataSourceData = calls
	}
	run := func(password string) any {
		var resp fwprovider.ConfigureResponse
		cache.configure(context.Background(), configureRequest(password), &resp, configure)
		return resp.ResourceData
	}

	if got := run("a"); got != 1 || calls != 1 {
		t.Fatalf("first configure: data %v, %d calls", got, calls)
	}
	if got := run("a"); got != 1 || calls != 1 {
		t.Fatalf("same configuration: data %v, %d calls; want the cached client", got, calls)
	}
	if got := run("b"); got != 2 || calls != 2 {
		t.Fatalf("new password: data %v, %d calls; want a new client", got, calls)
	}
	now = now.Add(configureTTL)
	if got := run("a"); got != 3 || calls != 3 {
		t.Fatalf("after the TTL: data %v, %d calls; want a new client", got, calls)
	}
	if len(cache.entries) != 1 {
		t.Errorf("the cache holds %d entries, want only the fresh one", len(cache.entries))
	}
}

// A failed Configure, for example a rejected sign-in, must not be cached.
func TestConfigureCacheSkipsErrors(t *testing.T) {
	cache := newConfigureCache()
	calls := 0
	failing := func(_ context.Context, _ fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
		calls++
		resp.Diagnostics = diag.Diagnostics{diag.NewErrorDiagnostic("Unable to Create Superset API Client", "429")}
	}
	for i := 0; i < 2; i++ {
		var resp fwprovider.ConfigureResponse
		cache.configure(context.Background(), configureRequest("a"), &resp, failing)
		if !resp.Diagnostics.HasError() {
			t.Fatal("the error was lost")
		}
	}
	if calls != 2 {
		t.Errorf("Configure ran %d times, want 2: errors must not be cached", calls)
	}
}

// Concurrent configures of one configuration must sign in once, for example when many managed
// resources are checked right after the provider starts.
func TestConfigureCacheSignsInOnceForConcurrentCalls(t *testing.T) {
	cache := newConfigureCache()
	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})
	slow := func(_ context.Context, _ fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		resp.ResourceData = "client"
	}
	var wg sync.WaitGroup
	results := make(chan any, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var resp fwprovider.ConfigureResponse
			cache.configure(context.Background(), configureRequest("a"), &resp, slow)
			results <- resp.ResourceData
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)
	for data := range results {
		if data != "client" {
			t.Errorf("a caller got %v", data)
		}
	}
	if calls != 1 {
		t.Errorf("Configure ran %d times, want 1", calls)
	}
}

// Two configurations must never share a key, even when their values contain the characters that
// a printed form of the configuration uses as separators.
func TestConfigurationKeyIsUnambiguous(t *testing.T) {
	typ := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"password": tftypes.String, "username": tftypes.String}}
	request := func(user, password string) fwprovider.ConfigureRequest {
		return fwprovider.ConfigureRequest{Config: tfsdk.Config{Raw: tftypes.NewValue(typ, map[string]tftypes.Value{
			"username": tftypes.NewValue(tftypes.String, user),
			"password": tftypes.NewValue(tftypes.String, password),
		})}}
	}
	a, err := configurationKey(request("c", `a">, "username":tftypes.String<"b`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := configurationKey(request(`b">, "username":tftypes.String<"c`, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two different configurations have the same key")
	}
	again, _ := configurationKey(request("c", `a">, "username":tftypes.String<"b`))
	if a != again {
		t.Error("the key of a configuration is not stable")
	}
}
