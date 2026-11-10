# Getting started

In this tutorial, you connect Crossplane to Apache Superset. Then you describe a small analytics
setup as Kubernetes objects: a database connection, a dataset, a chart, a dashboard, a role and a
user. At the end, you delete everything again.

The tutorial takes about 20 minutes.

## What you need

* A Kubernetes cluster with Crossplane v2. The [Crossplane documentation](https://docs.crossplane.io/latest/get-started/install/)
  explains how to install it. For a test, a local [kind](https://kind.sigs.k8s.io) cluster is
  enough.
* `kubectl`, connected to that cluster.
* An Apache Superset 6.1 that the cluster can reach, and the username and password of an admin.
  Superset needs `FAB_ADD_SECURITY_API = True` in `superset_config.py` for the role and the user.
* A database that Superset can reach. This tutorial uses PostgreSQL with a table `public.sales`
  that has the columns `id`, `region`, `amount` and `day`.

If you do not have a Superset, start one with Docker. The repository has a setup with
Superset 6.1, a PostgreSQL and the `sales` table:

```shell
git clone https://github.com/hayone1/crossplane-provider-superset
cd crossplane-provider-superset
make superset.up
```

This Superset answers on `http://localhost:8088` with the user `admin` and the password `admin`.
This Superset also joins the Docker network of kind. So a kind cluster on the same machine reaches
it at `http://superset:8088`, and Superset reaches the PostgreSQL at `postgres:5432`.

## Step 1: Install the provider

1. Install the provider package. Replace the version with the latest release.

   ```shell
   kubectl apply -f - <<'EOF'
   apiVersion: pkg.crossplane.io/v1
   kind: Provider
   metadata:
     name: provider-superset
   spec:
     package: ghcr.io/hayone1/provider-superset:v0.1.0
   EOF
   ```

2. Wait until the provider is healthy. This takes about a minute.

   ```shell
   kubectl wait provider.pkg/provider-superset --for=condition=Healthy --timeout=5m
   ```

## Step 2: Connect the provider to Superset

1. Store the URL and the sign-in details in a Secret. Use the address under which the cluster
   reaches Superset.

   ```shell
   kubectl create secret generic superset-creds -n crossplane-system \
     --from-literal=credentials='{"host": "http://superset:8088", "username": "admin", "password": "admin"}'
   ```

2. Create a ClusterProviderConfig. Namespaced managed resources use the one named `default`
   unless they name another one.

   ```shell
   kubectl apply -f - <<'EOF'
   apiVersion: superset.m.crossplane.io/v1beta1
   kind: ClusterProviderConfig
   metadata:
     name: default
   spec:
     credentials:
       source: Secret
       secretRef: {namespace: crossplane-system, name: superset-creds, key: credentials}
   EOF
   ```

## Step 3: Describe your analytics setup

1. Create a namespace for the tutorial.

   ```shell
   kubectl create namespace analytics
   ```

2. Save the following manifest as `analytics.yaml`. Read the comments, which explain each part.

   ```yaml
   # The password of the database user. The Database below reads it from here, so it never
   # appears in the Database object.
   apiVersion: v1
   kind: Secret
   metadata:
     name: warehouse-db
     namespace: analytics
   stringData:
     password: examples
   ---
   # The connection from Superset to the database. Superset tests the connection when the
   # provider creates it, so a wrong host or password shows up as an error here.
   apiVersion: database.superset.m.crossplane.io/v1alpha1
   kind: Database
   metadata:
     name: warehouse
     namespace: analytics
   spec:
     forProvider:
       connectionName: Warehouse
       dbEngine: postgresql
       dbHost: postgres
       dbPort: 5432
       dbName: examples
       dbUser: examples
       dbPassSecretRef: {name: warehouse-db, key: password}
       exposeInSqllab: true
       allowRunAsync: false
       allowCtas: false
       allowCvas: false
       allowDml: false
   ---
   # A dataset for the table public.sales. databaseNameRef fills in the name of the
   # connection above, once Superset has it.
   apiVersion: dataset.superset.m.crossplane.io/v1alpha1
   kind: Dataset
   metadata:
     name: sales
     namespace: analytics
   spec:
     forProvider:
       databaseNameRef: {name: warehouse}
       schema: public
       tableName: sales
       metrics:
         - metricName: count
           expression: COUNT(*)
         - metricName: revenue
           expression: SUM(amount)
           d3format: ",.2f"
   ---
   # A table of the revenue per region.
   apiVersion: chart.superset.m.crossplane.io/v1alpha1
   kind: Chart
   metadata:
     name: revenue-by-region
     namespace: analytics
   spec:
     forProvider:
       name: Revenue by region
       vizType: table
       datasourceIdRef: {name: sales}
       params: |
         {"viz_type": "table", "query_mode": "aggregate", "groupby": ["region"], "metrics": ["revenue"], "row_limit": 100}
   ---
   # A role for the people who read the sales numbers, and the permissions of the role.
   apiVersion: security.superset.m.crossplane.io/v1alpha1
   kind: Role
   metadata:
     name: sales-readers
     namespace: analytics
   spec:
     forProvider:
       name: Sales readers
   ---
   apiVersion: security.superset.m.crossplane.io/v1alpha1
   kind: RolePermissions
   metadata:
     name: sales-readers
     namespace: analytics
   spec:
     forProvider:
       roleNameRef: {name: sales-readers}
       resourcePermissions:
         - {permission: can_read, viewMenu: Dashboard}
         - {permission: can_read, viewMenu: Chart}
         - {permission: menu_access, viewMenu: Dashboards}
         # The permission to read the sales dataset. viewMenuRef fills in the name that
         # Superset gives the dataset's permission.
         - permission: datasource_access
           viewMenuRef: {name: sales}
   ---
   # A dashboard. Its roles decide who may see it when Superset has the feature flag
   # DASHBOARD_RBAC; without the flag, the permissions of the role decide.
   apiVersion: dashboard.superset.m.crossplane.io/v1alpha1
   kind: Dashboard
   metadata:
     name: sales
     namespace: analytics
   spec:
     forProvider:
       title: Sales
       slug: sales
       published: true
       rolesRefs:
         - name: sales-readers
   ---
   # A user with the role. The password comes from a Secret.
   apiVersion: v1
   kind: Secret
   metadata:
     name: dana-password
     namespace: analytics
   stringData:
     password: Change-me-1234
   ---
   apiVersion: security.superset.m.crossplane.io/v1alpha1
   kind: User
   metadata:
     name: dana
     namespace: analytics
   spec:
     forProvider:
       username: dana
       firstName: Dana
       lastName: Reader
       email: dana@example.com
       passwordSecretRef: {name: dana-password, key: password}
       rolesRefs:
         - name: sales-readers
   ```

3. Apply the manifest.

   ```shell
   kubectl apply -f analytics.yaml
   ```

## Step 4: Check the result

1. Watch the managed resources until all of them are `READY` and `SYNCED`. This takes less than a
   minute. Press Ctrl+C to stop watching.

   ```shell
   kubectl get managed -n analytics --watch
   ```

   Managed resources that refer to others, such as the Chart, wait until those exist. They show
   `SYNCED False` with a message about the reference for a few seconds.

2. Open Superset and sign in. You see the database connection under **Settings > Database
   Connections**, the dataset under **Datasets**, the chart under **Charts**, and the dashboard
   under **Dashboards**.

3. Look at the IDs that Superset gave the objects:

   ```shell
   kubectl get chart.chart.superset.m.crossplane.io -n analytics revenue-by-region \
     -o jsonpath='{.status.atProvider.id}{"\n"}'
   ```

4. Change something, and watch Crossplane apply the change. For example, rename the chart:

   ```shell
   kubectl patch chart.chart.superset.m.crossplane.io -n analytics revenue-by-region \
     --type merge -p '{"spec":{"forProvider":{"name":"Revenue per region"}}}'
   ```

   A moment later, Superset shows the new name. If you rename the chart in Superset instead,
   Crossplane changes it back at its next check, which is within ten minutes by default.

The chart is not on the dashboard yet. A dashboard places its charts through its layout, which
names the charts by their IDs. [Dashboards and imports](dashboards.md) shows how to fill the
layout, and how to manage whole dashboards as exported files instead.

## Step 5: Clean up

1. Delete the manifest. Crossplane deletes the objects in Superset.

   ```shell
   kubectl delete -f analytics.yaml
   kubectl delete namespace analytics
   ```

   Superset refuses to delete a database connection while datasets use it, and a dataset while
   charts use it. Crossplane retries until the dependent objects are gone, so the deletion can
   take a minute.

2. Remove the connection to Superset and the provider.

   ```shell
   kubectl delete clusterproviderconfig.superset.m.crossplane.io default
   kubectl -n crossplane-system delete secret superset-creds
   kubectl delete provider.pkg provider-superset
   ```

3. If you started Superset with `make superset.up`, stop it and remove its data.

   ```shell
   make superset.down
   ```

## Next steps

* Browse [examples/](../examples) for a complete manifest of every kind.
* Read [Behaviour](behaviour.md) to learn how the provider finds, changes and deletes objects.
* Read [Authentication](authentication.md) to give each team its own ProviderConfig.
