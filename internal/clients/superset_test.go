package clients

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	namespacedv1beta1 "github.com/hayone1/crossplane-provider-superset/apis/namespaced/v1beta1"
)

func TestConfiguration(t *testing.T) {
	full := func(overrides map[string]any) map[string]any {
		cfg := map[string]any{
			keyHost: "https://superset.example.com", keyUsername: "admin", keyPassword: " secret ",
			keyAuthProvider: authProviderDB, keyCSRFEnabled: true, keyValidateCredentials: true,
		}
		for k, v := range overrides {
			cfg[k] = v
		}
		return cfg
	}
	cases := map[string]struct {
		creds   string
		spec    *namespacedv1beta1.ProviderConfigSpec
		want    map[string]any
		wantErr string
	}{
		"minimal": {
			creds: `{"host": "https://superset.example.com/", "username": " admin ", "password": " secret "}`,
			want:  full(nil),
		},
		"host from the spec": {
			creds: `{"username": "admin", "password": " secret "}`,
			spec:  &namespacedv1beta1.ProviderConfigSpec{Host: "https://superset.example.com"},
			want:  full(nil),
		},
		"same host in both places": {
			creds: `{"host": "https://superset.example.com", "username": "admin", "password": " secret "}`,
			spec:  &namespacedv1beta1.ProviderConfigSpec{Host: "https://superset.example.com/"},
			want:  full(nil),
		},
		"host with a path": {
			creds: `{"host": "https://example.com/superset/", "username": "admin", "password": " secret "}`,
			want:  full(map[string]any{keyHost: "https://example.com/superset"}),
		},
		"ldap and no csrf": {
			creds: `{"host": "https://superset.example.com", "username": "admin", "password": " secret ", "auth_provider": "LDAP", "csrf_enabled": "false"}`,
			want:  full(map[string]any{keyAuthProvider: authProviderLDAP, keyCSRFEnabled: false}),
		},
		"different hosts": {
			creds:   `{"host": "https://a.example.com", "username": "admin", "password": "x"}`,
			spec:    &namespacedv1beta1.ProviderConfigSpec{Host: "https://b.example.com"},
			wantErr: "different values for host",
		},
		"no host":             {creds: `{"username": "admin", "password": "x"}`, wantErr: "URL is missing"},
		"host without http":   {creds: `{"host": "superset.example.com", "username": "admin", "password": "x"}`, wantErr: "must start with http"},
		"host with a query":   {creds: `{"host": "https://superset.example.com?a=b", "username": "admin", "password": "x"}`, wantErr: "query or a fragment"},
		"no password":         {creds: `{"host": "https://superset.example.com", "username": "admin"}`, wantErr: "both username and password"},
		"no username":         {creds: `{"host": "https://superset.example.com", "password": "x"}`, wantErr: "both username and password"},
		"unknown key":         {creds: `{"host": "https://s.example.com", "username": "admin", "password": "x", "user": "y"}`, wantErr: "unknown keys user"},
		"unknown provider":    {creds: `{"host": "https://s.example.com", "username": "admin", "password": "x", "auth_provider": "oauth"}`, wantErr: "must be db or ldap"},
		"bad csrf":            {creds: `{"host": "https://s.example.com", "username": "admin", "password": "x", "csrf_enabled": "maybe"}`, wantErr: "true or false"},
		"number as string":    {creds: `{"host": "https://s.example.com", "username": 1, "password": "x"}`, wantErr: "must be a string"},
		"not json":            {creds: `admin:secret`, wantErr: "must be a JSON object"},
		"broken json":         {creds: `{"username": "admin", "password": "se`, wantErr: "not a valid JSON object"},
		"empty":               {creds: ``, wantErr: "empty"},
		"bare token is wrong": {creds: `"secret"`, wantErr: "must be a JSON object"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Configuration([]byte(tc.creds), tc.spec)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one that contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("configuration (-want +got):\n%s", diff)
			}
		})
	}
}

// Error messages about the credentials must never contain the credentials.
func TestConfigurationErrorsHideSecrets(t *testing.T) {
	for _, creds := range []string{
		`{"host": "ftp://admin:TOPSECRET@s.example.com", "username": "admin", "password": "x"}`,
		`{"host": "https://admin:TOPSECRET@s.example.com?a=b", "username": "admin", "password": "x"}`,
		`{"host": "https://s.example.com", "username": "admin", "password": "TOPSECRET", "user": "y"}`,
		`{"username": "admin", "password": "TOPSECRET`,
		`{"host": "https://s.example.com", "password": "TOPSECRET"}`,
	} {
		_, err := Configuration([]byte(creds), nil)
		if err == nil {
			t.Fatalf("no error for %s", creds)
		}
		if strings.Contains(err.Error(), "TOPSECRET") {
			t.Errorf("the error shows the password: %v", err)
		}
	}
}

// Every argument of the Terraform provider must be set, so that the SUPERSET_* variables of
// the provider pod never apply.
func TestConfigurationSetsEveryArgument(t *testing.T) {
	cfg, err := Configuration([]byte(`{"host": "https://s.example.com", "username": "admin", "password": "x"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{keyHost, keyUsername, keyPassword, keyAuthProvider, keyCSRFEnabled, keyValidateCredentials} {
		v, ok := cfg[k]
		if !ok || v == "" {
			t.Errorf("%s is not set", k)
		}
	}
}
