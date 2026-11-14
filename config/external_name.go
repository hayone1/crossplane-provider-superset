package config

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/pkg/errors"
)

// The external name of a managed resource is the Terraform `id` of its object, which for
// almost every kind is the number that Superset assigns. Crossplane creates the object first
// and then records the number. To manage an existing object, set the
// crossplane.io/external-name annotation to its number.
//
// Upjet's own configuration for server-assigned IDs (config.IdentifierFromProvider) expects
// a string `id`. This provider's IDs are numbers, which needs two changes:
//
//   - Before the first create, the external name is empty, and Upjet would put "" into the
//     `id` of the state that it reads the object with. "" is not a number, so every
//     reconcile would fail. numericID reads the object with the ID 0 instead, which no
//     Superset object has.
//   - After a create or a read, the `id` in the state is a number, and Upjet's function
//     that turns it into the external name accepts only strings.
//
// Some kinds have an ID that is not a Superset object of its own:
//
//   - DashboardEmbedding, DatasetRefresh and DatabasePermissionsSync use the ID of their
//     dashboard, dataset or database.
//   - SQLExecution uses the ID of its last run in the query history of SQL Lab.
//   - RolePermissions uses the ID of its role (a string), and manages the permissions of
//     the role that roleName names.
//   - The import kinds (assets group) have no ID: an import is an action, not an object. Their
//     external name is the hash of the imported bundle, and a state without a hash means
//     that the bundle was not imported yet.

// numericIDNames are the resources whose external name is a number.
var numericIDNames = []string{
	"superset_annotation",
	"superset_annotation_layer",
	"superset_chart",
	"superset_css_template",
	"superset_dashboard",
	"superset_dashboard_embedding",
	"superset_database",
	"superset_database_permissions_sync",
	"superset_dataset",
	"superset_dataset_refresh",
	"superset_group",
	"superset_meta_database",
	"superset_report",
	"superset_role",
	"superset_role_permissions",
	"superset_row_level_security",
	"superset_saved_query",
	"superset_sql_execution",
	"superset_tag",
	"superset_theme",
	"superset_user",
}

// bundleNames are the import resources, whose external name is the hash of their bundle.
var bundleNames = []string{
	"superset_assets_import",
	"superset_chart_import",
	"superset_dashboard_import",
	"superset_dataset_import",
}

// Attributes that tell whether an object exists.
const (
	attrID          = "id"
	attrContentHash = "content_hash"
)

// runsInBackground reports whether the operations of a kind run in the background. Imports,
// dataset refreshes and permission syncs can take longer than a reconcile may (three minutes),
// and running them again does no harm: an import overwrites the same objects, and the other two
// have the ID of their dataset or connection. Every other kind runs while the reconcile waits.
// Then Crossplane records that a create started before it calls Superset, and after a provider
// restart in the middle of a create it stops with "cannot determine creation result" instead
// of creating a second object. A create in the background reports success at once, and a
// restart would lose the ID of the new object.
func runsInBackground(name string) bool {
	_, isAction := parentIDArguments[name]
	return existenceAttribute(name) == attrContentHash || (isAction && name != "superset_dashboard_embedding")
}

// parentIDArguments are the kinds whose ID is the ID of another object, with the argument that
// names that object.
var parentIDArguments = map[string]string{
	"superset_dashboard_embedding":       "dashboard_id",
	"superset_dataset_refresh":           "dataset_id",
	"superset_database_permissions_sync": "database_id",
}

// parentIDFromExternalName returns a function that fills the argument that names the parent
// object from the external name, when the spec leaves the argument out. A managed resource with
// the management policy Observe then needs only the external-name annotation, as other kinds
// do. A spec that sets the argument keeps its value.
func parentIDFromExternalName(arg string) config.SetIdentifierArgumentsFn {
	return func(base map[string]any, externalName string) {
		if externalName == "" || base[arg] != nil {
			return
		}
		if _, err := strconv.ParseInt(externalName, 10, 64); err == nil {
			base[arg] = externalName
		}
	}
}

// noID is the ID that numericID reads an object with before the first create. Superset
// numbers its objects from 1.
const noID = "0"

