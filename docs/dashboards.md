# Dashboards and imports

You can manage dashboards in two ways:

* **Object by object.** A Dashboard managed resource holds the title, the settings and the
  layout of the dashboard, and each chart is a Chart managed resource. This fits dashboards that
  you build from parts that Crossplane also manages, for example in a Composition.
* **As exported files.** You design the dashboard in Superset, export it, and put the files of
  the export into a DashboardImport (or AssetsImport). This fits dashboards that people design
  in the Superset editor and that you want to promote from one Superset to another.

## Put charts on a Dashboard

A dashboard places its charts through its layout, `spec.forProvider.positionJson`. The layout is
a JSON document that names each chart by its ID. This example puts one chart into one row:

```json
{
  "DASHBOARD_VERSION_KEY": "v2",
  "ROOT_ID": {"type": "ROOT", "id": "ROOT_ID", "children": ["GRID_ID"]},
  "GRID_ID": {"type": "GRID", "id": "GRID_ID", "parents": ["ROOT_ID"], "children": ["ROW-1"]},
  "ROW-1": {"type": "ROW", "id": "ROW-1", "parents": ["ROOT_ID", "GRID_ID"], "children": ["CHART-1"],
            "meta": {"background": "BACKGROUND_TRANSPARENT"}},
  "CHART-1": {"type": "CHART", "id": "CHART-1", "parents": ["ROOT_ID", "GRID_ID", "ROW-1"], "children": [],
              "meta": {"chartId": 42, "width": 6, "height": 50}}
}
```

