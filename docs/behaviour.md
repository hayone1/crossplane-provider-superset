# Behaviour

This page explains how the provider finds, changes and deletes objects in Apache Superset. It
also describes the limits that come from Superset. Read it before you manage a Superset that
people also change by hand.

## Supported Superset versions

We test the provider with Superset 6.1. Every example passes the end-to-end test in both API
variants.

We also tested every kind against Superset 5.0, through the Terraform provider that this provider
runs. Every kind except Group, Theme and DatabasePermissionsSync created, read and changed its
objects without unexpected differences. These three kinds use endpoints that only Superset 6 has.
On older versions, they fail with a message that says so.

## Feature flags

Some kinds need a feature flag or another setting in `superset_config.py`:

| Kind or field | Needs |
|---------------|-------|
| Role, RolePermissions, User, Group | `FAB_ADD_SECURITY_API = True` |
| Tag, and the `tags` of Chart and Dashboard | `FEATURE_FLAGS = {"TAGGING_SYSTEM": True}` |
| Report | `FEATURE_FLAGS = {"ALERT_REPORTS": True}`. Superset sends reports only when its Celery workers and beat run. Email reports also need SMTP settings. |
| DashboardEmbedding | `FEATURE_FLAGS = {"EMBEDDED_SUPERSET": True}` for guest tokens |
| The `roles` of a Dashboard | `FEATURE_FLAGS = {"DASHBOARD_RBAC": True}`. Without the flag, Superset stores the roles but does not use them to grant access. |
| The `themeId` of a Dashboard | Superset 6 |
| MetaDatabase | `FEATURE_FLAGS = {"ENABLE_SUPERSET_META_DB": True}` |
| The `sshTunnel` of a Database | `FEATURE_FLAGS = {"SSH_TUNNELING": True}` |
| Jinja templates in datasets and saved queries | `FEATURE_FLAGS = {"ENABLE_TEMPLATE_PROCESSING": True}` |

## How a managed resource finds its object

Superset gives every object a number when it creates the object. The provider records the number
in the `crossplane.io/external-name` annotation of the managed resource, and in
`status.atProvider.id`. From then on, the managed resource manages the object with that number,
even when you rename the object.

Before the first create, the external name is empty, and the provider creates a new object. Some
objects need a unique name, for example a role or a database connection. If Superset already has
an object with that name, the create fails, and the managed resource shows the message of
Superset. Then decide whether to take the object over (see below) or to choose another name.

A few kinds behave differently:

* **DashboardEmbedding** uses the ID of its dashboard. If the dashboard is already embeddable,
  the managed resource takes the embedding over and sets its allowed domains.
* **RolePermissions** uses the ID of its role. The managed resource takes over all permissions of
  the role that `roleName` names.
* **MetaDatabase** takes over a meta database with the same `databaseName`, if one exists.
* **DatasetRefresh** and **DatabasePermissionsSync** are actions, not objects. Their external
  name is the ID of the dataset or the database connection. They run when you create them. They
  run again whenever you change `triggers`, for example to `triggers: {version: "2"}`.
* **The import kinds** use a hash of their files as the external name. See
  [Dashboards and imports](dashboards.md).

### Find the ID of an object

The Superset API shows the ID of every object. For example, this request finds the ID of the
database connection named `Warehouse`:

```shell
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://superset.example.com/api/v1/database/?q=(filters:!((col:database_name,opr:eq,value:Warehouse)))" \
  | jq '.result[] | {id, database_name}'
```

To get the token, send `POST /api/v1/security/login` with the body
`{"username": "...", "password": "...", "provider": "db"}`. The other lists work the same way, for
example `/api/v1/chart/`, `/api/v1/dashboard/`, `/api/v1/dataset/` and `/api/v1/security/roles/`.
Some web pages also show the ID. For example, Superset opens a chart at `/explore/?slice_id=42`.

### Manage an object that exists already

To bring an object that people created in Superset under the management of Crossplane, first
observe the object. Then take it over:

1. Create the managed resource with the external name and the management policy `Observe`. The
   provider then only reads the object, so `spec.forProvider` may be empty.

   ```yaml
   apiVersion: chart.superset.m.crossplane.io/v1alpha1
   kind: Chart
   metadata:
     name: revenue
     namespace: analytics
     annotations:
       crossplane.io/external-name: "42"
   spec:
     managementPolicies: ["Observe"]
     forProvider: {}
   ```

2. Wait until the managed resource is `READY`. Then look at `status.atProvider`, which holds the
   current values of the object.

3. Copy every value of `status.atProvider` that the object should keep into `spec.forProvider`,
   and remove `managementPolicies`. The provider then changes the object to match
   `spec.forProvider`. Most fields that you leave out become empty in Superset (see the next
   section), so copy all of them except `id` and the other values that Superset computes.

Compare the values carefully before you remove `managementPolicies`. For example, the `params` of
a Chart replace the whole configuration of the chart, and a RowLevelSecurity without `roles`
applies to no role.