// numericID is the external name configuration of the resources with a numeric ID.
var numericID = config.ExternalName{
	SetIdentifierArgumentFn: config.NopSetIdentifierArgument,
	GetExternalNameFn:       numberAsExternalName(attrID),
	GetIDFn:                 externalNameAsNumericID,
	DisableNameInitializer:  true,
	TFPluginFrameworkOptions: config.TFPluginFrameworkOptions{
		// The `id` is computed, so it must not be part of the configuration that Upjet
		// plans with. Upjet still puts it into the state.
		ComputedIdentifierAttributes: []string{attrID},
	},
}

// bundleHash is the external name configuration of the import resources.
var bundleHash = config.ExternalName{
	SetIdentifierArgumentFn: config.NopSetIdentifierArgument,
	GetExternalNameFn:       stringAsExternalName(attrContentHash),
	GetIDFn:                 config.ExternalNameAsID,
	DisableNameInitializer:  true,
}

// ExternalNameConfigs holds the external name configuration of every resource. Only the
// resources listed here are generated.
var ExternalNameConfigs = func() map[string]config.ExternalName {
	m := make(map[string]config.ExternalName, len(numericIDNames)+len(bundleNames))
	for _, name := range numericIDNames {
		m[name] = numericID
	}
	for _, name := range bundleNames {
		m[name] = bundleHash
	}
	return m
}()

// existenceAttribute returns the attribute whose value tells whether the object of a
// resource exists.
func existenceAttribute(name string) string {
	for _, b := range bundleNames {
		if b == name {
			return attrContentHash
		}
	}
	return attrID
}

// ExternalNameConfigurations applies the external name configuration of each resource. It
// also makes Crossplane treat a state without an ID (or without a bundle hash) as "the
// object does not exist yet", and turns late initialization off.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if e, ok := ExternalNameConfigs[r.Name]; ok {
			r.ExternalName = e
		}
		if arg, ok := parentIDArguments[r.Name]; ok {
			r.ExternalName.SetIdentifierArgumentFn = parentIDFromExternalName(arg)
		}
		r.UseAsync = runsInBackground(r.Name)
		r.TerraformPluginFrameworkIsStateEmptyFn = stateWithout(existenceAttribute(r.Name))
		if existenceAttribute(r.Name) == attrID {
			r.TerraformConversions = append(r.TerraformConversions, idAsString{})
		}
		r.LateInitializer = config.LateInitializer{IgnoredFields: topLevelArguments(r)}
		crossplaneDocs(r)
	}
}

// topLevelArguments returns the names of all top-level arguments of a resource. The provider
// turns late initialization off for all of them. For an optional argument that Superset fills in
// itself (optional and computed in the Terraform schema), leaving it out means "leave it to
// Superset", for example the columns and metrics of a dataset or the owners of a chart. Copying
// the values of Superset into spec.forProvider would change that meaning: the managed resource
// would then own every column, and a column added in Superset would be removed again. The current
// values are in status.atProvider, and docs/behaviour.md tells users which fields Superset keeps.
func topLevelArguments(r *config.Resource) []string {
	names := make([]string, 0, len(r.TerraformResource.Schema))
	for name, s := range r.TerraformResource.Schema {
		if s.Optional || s.Required {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ExternalNameConfigured returns the include list of the Terraform plugin framework
// resources: every resource with an external name configuration.
func ExternalNameConfigured() []string {
	l := make([]string, 0, len(ExternalNameConfigs))
	for name := range ExternalNameConfigs {
		// The list holds regular expressions. "^" and "$" make each one match exactly.
		l = append(l, "^"+name+"$")
	}
	sort.Strings(l)
	return l
}

// idAsString turns the numeric `id` of the Terraform state into a string on its way to
// status.atProvider. Upjet generates status.atProvider.id as a string for every resource, and
// setting the observation fails with "ReadString: expects \" or n" when the state holds a
// number. The other direction needs no conversion: Upjet hands the string back to Terraform,
// which reads a string into a Number attribute.
type idAsString struct{}

func (idAsString) Convert(params map[string]any, _ *config.Resource, mode config.Mode) (map[string]any, error) {
	if mode != config.FromTerraform || params == nil {
		return params, nil
	}
	if _, isString := params[attrID].(string); isString || params[attrID] == nil {
		return params, nil
	}
	id, err := formatWholeNumber(params[attrID])
	if err != nil {
		return nil, errors.Wrapf(err, "cannot convert the %s of the Terraform state", attrID)
	}
	params[attrID] = id
	return params, nil
}

// formatWholeNumber formats a whole number of the Terraform state, as Upjet converts it to
// Go, without a fraction or an exponent.
func formatWholeNumber(v any) (string, error) {
	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10), nil
	case int:
		return strconv.Itoa(n), nil
	case float64:
		return formatWholeFloat(n)
	case json.Number:
		if _, err := strconv.ParseInt(n.String(), 10, 64); err != nil {
			return "", errors.Errorf("%s is not a whole number", n)
		}
		return n.String(), nil
	case *big.Float:
		if !n.IsInt() {
			return "", errors.Errorf("%v is not a whole number", n)
		}
		return n.Text('f', 0), nil
	case string:
		return strings.TrimSpace(n), nil
	default:
		return "", errors.Errorf("unexpected type %T", v)
	}
}

func formatWholeFloat(n float64) (string, error) {
	if n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
		return "", errors.Errorf("%v is not a whole number", n)
	}
	return strconv.FormatFloat(n, 'f', 0, 64), nil
}

