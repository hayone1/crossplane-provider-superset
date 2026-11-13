package config

import (
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
)

// apiGroup and kind of every managed resource. The kinds are unique across the groups,
// so that `kubectl get <kind>` is never ambiguous within this provider.
type kindInfo struct {
	group string
	kind  string
}

var kinds = map[string]kindInfo{
	// Users, roles and what they may see.
	"superset_role":               {"security", "Role"},
	"superset_role_permissions":   {"security", "RolePermissions"},
	"superset_user":               {"security", "User"},
	"superset_group":              {"security", "Group"},
	"superset_row_level_security": {"security", "RowLevelSecurity"},

	// Database connections.
	"superset_database":                  {"database", "Database"},
	"superset_meta_database":             {"database", "MetaDatabase"},
	"superset_database_permissions_sync": {"database", "DatabasePermissionsSync"},

	// Datasets: the tables and queries that charts are built on.
	"superset_dataset":         {"dataset", "Dataset"},
	"superset_dataset_refresh": {"dataset", "DatasetRefresh"},

	// Charts and dashboards.
	"superset_chart":               {"chart", "Chart"},
	"superset_dashboard":           {"dashboard", "Dashboard"},
	"superset_dashboard_embedding": {"dashboard", "DashboardEmbedding"},
	"superset_css_template":        {"dashboard", "CSSTemplate"},
	"superset_theme":               {"theme", "Theme"},

	// SQL Lab.
	"superset_saved_query":   {"sqllab", "SavedQuery"},
	"superset_sql_execution": {"sqllab", "SQLExecution"},

	// Annotations that charts show on their time axis.
	"superset_annotation_layer": {"annotation", "AnnotationLayer"},
	"superset_annotation":       {"annotation", "Annotation"},

	// Tags of charts and dashboards.
	"superset_tag": {"tag", "Tag"},

	// Alerts and reports.
	"superset_report": {"report", "Report"},

	// Imports of export bundles.
	"superset_assets_import":    {"assets", "AssetsImport"},
	"superset_dashboard_import": {"assets", "DashboardImport"},
	"superset_chart_import":     {"assets", "ChartImport"},
	"superset_dataset_import":   {"assets", "DatasetImport"},
}

// Extractors for references that do not use the external name of the referenced object. They
// read status.atProvider, which the provider fills only after the object exists in Superset,
// so a reference does not resolve before its object is created. A value from spec.forProvider
// would be there at once, and the referencing object would fail to create until the other one
// exists.
const (
	// extractConnectionName reads the connectionName of a Database. Datasets, reports and meta
	// databases name their database connection instead of giving its ID.
	extractConnectionName = `github.com/crossplane/upjet/v2/pkg/resource.ExtractParamPath("connection_name", true)`
	// extractName reads the name of the referenced object, for references by name.
	extractName = `github.com/crossplane/upjet/v2/pkg/resource.ExtractParamPath("name", true)`
	// extractDatasetPerm reads the perm of a Dataset: the view menu of its datasource_access
	// permission, such as [warehouse].[sales](id:12).
	extractDatasetPerm = `github.com/crossplane/upjet/v2/pkg/resource.ExtractParamPath("perm", true)`
)

