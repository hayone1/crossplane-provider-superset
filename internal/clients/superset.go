// Package clients turns a ProviderConfig into the configuration of the in-process Apache
// Superset Terraform provider.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1beta1 "github.com/hayone1/crossplane-provider-superset/apis/cluster/v1beta1"
	namespacedv1beta1 "github.com/hayone1/crossplane-provider-superset/apis/namespaced/v1beta1"
)

// Keys of the credentials JSON object. They are the argument names of the Terraform
// provider's configuration.
const (
	keyHost                = "host"
	keyUsername            = "username"
	keyPassword            = "password"
	keyAuthProvider        = "auth_provider"
	keyCSRFEnabled         = "csrf_enabled"
	keyValidateCredentials = "validate_credentials"
)

// Values of auth_provider that Superset's login API accepts.
const (
	authProviderDB   = "db"
	authProviderLDAP = "ldap"
)

// EnvironmentVariables are the variables that the Terraform provider reads when its
// configuration leaves an argument empty. The provider always passes every argument, so
// these variables have no effect.
var EnvironmentVariables = []string{
	"SUPERSET_HOST", "SUPERSET_USERNAME", "SUPERSET_PASSWORD", "SUPERSET_AUTH_PROVIDER",
}

const (
	errNoProviderConfig   = "no providerConfigRef provided"
	errGetProviderConfig  = "cannot get referenced ProviderConfig"
	errTrackUsage         = "cannot track ProviderConfig usage"
	errExtractCredentials = "cannot extract credentials"
)

// TerraformSetupBuilder returns a terraform.SetupFn that configures the in-process
// ("no-fork") Superset Terraform provider with the settings of the ProviderConfig that a
// managed resource references. newProvider creates the provider. One provider instance
// serves every ProviderConfig, because the Terraform provider keeps the API client of each
// configuration in its configure response, not in the instance.
func TerraformSetupBuilder(newProvider func() fwprovider.Provider) terraform.SetupFn {
	p := newProvider()
	return func(ctx context.Context, kube client.Client, mg resource.Managed) (terraform.Setup, error) {
		pc, err := resolveProviderConfig(ctx, kube, mg)
		if err != nil {
			return terraform.Setup{}, errors.Wrap(err, "cannot resolve provider config")
		}
		data, err := resource.CommonCredentialExtractor(ctx, pc.spec.Credentials.Source, kube, pc.spec.Credentials.CommonCredentialSelectors)
		if err != nil {
			return terraform.Setup{}, errors.Wrap(err, errExtractCredentials)
		}
		cfg, err := Configuration(data, pc.spec)
		if err != nil {
			return terraform.Setup{}, errors.Wrapf(err, "invalid credentials of %s", pc.name)
		}
		return terraform.Setup{
			Configuration:     cfg,
			FrameworkProvider: p,
		}, nil
	}
}

// Configuration builds the Terraform provider configuration from the credentials data and
// the ProviderConfig spec. Every argument of the Terraform provider is set, so that the
// Terraform provider never reads SUPERSET_* environment variables of the provider pod.
func Configuration(data []byte, spec *namespacedv1beta1.ProviderConfigSpec) (map[string]any, error) {
	if spec == nil {
		spec = &namespacedv1beta1.ProviderConfigSpec{}
	}
	creds, err := parseCredentials(data)
	if err != nil {
		return nil, err
	}
	cfg, err := stringSettings(creds)
	if err != nil {
		return nil, err
	}
	if err := applyHost(cfg, spec.Host); err != nil {
		return nil, err
	}
	if cfg[keyUsername] == "" || cfg[keyPassword] == "" {
		return nil, errors.New("the credentials need both username and password")
	}
	if cfg[keyAuthProvider], err = authProvider(cfg[keyAuthProvider].(string)); err != nil {
		return nil, err
	}
	if cfg[keyCSRFEnabled], err = boolValue(creds, keyCSRFEnabled, true); err != nil {
		return nil, err
	}
	// The Terraform provider logs in when it is configured, so wrong credentials fail the
	// connection with Superset's answer instead of the first request of a reconcile.
	cfg[keyValidateCredentials] = true
	return cfg, nil
}

