package clients

import (
	"context"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// WithConfigChecks wraps the Terraform provider so that each resource validates its
// configuration before Create and Update, as Terraform does when it plans. Upjet never calls
// ValidateResourceConfig, so without the wrapper a spec that breaks a rule of the Terraform
// schema, such as an import with both source and files, or a report_format that Superset does
// not know, would reach Superset. The checks run only before writes, so a managed resource
// with an invalid spec can still be observed and deleted.
//
// The validation goes through the plugin protocol (ValidateResourceConfig) of a separate,
// never configured server of the same provider, so it runs everything that Terraform runs:
// the validators of each attribute and the resource's own checks.
//
// The wrapper also reuses the API client of a configuration for a while (see configureTTL), so
// that the provider does not sign in to Superset for every reconcile.
func WithConfigChecks(newProvider func() fwprovider.Provider) func() fwprovider.Provider {
	return func() fwprovider.Provider {
		v := &configValidator{newProvider: newProvider}
		return configCheckProvider{Provider: newProvider(), validator: v, clients: newConfigureCache()}
	}
}

// configValidator validates resource configurations through the plugin protocol.
type configValidator struct {
	newProvider func() fwprovider.Provider
	once        sync.Once
	server      tfprotov6.ProviderServer
}

func (v *configValidator) validate(ctx context.Context, typeName string, cfg tfsdk.Config, diags *diag.Diagnostics) {
	v.once.Do(func() {
		v.server = providerserver.NewProtocol6(v.newProvider())()
	})
	value, err := tfprotov6.NewDynamicValue(cfg.Raw.Type(), cfg.Raw)
	if err != nil {
		diags.AddError("Unable to Validate the Configuration", err.Error())
		return
	}
	resp, err := v.server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: typeName, Config: &value})
	if err != nil {
		diags.AddError("Unable to Validate the Configuration", err.Error())
		return
	}
	for _, d := range resp.Diagnostics {
		if d == nil {
			continue
		}
		summary, detail := d.Summary, d.Detail
		if d.Attribute != nil {
			detail = d.Attribute.String() + ": " + detail
		}
		switch d.Severity {
		case tfprotov6.DiagnosticSeverityError:
			diags.AddError(summary, detail)
		case tfprotov6.DiagnosticSeverityWarning:
			diags.AddWarning(summary, detail)
		case tfprotov6.DiagnosticSeverityInvalid:
			// The framework never sends a diagnostic without a severity.
		}
	}
}

// configCheckProvider forwards everything to the Terraform provider and wraps its resources.
// terraform-provider-superset implements no optional provider interfaces that Upjet uses, so
// embedding the interface hides nothing.
type configCheckProvider struct {
	fwprovider.Provider
	validator *configValidator
	clients   *configureCache
}

// Configure reuses the API client of an earlier Configure with the same configuration.
func (p configCheckProvider) Configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
	p.clients.configure(ctx, req, resp, p.Provider.Configure)
}

func (p configCheckProvider) Resources(ctx context.Context) []func() resource.Resource {
	// The framework calls Metadata on one instance of each resource to learn its type name,
	// and creates new instances for Create and Update. The type name is therefore resolved
	// here, once, for the validation request.
	var pm fwprovider.MetadataResponse
	p.Metadata(ctx, fwprovider.MetadataRequest{}, &pm)
	inner := p.Provider.Resources(ctx)
	out := make([]func() resource.Resource, 0, len(inner))
	for _, newResource := range inner {
		var rm resource.MetadataResponse
		newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: pm.TypeName}, &rm)
		typeName := rm.TypeName
		out = append(out, func() resource.Resource {
			return &configCheck{inner: newResource(), validator: p.validator, typeName: typeName}
		})
	}
	return out
}

// configCheck forwards every call to the wrapped resource, and validates the configuration
// before Create and Update. It implements each optional interface that the resources of
// terraform-provider-superset use. A method whose interface the wrapped resource does not
// implement behaves as the framework does when the interface is missing.
type configCheck struct {
	inner     resource.Resource
	validator *configValidator
	typeName  string
}

var (
	_ resource.ResourceWithConfigure        = (*configCheck)(nil)
	_ resource.ResourceWithModifyPlan       = (*configCheck)(nil)
	_ resource.ResourceWithImportState      = (*configCheck)(nil)
	_ resource.ResourceWithValidateConfig   = (*configCheck)(nil)
	_ resource.ResourceWithConfigValidators = (*configCheck)(nil)
)

func (g *configCheck) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	g.inner.Metadata(ctx, req, resp)
}

func (g *configCheck) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	g.inner.Schema(ctx, req, resp)
}

func (g *configCheck) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if g.emptyRequiredCollections(ctx, &req.Config.Raw, &req.Plan.Raw, &resp.Diagnostics); resp.Diagnostics.HasError() {
		return
	}
	if g.validator.validate(ctx, g.typeName, req.Config, &resp.Diagnostics); resp.Diagnostics.HasError() {
		return
	}
	g.inner.Create(ctx, req, resp)
}

func (g *configCheck) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	g.inner.Read(ctx, req, resp)
}