// references of every resource, keyed by Terraform resource name and then by argument path.
// They are declared by Terraform resource name, so the same configuration resolves to the
// right Go types in either API variant.
var references = map[string]map[string]ujconfig.Reference{
	"superset_role_permissions": {
		"role_name":                      {TerraformName: "superset_role", Extractor: extractName},
		"resource_permissions.view_menu": {TerraformName: "superset_dataset", Extractor: extractDatasetPerm},
	},
	"superset_user": {
		"roles":  {TerraformName: "superset_role"},
		"groups": {TerraformName: "superset_group"},
	},
	"superset_group": {
		"roles": {TerraformName: "superset_role"},
		"users": {TerraformName: "superset_user"},
	},
	"superset_row_level_security": {
		"roles":  {TerraformName: "superset_role"},
		"tables": {TerraformName: "superset_dataset"},
	},
	"superset_meta_database": {
		"allowed_databases": {TerraformName: "superset_database", Extractor: extractConnectionName},
	},
	"superset_database_permissions_sync": {
		"database_id": {TerraformName: "superset_database"},
	},
	"superset_dataset": {
		"database_name": {TerraformName: "superset_database", Extractor: extractConnectionName},
		"owners":        {TerraformName: "superset_user"},
	},
	"superset_dataset_refresh": {
		"dataset_id": {TerraformName: "superset_dataset"},
	},
	"superset_chart": {
		"datasource_id": {TerraformName: "superset_dataset"},
		"dashboards":    {TerraformName: "superset_dashboard"},
		"owners":        {TerraformName: "superset_user"},
		"tags":          {TerraformName: "superset_tag", Extractor: extractName},
	},
	"superset_dashboard": {
		"owners":   {TerraformName: "superset_user"},
		"roles":    {TerraformName: "superset_role"},
		"theme_id": {TerraformName: "superset_theme"},
		"tags":     {TerraformName: "superset_tag", Extractor: extractName},
	},
	"superset_dashboard_embedding": {
		"dashboard_id": {TerraformName: "superset_dashboard"},
	},
	"superset_saved_query": {
		"database_id": {TerraformName: "superset_database"},
	},
	"superset_sql_execution": {
		"database_id":   {TerraformName: "superset_database"},
		"database_name": {TerraformName: "superset_database", Extractor: extractConnectionName},
	},
	"superset_annotation": {
		"layer_id": {TerraformName: "superset_annotation_layer"},
	},
	"superset_report": {
		"chart_id":      {TerraformName: "superset_chart"},
		"dashboard_id":  {TerraformName: "superset_dashboard"},
		"database_name": {TerraformName: "superset_database", Extractor: extractConnectionName},
		"owners":        {TerraformName: "superset_user"},
	},
}

// configurators are applied to both the cluster-scoped and the namespaced provider.
var configurators = []func(*ujconfig.Provider){
	configureKinds,
	configureReferences,
	configureImports,
	configureEmbedding,
}

func configureKinds(p *ujconfig.Provider) {
	for name, k := range kinds {
		p.AddResourceConfigurator(name, func(r *ujconfig.Resource) {
			r.ShortGroup = k.group
			r.Kind = k.kind
		})
	}
}

func configureReferences(p *ujconfig.Provider) {
	for name, refs := range references {
		p.AddResourceConfigurator(name, func(r *ujconfig.Resource) {
			for field, ref := range refs {
				r.References[field] = ref
			}
		})
	}
}

// configureImports gives the import kinds their bundle inline only. The Terraform argument
// source names a directory or zip file on the machine that runs the provider. In a provider
// pod that would only let a managed resource make the provider read the pod's own files, so
// the CRDs leave it out, and files, the export's files keyed by their path, is required.
func configureImports(p *ujconfig.Provider) {
	for _, name := range bundleNames {
		p.AddResourceConfigurator(name, func(r *ujconfig.Resource) {
			delete(r.TerraformResource.Schema, "source")
			if files, ok := r.TerraformResource.Schema["files"]; ok {
				files.Optional = false
				files.Required = true
			}
		})
	}
}

// configureEmbedding publishes the UUID of an embedded dashboard as the connection detail
// "uuid". The Superset embedded SDK needs it, so an application can read it from the connection
// secret of the DashboardEmbedding (spec.writeConnectionSecretToRef).
func configureEmbedding(p *ujconfig.Provider) {
	p.AddResourceConfigurator("superset_dashboard_embedding", func(r *ujconfig.Resource) {
		r.Sensitive.AdditionalConnectionDetailsFn = embeddingConnectionDetails
	})
}

func embeddingConnectionDetails(attr map[string]any) (map[string][]byte, error) {
	conn := map[string][]byte{}
	if uuid, ok := attr["uuid"].(string); ok && uuid != "" {
		conn["uuid"] = []byte(uuid)
	}
	return conn, nil
}
