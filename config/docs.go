package config

import (
	"regexp"
	"strings"

	"github.com/crossplane/upjet/v2/pkg/config"
)

// The descriptions in the CRDs come from the Terraform provider's documentation. The
// rewrites below turn the parts that only make sense for Terraform into Crossplane terms:
// Terraform resource names become kinds, "destroy" becomes "delete the managed resource",
// and import instructions describe the external-name annotation.

// docReplacements are exact text replacements, applied in order.
var docReplacements = []struct{ old, new string }{
	{"outside Terraform", "outside Crossplane"},
	{"Terraform does not manage", "Crossplane does not manage"},
	{"Destroying the resource", "Deleting the managed resource"},
	{"destroying the resource", "deleting the managed resource"},
	{"When the resource is destroyed", "When the managed resource is deleted"},
	{"when the resource is destroyed", "when the managed resource is deleted"},
	{"order them with `depends_on` (datasets, then charts, then dashboards)", "delete the dashboard imports first, then the chart imports, then the dataset imports"},
	{"order them with depends_on (datasets, then charts, then dashboards)", "delete the dashboard imports first, then the chart imports, then the dataset imports"},
	{"the next plan imports the bundle again", "the provider imports the bundle again"},
	{"A change of the files plans an import.", "A change of the files imports the bundle again."},
	{"plans an import", "imports the bundle again"},
	{"terraform plan", "the next reconcile"},
	{" is destroyed", " is deleted"},
	{" are destroyed", " are deleted"},
}

var (
	// The import sentence is always the last sentence.
	reImportSentence   = regexp.MustCompile(`\s*(To import|Import is supported).*$`)
	reTerraformName    = regexp.MustCompile("`?\\bsuperset_[a-z0-9_]+\\b`?")
	reMultipleSpaces   = regexp.MustCompile(`  +`)
	reSpaceBeforePunct = regexp.MustCompile(` +([.,])`)
)

// parentIDs describes the kinds whose ID is the ID of another object.
var parentIDs = map[string]string{
	"superset_dashboard_embedding":       "The external name is the ID of the dashboard. If the dashboard is already embeddable, the managed resource takes the embedding over.",
	"superset_dataset_refresh":           "The external name is the ID of the dataset.",
	"superset_database_permissions_sync": "The external name is the ID of the database connection.",
	"superset_role_permissions":          "The external name is the ID of the role. The managed resource takes over the permissions of the role that roleName names.",
}

// importSentence returns the sentence that explains how a managed resource finds an
// existing object.
func importSentence(name string) string {
	if s, ok := parentIDs[name]; ok {
		return s
	}
	switch existenceAttribute(name) {
	case attrContentHash:
		return "The external name is the hash of the imported bundle; the provider sets it."
	default:
		return "To manage an existing object, set the crossplane.io/external-name annotation to its numeric ID."
	}
}

// crossplaneDocs rewrites the descriptions of a resource and its arguments.
func crossplaneDocs(r *config.Resource) {
	if r.MetaResource == nil {
		return
	}
	r.MetaResource.Description = rewriteDoc(r.MetaResource.Description)
	if reImportSentence.MatchString(r.MetaResource.Description) {
		r.MetaResource.Description = reImportSentence.ReplaceAllString(r.MetaResource.Description, " "+importSentence(r.Name))
	} else {
		r.MetaResource.Description = strings.TrimSpace(r.MetaResource.Description) + " " + importSentence(r.Name)
	}
	r.MetaResource.Description = tidy(r.MetaResource.Description)
	for k, v := range r.MetaResource.ArgumentDocs {
		r.MetaResource.ArgumentDocs[k] = tidy(rewriteDoc(v))
	}
}

func rewriteDoc(s string) string {
	for _, rep := range docReplacements {
		s = strings.ReplaceAll(s, rep.old, rep.new)
	}
	return reTerraformName.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.Trim(match, "`")
		if k, ok := kinds[name]; ok {
			return k.kind
		}
		return match
	})
}

func tidy(s string) string {
	s = reMultipleSpaces.ReplaceAllString(s, " ")
	s = reSpaceBeforePunct.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}