// stringSettings returns the string settings of the credentials, with an empty string for each
// one that the credentials leave out. The password is used exactly as given; the other settings
// are trimmed.
func stringSettings(creds map[string]any) (map[string]any, error) {
	cfg := map[string]any{}
	for _, k := range []string{keyHost, keyUsername, keyPassword, keyAuthProvider} {
		s, err := stringValue(creds, k)
		if err != nil {
			return nil, err
		}
		if k != keyPassword {
			s = strings.TrimSpace(s)
		}
		cfg[k] = s
	}
	return cfg, nil
}

// authProvider returns the authentication provider of the login: db unless ldap is set.
func authProvider(value string) (string, error) {
	switch p := strings.ToLower(value); p {
	case "":
		return authProviderDB, nil
	case authProviderDB, authProviderLDAP:
		return p, nil
	default:
		return "", errors.Errorf("the credentials key auth_provider must be %s or %s, not %q", authProviderDB, authProviderLDAP, p)
	}
}

// applyHost adds the Superset URL from the ProviderConfig spec. A spec value that differs
// from the credentials is an error. The URL is used without a trailing slash, because the
// Terraform provider appends paths such as /api/v1/chart/ to it.
func applyHost(cfg map[string]any, specHost string) error {
	host := cfg[keyHost].(string)
	if specHost = strings.TrimSpace(specHost); specHost != "" {
		if host != "" && strings.TrimRight(host, "/") != strings.TrimRight(specHost, "/") {
			return errors.New("the ProviderConfig spec and the credentials set different values for host: set it in only one place")
		}
		host = specHost
	}
	host = strings.TrimRight(host, "/")
	switch {
	case host == "":
		return errors.New("the Superset URL is missing: set spec.host or the credentials key host")
	case !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://"):
		return errors.Errorf("the Superset URL %q must start with http:// or https://", host)
	case strings.ContainsAny(host, "?#"):
		return errors.Errorf("the Superset URL %q must not have a query or a fragment", host)
	}
	cfg[keyHost] = host
	return nil
}

// parseCredentials reads the credentials JSON object and rejects unknown keys, so that a
// typing mistake such as "user" does not go unnoticed.
func parseCredentials(data []byte) (map[string]any, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("the credentials are empty")
	}
	if data[0] != '{' {
		return nil, errors.New(`the credentials must be a JSON object, for example {"username": "admin", "password": "..."}`)
	}
	creds := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&creds); err != nil {
		// Do not include the data in the message: it holds secrets.
		return nil, errors.New("the credentials are not a valid JSON object")
	}
	known := map[string]bool{keyHost: true, keyUsername: true, keyPassword: true, keyAuthProvider: true, keyCSRFEnabled: true}
	var unknown []string
	for k := range creds {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		allowed := make([]string, 0, len(known))
		for k := range known {
			allowed = append(allowed, k)
		}
		sort.Strings(allowed)
		return nil, errors.Errorf("the credentials contain unknown keys %s: the allowed keys are %s", strings.Join(unknown, ", "), strings.Join(allowed, ", "))
	}
	return creds, nil
}

func stringValue(creds map[string]any, key string) (string, error) {
	switch v := creds[key].(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	default:
		return "", errors.Errorf("the credentials key %q must be a string", key)
	}
}

func boolValue(creds map[string]any, key string, def bool) (bool, error) {
	switch v := creds[key].(type) {
	case nil:
		return def, nil
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, errors.Errorf("the credentials key %s must be true or false", key)
		}
		return b, nil
	default:
		return false, errors.Errorf("the credentials key %s must be true or false", key)
	}
}

// SetEnvironmentVariables returns the Terraform provider variables that are set in the
// provider's environment. They have no effect, and the provider logs a message about them.
func SetEnvironmentVariables() []string {
	var set []string
	for _, v := range EnvironmentVariables {
		if _, ok := os.LookupEnv(v); ok {
			set = append(set, v)
		}
	}
	return set
}

// providerConfig is a resolved ProviderConfig or ClusterProviderConfig.
type providerConfig struct {
	name string
	spec *namespacedv1beta1.ProviderConfigSpec
}

