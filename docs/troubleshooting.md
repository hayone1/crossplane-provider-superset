# Troubleshooting

This page explains how to find out why a managed resource of the Apache Superset provider does
not become ready. It also explains what common messages mean.

## Find the error message

1. List the managed resources and their state:

   ```shell
   kubectl get managed -A
   ```

   A healthy managed resource is `READY True` and `SYNCED True`.

2. Look at the conditions and the events of a managed resource that is not:

   ```shell
   kubectl describe chart.chart.superset.m.crossplane.io -n analytics revenue-by-region
   ```

   The `Synced` condition holds the last error. The events show what the provider tried.

3. If the message mentions an HTTP status code, the text after it is Superset's answer. It
   usually says which field Superset rejects.

## Common messages

### "cannot extract credentials"

The provider could not read the Secret that the ProviderConfig names. Check that the Secret exists,
and that `secretRef` names its namespace, name and key correctly. A namespaced ProviderConfig reads
the Secret from its own namespace.

### "invalid credentials of ProviderConfig ..." or "invalid credentials of ClusterProviderConfig ..."

The provider read the Secret, but its content is wrong. The rest of the message says why, for
example `the credentials need both username and password` or `the credentials contain unknown keys
user`. [Authentication](authentication.md) lists the keys.

### "Unable to Create Superset API Client ... authenticate with Superset"

Superset rejected the sign-in. Check these points:

* The username and the password are right. Sign in to the Superset web page with them to check.
* `auth_provider` matches how Superset authenticates: `db` or `ldap`.
* The URL is the one that the provider pod reaches. Inside a cluster, `localhost` is the pod
  itself.
* Superset accepts API sign-ins. An instance that signs users in only through OAuth does not.

### "Superset does not expose this endpoint ... FAB_ADD_SECURITY_API"

Roles, users, groups and permissions need the security API of Flask-AppBuilder. Add
`FAB_ADD_SECURITY_API = True` to `superset_config.py` and restart Superset.

### "This endpoint does not exist on this version of Superset"

Group, Theme and DatabasePermissionsSync need Superset 6.

### "cannot resolve references" or "referenced field was empty"

A `...Ref` or `...Selector` names a managed resource that does not exist yet, or whose object
Superset has not created yet. The managed resource waits and tries again. If the message stays,
check that the referenced managed resource exists in the same namespace, and that it is ready.

### "a namespaced ProviderConfig supports only the Secret credentials source"

A ProviderConfig in a namespace may only read a Secret. Use a ClusterProviderConfig for the
`Environment` and `Filesystem` sources.

### "the external name ... is not the ID of a Superset object"

The `crossplane.io/external-name` annotation must be the number of the object in Superset, such as
`"42"`. Fix the annotation, or remove it to let the provider create a new object.

### "diff contains fields that require resource replacement"

You changed a field that Superset cannot change on an existing object, such as the `databaseName`
of a Dataset. The provider does not delete and recreate the object for you. Delete the managed
resource and create it again, or change the field back.

### HTTP 403 from Superset

The user of the ProviderConfig may not do what the managed resource asks for. Give the user a role
with the permission, or use an admin.

### HTTP 422 or 400 with a message about a field

Superset rejected a value. Common causes:

* A name that another object already uses, for example a role, a tag or a database connection with
  the same name. Choose another name, or [take the object over](behaviour.md#manage-an-object-that-exists-already).
* A JSON field, such as `params` of a Chart or `jsonMetadata` of a Dashboard, that is not valid
  JSON.
* A Database whose connection test fails. Superset tests the connection with the given host,
  port, user and password, from the Superset server.

### "Database could not be created" right after Superset starts

Superset can get its first database connections at the same moment. Then Superset can try to
create the same permission twice, and it refuses one of the connections. The Superset log shows a
`UniqueViolation` on `ab_permission_name_key`. The provider tries again, and the second try
succeeds. You only see the message in the events of the managed resource.

### "Cannot delete a database that has datasets attached" and similar

Superset refuses to delete an object that other objects still use. The provider retries until the
other objects are gone. See [Delete objects in the right order](behaviour.md#delete-objects-in-the-right-order).

### "cannot determine creation result"

The provider stopped while it created an object, so it does not know whether the object exists.
[A restart during a create](behaviour.md#a-restart-during-a-create) explains what to do.

### The managed resource keeps changing the object

Two managed resources may manage the same object, or another tool changes the object back. See
[Two managed resources for the same object](behaviour.md#two-managed-resources-for-the-same-object).

## More detail in the log

The provider log shows each request that fails:

```shell
kubectl -n crossplane-system logs -l pkg.crossplane.io/provider=provider-superset --tail=200
```

For more detail, run the provider with the `--debug` flag through a `DeploymentRuntimeConfig`:

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: provider-superset-debug
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              args: ["--debug"]
```

Set `runtimeConfigRef: {name: provider-superset-debug}` in the Provider. Debug logs never show
passwords, but they show names and other values of your objects.