func (g *configCheck) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if g.emptyRequiredCollections(ctx, &req.Config.Raw, &req.Plan.Raw, &resp.Diagnostics); resp.Diagnostics.HasError() {
		return
	}
	if g.validator.validate(ctx, g.typeName, req.Config, &resp.Diagnostics); resp.Diagnostics.HasError() {
		return
	}
	g.inner.Update(ctx, req, resp)
}

// EmptiableCollections are the required top-level lists, sets and maps that may be empty, by
// Terraform resource name. The generated Go types leave out an empty list (omitempty), so a spec
// with "allowedDomains: []" reaches the Terraform provider as null, which the framework rejects
// as a missing required attribute. The CRDs require these fields, so a null value can only come
// from an empty list. Required collections that a reference can fill, such as the roles of a
// User, are not in the list: the CRD lets you leave them out, and then the Terraform provider must
// reject the null value instead of removing every role.
var EmptiableCollections = map[string][]string{
	"superset_dashboard_embedding": {"allowed_domains"},
	"superset_report":              {"recipients"},
	"superset_role_permissions":    {"resource_permissions"},
}

// emptyRequiredCollections replaces null values of the EmptiableCollections of the resource with
// empty ones, in the configuration and in the plan.
func (g *configCheck) emptyRequiredCollections(_ context.Context, cfg, plan *tftypes.Value, diags *diag.Diagnostics) {
	names := map[string]bool{}
	for _, name := range EmptiableCollections[g.typeName] {
		names[name] = true
	}
	if len(names) == 0 {
		return
	}
	fill := func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		if v.IsNull() && isTopLevel(p, names) {
			return emptyCollection(v.Type()), nil
		}
		return v, nil
	}
	for _, raw := range []*tftypes.Value{cfg, plan} {
		if raw.IsNull() || !raw.IsKnown() {
			continue
		}
		out, err := tftypes.Transform(*raw, fill)
		if err != nil {
			diags.AddError("Unable to Prepare the Configuration", err.Error())
			return
		}
		*raw = out
	}
}

// isTopLevel reports whether a path is a top-level attribute with one of the names.
func isTopLevel(p *tftypes.AttributePath, names map[string]bool) bool {
	if p == nil || len(p.Steps()) != 1 {
		return false
	}
	name, ok := p.Steps()[0].(tftypes.AttributeName)
	return ok && names[string(name)]
}

// emptyCollection returns an empty value of a list, set or map type, and a null value of any
// other type.
func emptyCollection(typ tftypes.Type) tftypes.Value {
	switch {
	case typ.Is(tftypes.Map{}):
		return tftypes.NewValue(typ, map[string]tftypes.Value{})
	case typ.Is(tftypes.List{}), typ.Is(tftypes.Set{}):
		return tftypes.NewValue(typ, []tftypes.Value{})
	default:
		return tftypes.NewValue(typ, nil)
	}
}

func (g *configCheck) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	g.inner.Delete(ctx, req, resp)
}

func (g *configCheck) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if r, ok := g.inner.(resource.ResourceWithConfigure); ok {
		r.Configure(ctx, req, resp)
	}
}

func (g *configCheck) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r, ok := g.inner.(resource.ResourceWithModifyPlan)
	if !ok {
		return
	}
	r.ModifyPlan(ctx, req, resp)
	if strings.HasSuffix(g.typeName, "_import") && resp.Diagnostics.HasError() && !req.Plan.Raw.IsNull() {
		g.deferBundleErrors(ctx, resp)
	}
}

// invalidBundleHash is the content hash that the plan of an import gets when its files cannot be
// read. It differs from every real hash, so the plan shows a change and the provider runs the
// update, which reports the error.
const invalidBundleHash = "invalid"

// deferBundleErrors turns the errors that the plan of an import reports about its files into
// warnings. Upjet plans every time it observes a managed resource, and an observation that fails
// stops Crossplane from deleting the managed resource. So an import whose files someone broke
// could not be deleted, and its imported objects would stay in Superset. With the errors deferred,
// the observation works, and the update reports the errors instead.
func (g *configCheck) deferBundleErrors(ctx context.Context, resp *resource.ModifyPlanResponse) {
	var deferred diag.Diagnostics
	for _, d := range resp.Diagnostics {
		if d.Severity() == diag.SeverityError {
			deferred.AddWarning(d.Summary(), d.Detail())
			continue
		}
		deferred.Append(d)
	}
	resp.Diagnostics = deferred
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_hash"), invalidBundleHash)...)
}

func (g *configCheck) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r, ok := g.inner.(resource.ResourceWithImportState); ok {
		r.ImportState(ctx, req, resp)
		return
	}
	resp.Diagnostics.AddError("Resource Import Not Implemented", "This resource does not support import.")
}

func (g *configCheck) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r, ok := g.inner.(resource.ResourceWithValidateConfig); ok {
		r.ValidateConfig(ctx, req, resp)
	}
}

func (g *configCheck) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	if r, ok := g.inner.(resource.ResourceWithConfigValidators); ok {
		return r.ConfigValidators(ctx)
	}
	return nil
}
