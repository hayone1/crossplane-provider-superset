package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"sigs.k8s.io/yaml"
)

func schemaResourceNames(t *testing.T) []string {
	t.Helper()
	var doc struct {
		ProviderSchemas map[string]struct {
			ResourceSchemas map[string]json.RawMessage `json:"resource_schemas"`
		} `json:"provider_schemas"`
	}
	if err := json.Unmarshal([]byte(providerSchema), &doc); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ps := range doc.ProviderSchemas {
		for name := range ps.ResourceSchemas {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Every resource of the Terraform provider must be generated, in both API variants, with a
// kind, a group, a framework resource and an existence check.
func TestProviders_configureEveryResource(t *testing.T) {
	names := schemaResourceNames(t)
	if len(names) != 24 {
		t.Fatalf("schema.json has %d resources, want 24", len(names))
	}
	for scope, pc := range map[string]*ujconfig.Provider{"cluster": GetProvider(), "namespaced": GetProviderNamespaced()} {
		seenKinds := map[string]string{}
		for _, name := range names {
			r, ok := pc.Resources[name]
			if !ok {
				t.Errorf("%s: %s is not configured", scope, name)
				continue
			}
			k, ok := kinds[name]
			if !ok {
				t.Errorf("%s has no group and kind", name)
				continue
			}
			if r.Kind != k.kind || r.ShortGroup != k.group {
				t.Errorf("%s: %s has kind %s.%s, want %s.%s", scope, name, r.ShortGroup, r.Kind, k.group, k.kind)
			}
			if other, dup := seenKinds[r.Kind]; dup {
				t.Errorf("kind %s is used by %s and %s", r.Kind, other, name)
			}
			seenKinds[r.Kind] = name
			if r.TerraformPluginFrameworkResource == nil {
				t.Errorf("%s: %s has no framework resource", scope, name)
			}
			if r.TerraformPluginFrameworkIsStateEmptyFn == nil {
				t.Errorf("%s: %s has no check for an empty state", scope, name)
			}
			// See runsInBackground.
			wantAsync := map[string]bool{
				"superset_assets_import": true, "superset_chart_import": true, "superset_dashboard_import": true,
				"superset_dataset_import": true, "superset_dataset_refresh": true, "superset_database_permissions_sync": true,
			}[name]
			if r.UseAsync != wantAsync {
				t.Errorf("%s: %s has UseAsync %t, want %t", scope, name, r.UseAsync, wantAsync)
			}
		}
		if len(pc.Resources) != len(names) {
			t.Errorf("%s: %d resources configured, want %d", scope, len(pc.Resources), len(names))
		}
	}
	if len(ExternalNameConfigs) != len(names) || len(kinds) != len(names) {
		t.Errorf("ExternalNameConfigs has %d entries and kinds has %d, want %d each", len(ExternalNameConfigs), len(kinds), len(names))
	}
}

// Every resource except the imports has a numeric id, which numericID handles, and the imports
// have none.
func TestExternalNameMatchesTheIDType(t *testing.T) {
	for name, r := range GetProvider().Resources {
		_, hasID := r.TerraformResource.Schema[attrID]
		switch existenceAttribute(name) {
		case attrID:
			if !hasID {
				t.Errorf("%s has no id, but its external name is the id", name)
			}
		case attrContentHash:
			if hasID {
				t.Errorf("%s has an id, but its external name is the bundle hash", name)
			}
			if _, ok := r.TerraformResource.Schema[attrContentHash]; !ok {
				t.Errorf("%s has no %s", name, attrContentHash)
			}
		}
	}
}

func TestExternalNameAsNumericID(t *testing.T) {
	for in, want := range map[string]string{"": noID, "12": "12", "9007199254740993": "9007199254740993"} {
		got, err := externalNameAsNumericID(context.Background(), in, nil, nil)
		if err != nil || got != want {
			t.Errorf("externalNameAsNumericID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", "0", "-3", "1.5", " 12"} {
		if _, err := externalNameAsNumericID(context.Background(), in, nil, nil); err == nil {
			t.Errorf("externalNameAsNumericID(%q) did not fail", in)
		}
	}
}

func TestNumberAsExternalName(t *testing.T) {
	get := numberAsExternalName(attrID)
	for _, in := range []any{int64(42), 42, float64(42), json.Number("42"), big.NewFloat(42), "42"} {
		got, err := get(map[string]any{attrID: in})
		if err != nil || got != "42" {
			t.Errorf("numberAsExternalName(%T %v) = %q, %v; want 42", in, in, got, err)
		}
	}
	for _, state := range []map[string]any{{}, {attrID: nil}, {attrID: ""}, {attrID: float64(0)}, {attrID: 4.5}, {attrID: true}} {
		if got, err := get(state); err == nil {
			t.Errorf("numberAsExternalName(%v) = %q, want an error", state, got)
		}
	}
}

// Upjet generates status.atProvider.id as a string, so the numeric id of the state must turn
// into a string on its way there, and only then.
func TestIDAsString(t *testing.T) {
	c := idAsString{}
	got, err := c.Convert(map[string]any{attrID: float64(7), "name": "x"}, nil, ujconfig.FromTerraform)
	if err != nil || got[attrID] != "7" || got["name"] != "x" {
		t.Errorf("FromTerraform: %v, %v", got, err)
	}
	got, err = c.Convert(map[string]any{attrID: "7"}, nil, ujconfig.ToTerraform)
	if err != nil || got[attrID] != "7" {
		t.Errorf("ToTerraform: %v, %v", got, err)
	}
	got, err = c.Convert(map[string]any{}, nil, ujconfig.FromTerraform)
	if err != nil || len(got) != 0 {
		t.Errorf("no id: %v, %v", got, err)
	}
	if _, err := c.Convert(map[string]any{attrID: 1.5}, nil, ujconfig.FromTerraform); err == nil {
		t.Error("a fraction must fail")
	}
	for name, r := range GetProvider().Resources {
		has := false
		for _, conv := range r.TerraformConversions {
			if _, ok := conv.(idAsString); ok {
				has = true
			}
		}
		if has != (existenceAttribute(name) == attrID) {
			t.Errorf("%s: the id conversion is configured: %t", name, has)
		}
	}
}

func TestStateWithout(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"id": tftypes.Number, "content_hash": tftypes.String}}
	state := func(id any, hash any) tftypes.Value {
		var idv, hashv tftypes.Value
		switch v := id.(type) {
		case nil:
			idv = tftypes.NewValue(tftypes.Number, nil)
		case string:
			idv = tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
		default:
			idv = tftypes.NewValue(tftypes.Number, big.NewFloat(float64(v.(int))))
		}
		if hash == nil {
			hashv = tftypes.NewValue(tftypes.String, nil)
		} else {
			hashv = tftypes.NewValue(tftypes.String, hash)
		}
		return tftypes.NewValue(objType, map[string]tftypes.Value{"id": idv, "content_hash": hashv})
	}
	cases := []struct {
		name  string
		attr  string
		value tftypes.Value
		empty bool
	}{
		{"null state", attrID, tftypes.NewValue(objType, nil), true},
		{"null id", attrID, state(nil, nil), true},
		{"unknown id", attrID, state("unknown", nil), true},
		{"zero id", attrID, state(0, nil), true},
		{"id", attrID, state(12, nil), false},
		{"no hash", attrContentHash, state(nil, nil), true},
		{"empty hash", attrContentHash, state(nil, ""), true},
		{"hash", attrContentHash, state(nil, "abc"), false},
		{"missing attribute", "other", state(12, "abc"), true},
	}
	for _, tc := range cases {
		got, err := stateWithout(tc.attr)(context.Background(), tc.value, schema.Schema{})
		if err != nil || got != tc.empty {
			t.Errorf("%s: empty = %t, %v; want %t", tc.name, got, err, tc.empty)
		}
	}
	// A string id, as RolePermissions has.
	strType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"id": tftypes.String}}
	for id, empty := range map[string]bool{"": true, noID: true, "5": false} {
		v := tftypes.NewValue(strType, map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, id)})
		if got, err := stateWithout(attrID)(context.Background(), v, schema.Schema{}); err != nil || got != empty {
			t.Errorf("string id %q: empty = %t, %v; want %t", id, got, err, empty)
		}
	}
}

