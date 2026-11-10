package clients

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	schemav2 "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	superset "github.com/platacard/terraform-provider-superset/provider"

	providerconfig "github.com/hayone1/crossplane-provider-superset/config"
)

// The configuration must set every argument of the Terraform provider and nothing else, so
// that the SUPERSET_* variables of the pod never apply and Configure never fails on an unknown
// argument.
func TestConfigurationMatchesProviderSchema(t *testing.T) {
	var resp fwprovider.SchemaResponse
	superset.New("test")().Schema(context.Background(), fwprovider.SchemaRequest{}, &resp)
	want := make([]string, 0, len(resp.Schema.Attributes))
	for name := range resp.Schema.Attributes {
		want = append(want, name)
	}
	cfg, err := Configuration([]byte(`{"host": "https://s.example.com", "username": "admin", "password": "x"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(cfg))
	for name := range cfg {
		got = append(got, name)
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the configuration sets %v, the Terraform provider has %v", got, want)
	}
}

// wrappedResource returns the wrapped resource of a type and its schema.
func wrappedResource(t *testing.T, typeName string) (resource.Resource, resource.SchemaResponse) {
	t.Helper()
	ctx := context.Background()
	p := WithConfigChecks(superset.New("test"))()
	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "superset"}, &meta)
		if meta.TypeName != typeName {
			continue
		}
		var s resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &s)
		return r, s
	}
	t.Fatalf("no resource %s", typeName)
	return nil, resource.SchemaResponse{}
}

// config builds a configuration with the given attribute values; the others are null.
func config(t *testing.T, s resource.SchemaResponse, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	typ := s.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	all := map[string]tftypes.Value{}
	for name, at := range typ.AttributeTypes {
		if v, ok := values[name]; ok {
			all[name] = v
		} else {
			all[name] = tftypes.NewValue(at, nil)
		}
	}
	return tfsdk.Config{Schema: s.Schema, Raw: tftypes.NewValue(typ, all)}
}

// The wrapper must reject a configuration that Terraform would reject when it plans, before
// the resource calls Superset. The wrapped resources are not configured, so a Create that got
// through would fail with a different error.
func TestConfigCheck_rejectsInvalidConfigurationBeforeWrites(t *testing.T) {
	cases := map[string]struct {
		typeName string
		values   func(s resource.SchemaResponse) map[string]tftypes.Value
		want     string
	}{
		"attribute validator": {
			typeName: "superset_row_level_security",
			values: func(resource.SchemaResponse) map[string]tftypes.Value {
				return map[string]tftypes.Value{
					"name":        tftypes.NewValue(tftypes.String, "rule"),
					"clause":      tftypes.NewValue(tftypes.String, "1 = 1"),
					"filter_type": tftypes.NewValue(tftypes.String, "Sometimes"),
				}
			},
			want: "value must be one of",
		},
		"resource check": {
			typeName: "superset_dashboard_import",
			values: func(resource.SchemaResponse) map[string]tftypes.Value {
				return map[string]tftypes.Value{}
			},
			want: "Set either source",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, s := wrappedResource(t, tc.typeName)
			cfg := config(t, s, tc.values(s))
			plan := tfsdk.Plan{Schema: s.Schema, Raw: cfg.Raw}
			var create resource.CreateResponse
			r.Create(context.Background(), resource.CreateRequest{Config: cfg, Plan: plan}, &create)
			if !create.Diagnostics.HasError() || !strings.Contains(diagnosticsString(create.Diagnostics), tc.want) {
				t.Errorf("Create: %v, want an error with %q", create.Diagnostics, tc.want)
			}
			var update resource.UpdateResponse
			r.Update(context.Background(), resource.UpdateRequest{Config: cfg, Plan: plan, State: tfsdk.State{Schema: s.Schema, Raw: cfg.Raw}}, &update)
			if !update.Diagnostics.HasError() || !strings.Contains(diagnosticsString(update.Diagnostics), tc.want) {
				t.Errorf("Update: %v, want an error with %q", update.Diagnostics, tc.want)
			}
		})
	}
}

func diagnosticsString(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Summary() + ": " + d.Detail() + "\n")
	}
	return b.String()
}

// The wrapper forwards the optional interfaces that the framework and Upjet use.
func TestConfigCheck_forwardsOptionalInterfaces(t *testing.T) {
	r, _ := wrappedResource(t, "superset_dashboard_import")
	if _, ok := r.(resource.ResourceWithModifyPlan); !ok {
		t.Error("ModifyPlan is not forwarded")
	}
	if _, ok := r.(resource.ResourceWithConfigure); !ok {
		t.Error("Configure is not forwarded")
	}
	if _, ok := r.(resource.ResourceWithValidateConfig); !ok {
		t.Error("ValidateConfig is not forwarded")
	}
}

// A required list that the spec sets to [] reaches the provider as null, because the generated
// types leave out empty lists. The wrapper must treat it as the empty list that the spec says,
// in the configuration that it validates and in the configuration and plan of the resource.
func TestConfigCheck_requiredEmptyListIsNotMissing(t *testing.T) {
	r, s := wrappedResource(t, "superset_dashboard_embedding")
	g := r.(*configCheck)
	cfg := config(t, s, map[string]tftypes.Value{"dashboard_id": tftypes.NewValue(tftypes.Number, 3)})
	plan := tfsdk.Plan{Schema: s.Schema, Raw: cfg.Raw}

	var diags diag.Diagnostics
	g.emptyRequiredCollections(context.Background(), &cfg.Raw, &plan.Raw, &diags)
	if diags.HasError() {
		t.Fatal(diagnosticsString(diags))
	}
	for name, raw := range map[string]tftypes.Value{"config": cfg.Raw, "plan": plan.Raw} {
		var attrs map[string]tftypes.Value
		if err := raw.As(&attrs); err != nil {
			t.Fatal(err)
		}
		var domains []tftypes.Value
		if err := attrs["allowed_domains"].As(&domains); err != nil || attrs["allowed_domains"].IsNull() || len(domains) != 0 {
			t.Errorf("%s: allowed_domains is %s, want an empty list", name, attrs["allowed_domains"])
		}
		if !attrs["uuid"].IsNull() {
			t.Errorf("%s: the optional uuid changed to %s", name, attrs["uuid"])
		}
	}
	g.validator.validate(context.Background(), g.typeName, cfg, &diags)
	if diags.HasError() {
		t.Errorf("the validation rejects an empty list: %s", diagnosticsString(diags))
	}
}

// A required list that a reference can fill, such as the roles of a User, stays null when the
// spec leaves it out. The validation then rejects the spec, instead of an update that removes
// every role of the user.
func TestConfigCheck_referencedRequiredListStaysNull(t *testing.T) {
	r, s := wrappedResource(t, "superset_user")
	g := r.(*configCheck)
	cfg := config(t, s, map[string]tftypes.Value{
		"username": tftypes.NewValue(tftypes.String, "alice"),
		"email":    tftypes.NewValue(tftypes.String, "alice@example.com"),
	})
	plan := tfsdk.Plan{Schema: s.Schema, Raw: cfg.Raw}
	var diags diag.Diagnostics
	g.emptyRequiredCollections(context.Background(), &cfg.Raw, &plan.Raw, &diags)
	var attrs map[string]tftypes.Value
	if err := cfg.Raw.As(&attrs); err != nil {
		t.Fatal(err)
	}
	if !attrs["roles"].IsNull() {
		t.Fatalf("roles became %s, want null", attrs["roles"])
	}
	g.validator.validate(context.Background(), g.typeName, cfg, &diags)
	if !diags.HasError() || !strings.Contains(diagnosticsString(diags), "roles") {
		t.Errorf("a User without roles must fail the validation, got %s", diagnosticsString(diags))
	}
}

// EmptiableCollections must list exactly the required top-level collections that no reference
// fills, so that a new resource of the Terraform provider is not forgotten. The files of an
// import are required only in the CRD; the Terraform provider itself reports an empty bundle.
func TestEmptiableCollectionsMatchTheSchema(t *testing.T) {
	want := map[string][]string{}
	for name, r := range providerconfig.GetProvider().Resources {
		if strings.HasSuffix(name, "_import") {
			continue
		}
		for attr, s := range r.TerraformResource.Schema {
			if !s.Required || (s.Type != schemav2.TypeList && s.Type != schemav2.TypeSet && s.Type != schemav2.TypeMap) {
				continue
			}
			if _, referenced := r.References[attr]; referenced {
				continue
			}
			want[name] = append(want[name], attr)
		}
	}
	for name := range want {
		sort.Strings(want[name])
	}
	if !cmp.Equal(want, EmptiableCollections) {
		t.Errorf("EmptiableCollections = %v, want %v", EmptiableCollections, want)
	}
}