func resolveProviderConfig(ctx context.Context, crClient client.Client, mg resource.Managed) (*providerConfig, error) {
	switch managed := mg.(type) {
	case resource.LegacyManaged: //nolint:staticcheck // the cluster-scoped API is still supported
		return resolveLegacy(ctx, crClient, managed)
	case resource.ModernManaged:
		return resolveModern(ctx, crClient, managed)
	default:
		return nil, errors.New("resource is not a managed resource")
	}
}

func resolveLegacy(ctx context.Context, kube client.Client, mg resource.LegacyManaged) (*providerConfig, error) { //nolint:staticcheck // the cluster-scoped API is still supported
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}
	pc := &clusterv1beta1.ProviderConfig{}
	if err := kube.Get(ctx, types.NamespacedName{Name: configRef.Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	t := resource.NewLegacyProviderConfigUsageTracker(kube, &clusterv1beta1.ProviderConfigUsage{})
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}
	spec, err := toSharedPCSpec(pc)
	if err != nil {
		return nil, err
	}
	return &providerConfig{name: fmt.Sprintf("ProviderConfig %s", pc.Name), spec: spec}, nil
}

func resolveModern(ctx context.Context, crClient client.Client, mg resource.ModernManaged) (*providerConfig, error) {
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}
	pcRuntimeObj, err := crClient.Scheme().New(namespacedv1beta1.SchemeGroupVersion.WithKind(configRef.Kind))
	if err != nil {
		return nil, errors.Wrapf(err, "unknown provider config kind %q", configRef.Kind)
	}
	pcObj, ok := pcRuntimeObj.(client.Object)
	if !ok {
		// This indicates a programming error: the types are not generated properly.
		return nil, errors.Errorf("%s is not a Kubernetes object", configRef.Kind)
	}
	key := types.NamespacedName{Name: configRef.Name, Namespace: mg.GetNamespace()}
	if _, cluster := pcObj.(*namespacedv1beta1.ClusterProviderConfig); cluster {
		key.Namespace = ""
	}
	if err := crClient.Get(ctx, key, pcObj); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	var pc providerConfig
	switch obj := pcObj.(type) {
	case *namespacedv1beta1.ProviderConfig:
		spec, err := namespacedSpec(obj, mg.GetNamespace())
		if err != nil {
			return nil, err
		}
		pc = providerConfig{name: fmt.Sprintf("ProviderConfig %s/%s", obj.Namespace, obj.Name), spec: spec}
	case *namespacedv1beta1.ClusterProviderConfig:
		pc = providerConfig{name: fmt.Sprintf("ClusterProviderConfig %s", obj.Name), spec: obj.Spec.DeepCopy()}
	default:
		return nil, errors.New("unknown provider config type")
	}
	t := resource.NewProviderConfigUsageTracker(crClient, &namespacedv1beta1.ProviderConfigUsage{})
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}
	return &pc, nil
}

// namespacedSpec returns the spec of a namespaced ProviderConfig. A namespaced
// ProviderConfig may only read a Secret in its own namespace. The Environment and Filesystem
// sources would let users of one namespace make the provider read its own environment or
// files, so they need a ClusterProviderConfig.
func namespacedSpec(pc *namespacedv1beta1.ProviderConfig, namespace string) (*namespacedv1beta1.ProviderConfigSpec, error) {
	spec := pc.Spec.DeepCopy()
	if spec.Credentials.Source != xpv2.CredentialsSourceSecret {
		return nil, errors.Errorf("ProviderConfig %s/%s: a namespaced ProviderConfig supports only the Secret credentials source; use a ClusterProviderConfig for the %s source", pc.Namespace, pc.Name, spec.Credentials.Source)
	}
	if spec.Credentials.SecretRef != nil {
		spec.Credentials.SecretRef.Namespace = namespace
	}
	return spec, nil
}

func toSharedPCSpec(pc *clusterv1beta1.ProviderConfig) (*namespacedv1beta1.ProviderConfigSpec, error) {
	data, err := json.Marshal(pc.Spec)
	if err != nil {
		return nil, err
	}
	var spec namespacedv1beta1.ProviderConfigSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
