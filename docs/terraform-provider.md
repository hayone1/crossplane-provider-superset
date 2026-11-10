# The Terraform provider inside

This provider runs the Superset Terraform provider of Plata Card
([platacard/terraform-provider-superset](https://github.com/platacard/terraform-provider-superset))
inside its own process. Crossplane calls the Go code of the Terraform provider directly, so the
managed resources behave like the Terraform resources of the same name.

The provider is built from version 1.0.0 of the Terraform provider, with the fixes below. We found
them while we tested this provider against Superset, and we proposed each one to Plata Card. When a
release of the Terraform provider contains all of them, this provider will use that release.

| Fix | What changes | Proposal |
|-----|--------------|----------|
| A public Go package | Other programs can embed the Terraform provider. This provider needs it. | [#90](https://github.com/platacard/terraform-provider-superset/pull/90) |
| A time limit for requests, and the database list of each client | A request to a Superset that does not answer fails after five minutes, instead of waiting forever. Two Superset instances no longer share a cached list of database connections, and a new database connection is found at once. | [#91](https://github.com/platacard/terraform-provider-superset/pull/91) |
| Inline bundles | The import kinds take their files in the managed resource (`files`), and the passwords of a bundle may be keyed by file name. | [#92](https://github.com/platacard/terraform-provider-superset/pull/92) |
| Objects that no longer exist | A Role, User or RolePermissions whose object was deleted in Superset is created again, instead of failing. | [#93](https://github.com/platacard/terraform-provider-superset/pull/93) |
| The cache timeout of a dataset | A Dataset without `cacheTimeout` no longer changes Superset again and again. | [#94](https://github.com/platacard/terraform-provider-superset/pull/94) |
| Actions with `triggers` | DatasetRefresh and DatabasePermissionsSync run again when `triggers` changes. Before, the change needed a replacement, which Crossplane does not do. | [#95](https://github.com/platacard/terraform-provider-superset/pull/95) |
| Deleting objects that are gone | Deleting a Role, User, MetaDatabase or RolePermissions whose object is already gone succeeds. | [#96](https://github.com/platacard/terraform-provider-superset/pull/96) |
| Rate limits | A request that Superset answers with "429 Too Many Requests" is sent again after a pause. | [#97](https://github.com/platacard/terraform-provider-superset/pull/97) |
| Imports with a deleted object | An import whose dashboard, chart or dataset was deleted in Superset imports the bundle again, and still deletes its other objects when you delete it. | [#98](https://github.com/platacard/terraform-provider-superset/pull/98) |

`go.mod` builds the branch `crossplane-integration` of the fork
[hayone1/terraform-provider-superset](https://github.com/hayone1/terraform-provider-superset),
which holds all of these fixes.