Observing an object needs only the external name, with these exceptions:

* A RolePermissions needs `roleName`, because the provider finds the role by its name.
* A SQLExecution needs `databaseId` (or `databaseIdRef`). With only `databaseName`, the provider
  cannot find the connection, and it runs the statement again.
* An import kind cannot observe objects. It imports its bundle again, which overwrites the objects
  with the same UUIDs, and then manages them.

## Settings that you leave out

The provider makes the object match `spec.forProvider`, as Terraform makes an object match its
configuration. A field that you leave out does one of three things:

1. **Superset keeps its value.** This is true only for fields that Superset fills in itself, and
   for links that you can set from either side (see below). People can then change these fields
   in Superset, and the provider leaves the change alone:

   | Kind | Fields |
   |------|--------|
   | Chart | `dashboards`, `owners`, `queryContext`, `tags` |
   | Dashboard | `jsonMetadata`, `owners`, `positionJson`, `tags` |
   | Database | `cacheTimeout`, `extra` |
   | Dataset | `catalog`, `columns`, `mainDttmCol`, `metrics`, `owners` |
   | Group | `users` |
   | Report | `gracePeriod`, `logRetention`, `owners`, `reportFormat`, `timezone`, `workingTimeout` |
   | User | `groups` |

2. **The field gets its default.** The description of such a field in the CRD names the default,
   for example `published` of a Dashboard or `active` of a User. Run
   `kubectl explain <kind>.<group>.superset.crossplane.io.spec.forProvider.<field>` to read it.
3. **The field becomes empty in Superset.** This is true for every other field. For example, a
   RowLevelSecurity without `roles` applies to no role.

So describe every value that matters to you in `spec.forProvider`. The provider does not copy the
values of Superset into `spec.forProvider` (Crossplane calls this late initialization, and the
provider turns it off). Look at `status.atProvider` for the values that Superset has.

Some fields own a whole list once you set them:

* The `columns` and `metrics` of a Dataset. With `columns`, the dataset has exactly these columns.
  Within a column, the attributes that you leave out keep the values of Superset, so the type that
  Superset detects stays.
* The `resourcePermissions` of a RolePermissions. The role has exactly these permissions, and the
  provider removes any other permission of the role.
* The `roles` of a User. A User needs `roles` or `rolesRefs`.

Some links can be set from either side. Set each one from one side only, or the two managed
resources keep changing it back:

* The members of a group: the `users` of a Group, or the `groups` of a User.
* The dashboards of a chart: the `positionJson` of a Dashboard, which also places the chart on the
  dashboard, or the `dashboards` of a Chart, which only links it. A linked chart without a place in
  the layout is listed on the dashboard but not shown.

An empty list, such as `users: []`, is the same as leaving the field out, so Superset keeps the
links. To remove all members of a group, remove them in Superset, or set `users` on the Group to
manage them there.

`spec.initProvider` does not help here. For these kinds, the provider applies `spec.initProvider`
at every check, not only when it creates the object, so it behaves like `spec.forProvider`.

## Changes outside Crossplane

The provider checks every managed resource at its poll interval, which is ten minutes by default.
When an object in Superset differs from `spec.forProvider`, the provider changes the object back.
When an object no longer exists, the provider creates it again with a new ID.

## Changes that need a new object

Some fields cannot change on an existing object:

* the `username` of a User,
* the `layerId` of an Annotation,
* the `dashboardId` of a DashboardEmbedding,
* the `datasetId` of a DatasetRefresh,
* the `databaseId` of a DatabasePermissionsSync, and
* the `sql`, `databaseName`, `databaseId`, `schema` and `catalog` of a SQLExecution.

A new `databaseName` of a Dataset does not need a new object: Superset moves the dataset to the
other connection, and the dataset keeps its ID and its charts.

Terraform replaces the object in this case. The provider does not delete and recreate objects for
you. It reports the change as an error instead. To make the change, delete the managed resource,
and create a new one.

## Deleting a managed resource

When you delete a managed resource, the provider deletes its object in Superset. These kinds
behave differently:

* **RolePermissions** removes all permissions of the role. The role stays.
* **DatasetRefresh** and **DatabasePermissionsSync** do nothing. The dataset or the connection
  stays as it is.
* **DashboardEmbedding** turns embedding off for the dashboard.
* **SQLExecution** runs its `destroySql`, for example `DROP VIEW IF EXISTS daily_sales`. Without
  `destroySql`, or when the connection no longer exists, it does nothing.
* **The import kinds** delete the imported objects of their kind, as
  [Dashboards and imports](dashboards.md) describes. Database connections stay.

To keep the object in Superset when you delete the managed resource:

* For a cluster-scoped managed resource, set `spec.deletionPolicy: Orphan`.
* For a namespaced managed resource, set
  `spec.managementPolicies: ["Observe", "Create", "Update", "LateInitialize"]`. These policies
  leave out `Delete`. Namespaced managed resources have no `deletionPolicy` field.

### Delete objects in the right order