Superset gives a chart its ID when the chart is created, so a static manifest cannot know it.
Fill it in with a Composition. The Chart managed resource reports the ID in
`status.atProvider.id`. This sketch uses
[function-go-templating](https://github.com/crossplane-contrib/function-go-templating) to
create a chart and a dashboard that shows it:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: sales-dashboard
spec:
  compositeTypeRef: {apiVersion: example.org/v1alpha1, kind: SalesDashboard}
  mode: Pipeline
  pipeline:
    - step: render
      functionRef: {name: function-go-templating}
      input:
        apiVersion: gotemplating.fn.crossplane.io/v1beta1
        kind: GoTemplate
        source: Inline
        inline:
          template: |
            ---
            apiVersion: chart.superset.m.crossplane.io/v1alpha1
            kind: Chart
            metadata:
              annotations: {gotemplating.fn.crossplane.io/composition-resource-name: chart}
            spec:
              forProvider:
                name: Revenue by region
                vizType: table
                datasourceId: {{ .observed.composite.resource.spec.datasetId }}
                params: '{"viz_type": "table", "groupby": ["region"], "metrics": ["revenue"]}'
            {{- $chart := index .observed.resources "chart" }}
            {{- if and $chart $chart.resource.status.atProvider.id }}
            ---
            apiVersion: dashboard.superset.m.crossplane.io/v1alpha1
            kind: Dashboard
            metadata:
              annotations: {gotemplating.fn.crossplane.io/composition-resource-name: dashboard}
            spec:
              forProvider:
                title: Sales
                positionJson: |
                  {"DASHBOARD_VERSION_KEY": "v2",
                   "ROOT_ID": {"type": "ROOT", "id": "ROOT_ID", "children": ["GRID_ID"]},
                   "GRID_ID": {"type": "GRID", "id": "GRID_ID", "parents": ["ROOT_ID"], "children": ["ROW-1"]},
                   "ROW-1": {"type": "ROW", "id": "ROW-1", "parents": ["ROOT_ID", "GRID_ID"], "children": ["CHART-1"], "meta": {"background": "BACKGROUND_TRANSPARENT"}},
                   "CHART-1": {"type": "CHART", "id": "CHART-1", "parents": ["ROOT_ID", "GRID_ID", "ROW-1"], "children": [], "meta": {"chartId": {{ $chart.resource.status.atProvider.id }}, "width": 6, "height": 50}}}
            {{- end }}
```

The dashboard appears after the chart exists, because the template adds it only once the chart
reports its ID.

Without `positionJson`, the provider leaves the layout as Superset has it. People can then
arrange the dashboard in the Superset editor, and Crossplane does not undo their changes.

## Manage dashboards as exported files

The import kinds load a bundle of exported files into Superset:

| Kind | Imports | Deleting the managed resource deletes |
|------|---------|---------------------------------------|
| `DashboardImport` | The dashboards of the bundle with their charts, datasets and databases | The dashboards |
| `ChartImport` | The charts of the bundle with their datasets and databases | The charts |
| `DatasetImport` | The datasets of the bundle with their databases | The datasets |
| `AssetsImport` | Everything in the bundle, including saved queries, through the assets endpoint of Superset | The dashboards, charts and datasets |

The database connections of an import always stay in Superset, because other objects may use
them. The saved queries of an AssetsImport stay too.

### Step 1: Export the dashboard

1. In Superset, open **Dashboards**, select the dashboard, and choose **Export** from the
   actions menu. Superset downloads a zip file.
2. Keep the zip file. It holds one folder with `metadata.yaml` and the folders `dashboards`,
   `charts`, `datasets` and `databases`. The provider writes its own `metadata.yaml`, so the
   managed resource does not need that file.

You can also export with the API: `GET /api/v1/dashboard/export/?q=!(<id>)`.

### Step 2: Write the managed resource

The managed resource holds every YAML file of the export in `spec.forProvider.files`, keyed by its
path inside the export folder. The script `hack/export-files.py` of this repository writes the
manifest from the zip file:

```shell
python3 hack/export-files.py sales_export.zip --kind DashboardImport --name sales-dashboard \
  --namespace analytics --password-secret sales-dashboard-db > sales-dashboard.yaml
```

The export leaves out the passwords of the database connections. Give them in a Secret, keyed by
the file name of each database. The manifest then looks like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sales-dashboard-db
  namespace: analytics
stringData:
  # The key is the file name of databases/Warehouse.yaml.
  Warehouse.yaml: the-database-password
---
apiVersion: assets.superset.m.crossplane.io/v1alpha1
kind: DashboardImport
metadata:
  name: sales-dashboard
  namespace: analytics
spec:
  forProvider:
    databasePasswordsSecretRef: {name: sales-dashboard-db}
    files:
      dashboards/Sales_1.yaml: |
        dashboard_title: Sales
        ...
      charts/Revenue_by_region_1.yaml: |
        slice_name: Revenue by region
        ...
      datasets/Warehouse/sales.yaml: |
        table_name: sales
        ...
      databases/Warehouse.yaml: |
        database_name: Warehouse
        ...
```

[examples/cluster/assets](../examples/cluster/assets) has complete manifests for every import
kind. A script builds them from the export in [test/superset/export](../test/superset/export).

### What happens after the import

* The managed resource records the IDs of the imported objects in `status.atProvider`, for example
  `dashboardIds`, keyed by the UUID of each object in the export.
* The external name is a hash of the files. When you change a file, the hash changes, and the
  provider imports the bundle again. Superset overwrites the objects with the same UUID, so their
  IDs stay the same.
* If someone deletes an imported object in Superset, the provider imports the bundle again at its
  next check. Until then, the managed resource keeps the IDs of the other objects, so deleting it
  still deletes them.
* Changes that people make in the Superset editor stay until the bundle is imported again. To keep
  them, export the dashboard again and update the files.

### Things to watch

* Each object of an export has a UUID. Two import managed resources that import the same UUID
  overwrite each other's objects.
* Superset refuses to delete a dataset that charts still use. When one export feeds several import
  managed resources, delete the DashboardImport first, then the ChartImport, then the
  DatasetImport. A Crossplane `Usage` can enforce the order.
* The files are part of the managed resource, so they count against the size limits of
  Kubernetes. `kubectl apply` copies the whole manifest into an annotation, and annotations may
  hold at most 256 KB. Apply large imports with `kubectl apply --server-side`, which needs no such
  annotation. Kubernetes limits the whole object to about 1.5 MB, and the files appear twice in
  it, also in `status.atProvider`. Split a very large export into several import managed
  resources.
