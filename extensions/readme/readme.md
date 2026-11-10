# Crossplane provider for Apache Superset

> This provider is an independent community project. It is not affiliated with, sponsored or
> endorsed by the Apache Software Foundation. Apache, Apache Superset and Superset are trademarks
> or registered trademarks of the Apache Software Foundation in the United States and/or other
> countries. This page uses them only to name the product that the provider works with.

`provider-superset` manages [Apache Superset](https://superset.apache.org) with Crossplane. It
covers database connections, datasets, charts, dashboards and their embedding, users, roles,
groups, permissions, row level security, saved queries, annotations, tags, alerts and reports,
themes, and imports of exported dashboards.

[Upjet](https://github.com/crossplane/upjet) generated the provider from the
[Superset Terraform provider](https://registry.terraform.io/providers/platacard/superset/latest).
The provider runs the Terraform provider inside its own process, so it needs no Terraform
binary. It is tested with Superset 6.1.

Every managed resource comes in two API variants. The cluster-scoped variant uses the groups
`*.superset.crossplane.io`. The namespaced variant, which Crossplane v2 introduced, uses the
groups `*.superset.m.crossplane.io`.

To get started, read the
[README](https://github.com/hayone1/crossplane-provider-superset#readme) and the
[examples](https://github.com/hayone1/crossplane-provider-superset/tree/main/examples).