// externalNameAsNumericID returns the Terraform `id` for an external name: the name itself,
// or noID before the first create.
func externalNameAsNumericID(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
	if externalName == "" {
		return noID, nil
	}
	n, err := strconv.ParseInt(externalName, 10, 64)
	if err != nil || n <= 0 {
		return "", errors.Errorf("the external name %q is not the ID of a Superset object: set the crossplane.io/external-name annotation to a positive whole number", externalName)
	}
	return externalName, nil
}

// numberAsExternalName returns a function that reads the external name from a numeric
// attribute of the Terraform state. Upjet's IDAsExternalName accepts only strings. The
// RolePermissions `id` is a string, which is accepted as well.
func numberAsExternalName(attr string) config.GetExternalNameFn {
	return func(tfstate map[string]any) (string, error) {
		v, ok := tfstate[attr]
		if !ok || v == nil {
			return "", errors.Errorf("cannot find the %s in the Terraform state", attr)
		}
		s, err := formatWholeNumber(v)
		if err != nil {
			return "", errors.Wrapf(err, "cannot read the %s of the Terraform state", attr)
		}
		if s == "" || s == noID {
			return "", errors.Errorf("cannot find the %s in the Terraform state", attr)
		}
		return s, nil
	}
}

// stringAsExternalName returns a function that reads the external name from a string
// attribute of the Terraform state.
func stringAsExternalName(attr string) config.GetExternalNameFn {
	return func(tfstate map[string]any) (string, error) {
		if s, ok := tfstate[attr].(string); ok && s != "" {
			return s, nil
		}
		return "", errors.Errorf("cannot find the %s in the Terraform state", attr)
	}
}

// stateWithout returns a function that reports a Terraform state as empty when the given
// attribute is missing, null, unknown, zero or an empty string. Upjet reads every object
// before it creates it, with the ID noID. Most kinds answer that read with "not found", but
// some read their object through another argument: a DatasetRefresh finds its dataset, an
// import finds nothing to check. Without this function Crossplane would treat such an object
// as existing and never create it.
func stateWithout(attr string) config.TerraformPluginFrameworkIsStateEmptyFn {
	return func(_ context.Context, v tftypes.Value, _ schema.Schema) (bool, error) {
		if v.IsNull() || !v.IsKnown() {
			return true, nil
		}
		var attrs map[string]tftypes.Value
		if err := v.As(&attrs); err != nil {
			return false, errors.Wrap(err, "cannot read the Terraform state as an object")
		}
		a, ok := attrs[attr]
		if !ok || a.IsNull() || !a.IsKnown() {
			return true, nil
		}
		empty, err := isZero(a)
		return empty, errors.Wrapf(err, "cannot read the %s of the Terraform state", attr)
	}
}

// isZero reports whether a known number is zero, or a known string is empty or noID.
func isZero(a tftypes.Value) (bool, error) {
	switch {
	case a.Type().Is(tftypes.Number):
		var n big.Float
		if err := a.As(&n); err != nil {
			return false, err
		}
		return n.Sign() == 0, nil
	case a.Type().Is(tftypes.String):
		var s string
		if err := a.As(&s); err != nil {
			return false, err
		}
		return s == "" || s == noID, nil
	default:
		return false, errors.Errorf("unexpected type %s", a.Type())
	}
}