func TestLateInitializationIsOff(t *testing.T) {
	for name, r := range GetProvider().Resources {
		ignored := map[string]bool{}
		for _, f := range r.LateInitializer.IgnoredFields {
			ignored[f] = true
		}
		for arg, s := range r.TerraformResource.Schema {
			if (s.Optional || s.Required) && !ignored[arg] {
				t.Errorf("%s: late initialization is on for %s", name, arg)
			}
		}
	}
}

// The import kinds take their bundle inline only: source would make the provider read files of
// its own pod.
func TestImportsTakeFilesOnly(t *testing.T) {
	for _, pc := range []*ujconfig.Provider{GetProvider(), GetProviderNamespaced()} {
		for _, name := range bundleNames {
			r := pc.Resources[name]
			if _, ok := r.TerraformResource.Schema["source"]; ok {
				t.Errorf("%s has the source argument", name)
			}
			files, ok := r.TerraformResource.Schema["files"]
			if !ok || !files.Required {
				t.Errorf("%s: files is missing or optional", name)
			}
		}
	}
}

// Every reference must point at a configured resource.
func TestReferencesPointAtResources(t *testing.T) {
	pc := GetProvider()
	for name, refs := range references {
		r, ok := pc.Resources[name]
		if !ok {
			t.Errorf("references of an unknown resource %s", name)
			continue
		}
		for field, ref := range refs {
			if _, ok := pc.Resources[ref.TerraformName]; !ok {
				t.Errorf("%s.%s refers to the unknown resource %s", name, field, ref.TerraformName)
			}
			if _, ok := r.References[field]; !ok {
				t.Errorf("%s.%s: the reference is not configured", name, field)
			}
		}
	}
}

