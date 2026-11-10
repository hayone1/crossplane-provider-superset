# Managed resources

The provider for Apache Superset has 24 kinds. Each kind exists in two API variants:

* cluster-scoped, in the group `<group>.superset.crossplane.io`, and
* namespaced, in the group `<group>.superset.m.crossplane.io`.

Both variants have the same `spec.forProvider` fields. A cluster-scoped managed resource also has
`spec.deletionPolicy`, and its references to Secrets name a namespace. A namespaced managed
resource reads Secrets from its own namespace.

The version of every kind is `v1alpha1`. The CRD of each kind describes its fields: run
`kubectl explain <kind>.<group>.superset.crossplane.io.spec.forProvider`, for example
`kubectl explain datasets.dataset.superset.crossplane.io.spec.forProvider`. The
[Terraform documentation](https://registry.terraform.io/providers/platacard/superset/latest/docs)
of each resource has the same fields in snake case.

## Kinds

The external name is the value of the `crossplane.io/external-name` annotation. It identifies
the object in Superset. The provider sets it after it creates an object. To manage an object
that exists already, create the managed resource with this annotation (see
[Behaviour](behaviour.md#manage-an-object-that-exists-already)).

| Kind | Group | Terraform resource | External name | Notes |
|------|-------|--------------------|---------------|-------|
| `Role` | `security` | `superset_role` | The role's ID | |
| `RolePermissions` | `security` | `superset_role_permissions` | The role's ID | Owns every permission of the role. Needs `FAB_ADD_SECURITY_API`. |
| `User` | `security` | `superset_user` | The user's ID | The password comes from a Secret. |
| `Group` | `security` | `superset_group` | The group's ID | Superset 6. |
| `RowLevelSecurity` | `security` | `superset_row_level_security` | The rule's ID | |
| `Database` | `database` | `superset_database` | The connection's ID | Superset tests the connection when the provider creates or changes it. |
| `MetaDatabase` | `database` | `superset_meta_database` | The connection's ID | Needs the feature flag `ENABLE_SUPERSET_META_DB`. Takes over a meta database with the same name. |
| `DatabasePermissionsSync` | `database` | `superset_database_permissions_sync` | The connection's ID | Superset 6. An action: runs again when `triggers` changes. |
| `Dataset` | `dataset` | `superset_dataset` | The dataset's ID | |
| `DatasetRefresh` | `dataset` | `superset_dataset_refresh` | The dataset's ID | An action: runs again when `triggers` changes. |
| `Chart` | `chart` | `superset_chart` | The chart's ID | |
| `Dashboard` | `dashboard` | `superset_dashboard` | The dashboard's ID | See [Dashboards and imports](dashboards.md). |
| `DashboardEmbedding` | `dashboard` | `superset_dashboard_embedding` | The dashboard's ID | Takes over the embedding of its dashboard. |
| `CSSTemplate` | `dashboard` | `superset_css_template` | The template's ID | |
| `Theme` | `theme` | `superset_theme` | The theme's ID | Superset 6. |
| `SavedQuery` | `sqllab` | `superset_saved_query` | The query's ID | |
| `AnnotationLayer` | `annotation` | `superset_annotation_layer` | The layer's ID | |
| `Annotation` | `annotation` | `superset_annotation` | The annotation's ID | |
| `Tag` | `tag` | `superset_tag` | The tag's ID | Needs the feature flag `TAGGING_SYSTEM`. |
| `Report` | `report` | `superset_report` | The report's ID | Needs the feature flag `ALERT_REPORTS`. |
| `AssetsImport` | `assets` | `superset_assets_import` | The hash of the bundle | Imports a whole export. |
| `DashboardImport` | `assets` | `superset_dashboard_import` | The hash of the bundle | |
| `ChartImport` | `assets` | `superset_chart_import` | The hash of the bundle | |
| `DatasetImport` | `assets` | `superset_dataset_import` | The hash of the bundle | |

## References

A reference fills a field from another managed resource, so you do not need to know the ID or
the name of its object. Each field below has a `...Ref` field that names a managed resource
(`...Refs` for a list), and a `...Selector` field that selects one by its labels. For example,
`datasourceIdRef: {name: sales}` fills `datasourceId` with the ID of the Dataset `sales`. By
default, a namespaced managed resource refers to managed resources in its own namespace.

A reference resolves only after the referenced object exists in Superset. Until then, the
managed resource waits and reports that the reference is not ready.

| Kind | Field | Refers to | Value |
|------|-------|-----------|-------|
| `RolePermissions` | `roleName` | `Role` | The role's name |
| `RolePermissions` | `resourcePermissions[].viewMenu` | `Dataset` | The `perm` of the dataset, such as `[Warehouse].[sales](id:12)`. Superset calls the object of a permission a view menu. |
| `User` | `roles` | `Role` | The role IDs |
| `Group` | `roles`, `users` | `Role`, `User` | The IDs |
| `RowLevelSecurity` | `roles`, `tables` | `Role`, `Dataset` | The IDs |
| `MetaDatabase` | `allowedDatabases` | `Database` | The connection names |
| `DatabasePermissionsSync` | `databaseId` | `Database` | The ID |
| `Dataset` | `databaseName` | `Database` | The connection name |
| `Dataset` | `owners` | `User` | The IDs |
| `DatasetRefresh` | `datasetId` | `Dataset` | The ID |
| `Chart` | `datasourceId` | `Dataset` | The ID |
| `Chart` | `owners`, `tags` | `User`, `Tag` | The user IDs, the tag names |
| `Dashboard` | `owners`, `roles`, `themeId`, `tags` | `User`, `Role`, `Theme`, `Tag` | The IDs, the tag names |
| `DashboardEmbedding` | `dashboardId` | `Dashboard` | The ID |
| `SavedQuery` | `databaseId` | `Database` | The ID |
| `Annotation` | `layerId` | `AnnotationLayer` | The ID |
| `Report` | `chartId`, `dashboardId`, `owners` | `Chart`, `Dashboard`, `User` | The IDs |
| `Report` | `databaseName` | `Database` | The connection name |

## Secrets

Fields that hold secrets come from Kubernetes Secrets. Their names end in `SecretRef`. They
never appear in the managed resource, and the provider does not write them into
`status.atProvider`.

| Kind | Field | Holds |
|------|-------|-------|
| `User` | `passwordSecretRef` | The user's password |
| `Database` | `dbPassSecretRef` | The password of the database user |
| `Database` | `sqlalchemyUriSecretRef` | The whole SQLAlchemy URI, which holds the password |
| `Database` | `maskedEncryptedExtraSecretRef` | Secret connection settings as a JSON document, such as a service account key |
| `Database` | `sshTunnel.passwordSecretRef`, `sshTunnel.privateKeySecretRef`, `sshTunnel.privateKeyPasswordSecretRef` | The sign-in details of the SSH tunnel |
| Import kinds | `databasePasswordsSecretRef` | One key per database file of the bundle, such as `warehouse.yaml`, with its password |
| Import kinds | `sshTunnelPasswordsSecretRef`, `sshTunnelPrivateKeysSecretRef`, `sshTunnelPrivateKeyPasswordsSecretRef` | The same for SSH tunnels |

A `...SecretRef` of a single value names a Secret and a key. A `...SecretRef` of an import kind
names a whole Secret: every key of the Secret becomes one entry.

## Connection details

A DashboardEmbedding writes the UUID of the embedding into its connection secret, under the key
`uuid`, when you set `spec.writeConnectionSecretToRef`. The Superset embedded SDK needs this UUID,
so an application can read it from the Secret. The other kinds write no connection details.