Superset refuses to delete some objects while other objects use them:

* a database connection that datasets use,
* a dataset that charts use, and
* a chart or a dashboard that a report uses.

If you delete all managed resources at once, the provider retries the refused deletes. The
deletes succeed once the dependent objects are gone, which can take a minute. To make Crossplane
delete in order, add a [Usage](https://docs.crossplane.io/latest/managed-resources/usages/). With
`replayDeletion: true`, Crossplane deletes the used object as soon as nothing uses it any more:

```yaml
apiVersion: protection.crossplane.io/v1beta1
kind: Usage
metadata:
  name: sales-uses-warehouse
  namespace: analytics
spec:
  replayDeletion: true
  of:
    apiVersion: database.superset.m.crossplane.io/v1alpha1
    kind: Database
    resourceRef: {name: warehouse}
  by:
    apiVersion: dataset.superset.m.crossplane.io/v1alpha1
    kind: Dataset
    resourceRef: {name: sales}
```

## Secrets

Passwords and other secrets come from Kubernetes Secrets, through the fields that end in
`SecretRef`. The provider reads the Secret each time it checks the managed resource. When the
Secret changes, the provider sends the new value to Superset.

Superset never returns passwords. So the provider cannot see whether someone changed a password
in Superset. The provider only sends a password again when the Secret changes.

If a referenced Secret does not exist yet, the provider sends an empty value, and Superset usually
rejects it. Create Secrets before the managed resources that use them, or in the same manifest.

After the provider restarts, the provider rebuilds what it knows about each object from the
current Secrets. A Secret that changed while the provider was stopped therefore looks unchanged.
So the provider does not send the new value. To send it, change the Secret once more while the
provider runs.

## SQL statements

A SQLExecution runs its `sql` through SQL Lab when the provider creates it, and again whenever
`triggers` changes. Its external name is the ID of the last run in the query history of SQL Lab.
After the run, the provider only checks that the database connection still exists. To take
over a run with the external name, set `databaseId` (or `databaseIdRef`), not only `databaseName`. It does not
check what the statement made, so a view that someone drops in Superset stays dropped until
`triggers` changes. If the connection is deleted and created again, the provider runs the
statement again on the new connection.

The connection must be exposed in SQL Lab (`exposeInSqllab`). For a statement that changes data
or structure, it must also allow DML (`allowDml`).

Write statements that can run twice without harm, such as `CREATE OR REPLACE VIEW` or
`CREATE TABLE IF NOT EXISTS`. A statement runs while the reconcile waits, and Crossplane stops a
reconcile after three minutes (see the next section). If Superset finishes a slow statement after
that, the provider can run it again. Keep long work out of a SQLExecution, for example in a
scheduled job of the database.

## Which values the provider checks

Before each create and each change, the provider runs the same checks that Terraform runs when it
plans. For example, the `filterType` of a RowLevelSecurity must be `Regular` or `Base`. A managed
resource that fails a check shows the message and does not change Superset. Superset checks
everything else, for example SQL and JSON documents. The managed resource then shows the message
of Superset.

## Requests that take long

Crossplane handles each managed resource in a reconcile: one round of reading the object and
changing it. Crossplane stops a reconcile after three minutes. Most kinds create, change and
delete their objects inside the reconcile, and Superset answers these requests in seconds. Even a
database connection test ends after 30 seconds, because Superset stops the test then
(`TEST_DATABASE_CONNECTION_TIMEOUT`).

The import kinds, DatasetRefresh and DatabasePermissionsSync can take longer. So they run in the
background, for up to one hour.

If Superset does not answer at all, the provider gives up a request after five minutes. When this
happens during a create, the reconcile has already ended. Crossplane then cannot tell whether the
object exists, and it stops with `cannot determine creation result`. See the next section.

If Superset answers "429 Too Many Requests", the provider waits and tries again, up to four times.
Superset allows each client address 50 requests a second by default (`RATELIMIT_APPLICATION`).
Every reconcile sends a few requests. If you manage many objects, raise the limit for the
provider, or lower the `--max-reconcile-rate` of the provider (10 a second by default).

## A restart during a create

The provider creates an object, and then records its ID in the external name. If the provider
stops between these two steps, the managed resource does not know the object. Crossplane notices
this and stops with the condition `ReconcileError` and the message `cannot determine creation
result`. To continue:

1. Look in Superset for the object.
2. If the object exists, set the external name to its ID. Otherwise, do nothing.
3. Remove the annotation `crossplane.io/external-create-pending` from the managed resource.

The provider then continues and does not create a second object.

## Run one provider replica

Run the provider with one replica. Leader election is on by default, so a second pod waits until
the first pod stops, for example during an upgrade. Two pods that create the same object at the
same time would create it twice, because Superset assigns the IDs.

## Two managed resources for the same object

Do not manage one object with two managed resources. Examples are two DashboardEmbeddings for one
dashboard, or two RolePermissions for one role. Each managed resource changes the object back to
its own values, so they keep overwriting each other.