// The CRD descriptions must not talk about Terraform.
func TestDescriptionsUseCrossplaneTerms(t *testing.T) {
	forbidden := []string{"Terraform", "terraform", "jsonencode", "To import", "depends_on", "path.module", "superset_"}
	for name, r := range GetProvider().Resources {
		texts := map[string]string{"description": r.MetaResource.Description}
		for k, v := range r.MetaResource.ArgumentDocs {
			if _, ok := r.TerraformResource.Schema[k]; ok {
				texts[k] = v
			}
		}
		for field, text := range texts {
			if strings.Contains(text, "destroy") {
				t.Errorf("%s %s says destroy: %s", name, field, text)
			}
			for _, f := range forbidden {
				// superset_config.py is Superset's own configuration file.
				if strings.Contains(strings.ReplaceAll(text, "superset_config.py", ""), f) {
					t.Errorf("%s %s contains %q: %s", name, field, f, text)
				}
			}
		}
		if !strings.HasSuffix(r.MetaResource.Description, importSentence(name)) {
			t.Errorf("%s description does not end with %q: %s", name, importSentence(name), r.MetaResource.Description)
		}
	}
}

func TestResourcesDocListsEveryKind(t *testing.T) {
	doc, err := os.ReadFile("../docs/resources.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, k := range kinds {
		row := fmt.Sprintf("| `%s` | `%s` | `%s` |", k.kind, k.group, name)
		if !strings.Contains(string(doc), row) {
			t.Errorf("docs/resources.md has no row that starts with %s", row)
		}
	}
}

// Every kind needs a cluster-scoped and a namespaced example, so that users can copy one and
// the end-to-end tests cover it.
func TestEveryKindHasExamples(t *testing.T) {
	for scope, suffix := range map[string]string{"cluster": "superset.crossplane.io", "namespaced": "superset.m.crossplane.io"} {
		found := map[string]bool{}
		err := filepath.WalkDir(filepath.Join("..", "examples", scope), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, doc := range strings.Split(string(b), "\n---\n") {
				var obj struct {
					APIVersion string `json:"apiVersion"`
					Kind       string `json:"kind"`
				}
				if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				found[obj.APIVersion+"/"+obj.Kind] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range kinds {
			if !found[k.group+"."+suffix+"/v1alpha1/"+k.kind] {
				t.Errorf("examples/%s has no %s (%s.%s)", scope, k.kind, k.group, suffix)
			}
		}
	}
}

func TestEmbeddingConnectionDetails(t *testing.T) {
	got, err := embeddingConnectionDetails(map[string]any{"uuid": "8b0c", "id": "3"})
	if err != nil || string(got["uuid"]) != "8b0c" || len(got) != 1 {
		t.Errorf("got %v, %v", got, err)
	}
	if got, _ := embeddingConnectionDetails(map[string]any{}); len(got) != 0 {
		t.Errorf("no uuid: got %v", got)
	}
	if GetProvider().Resources["superset_dashboard_embedding"].Sensitive.AdditionalConnectionDetailsFn == nil {
		t.Error("the DashboardEmbedding publishes no connection details")
	}
}

func TestParentIDFromExternalName(t *testing.T) {
	set := parentIDFromExternalName("dashboard_id")
	params := map[string]any{}
	set(params, "12")
	if params["dashboard_id"] != "12" {
		t.Errorf("the dashboard ID is not filled in: %v", params)
	}
	params = map[string]any{"dashboard_id": float64(7)}
	set(params, "12")
	if params["dashboard_id"] != float64(7) {
		t.Errorf("the spec's dashboard ID was replaced: %v", params)
	}
	for _, name := range []string{"", "abc"} {
		params = map[string]any{}
		set(params, name)
		if _, ok := params["dashboard_id"]; ok {
			t.Errorf("external name %q filled in %v", name, params)
		}
	}
	for name, arg := range parentIDArguments {
		r := GetProvider().Resources[name]
		if _, ok := r.TerraformResource.Schema[arg]; !ok {
			t.Errorf("%s has no argument %s", name, arg)
		}
	}
}

// test/e2e/getting-started.yaml tests the manifest of docs/getting-started.md as written, so the
// two must hold the same manifest.
func TestGettingStartedManifestMatchesTheDocs(t *testing.T) {
	doc, err := os.ReadFile("../docs/getting-started.md")
	if err != nil {
		t.Fatal(err)
	}
	test, err := os.ReadFile("../test/e2e/getting-started.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)Save the following manifest as `analytics.yaml`.*?```yaml\n(.*?)\n   ```").FindSubmatch(doc)
	if m == nil {
		t.Fatal("docs/getting-started.md has no analytics.yaml manifest")
	}
	lines := strings.Split(string(m[1]), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "   ")
	}
	if !strings.HasSuffix(string(test), strings.Join(lines, "\n")+"\n") {
		t.Error("test/e2e/getting-started.yaml differs from the manifest of docs/getting-started.md")
	}
}
