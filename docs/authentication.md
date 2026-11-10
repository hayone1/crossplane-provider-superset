# Authentication

The provider signs in to Apache Superset through the Superset web API. The provider sends a
username and a password to `/api/v1/security/login`. Superset returns an access token, and the
provider uses that token for its requests. This page explains where the provider finds the
sign-in details, and which settings change how it signs in.

## The credentials

A ProviderConfig names a Secret key that holds a JSON object. The object has these keys:

| Key | Required | Meaning |
|-----|----------|---------|
| `host` | Yes, unless `spec.host` is set | The URL of Superset, for example `https://superset.example.com`. If Superset runs under a path, include the path, as in `https://example.com/superset`. The URL must start with `http://` or `https://`, and it must not have a query or a fragment. |
| `username` | Yes | The user that the provider signs in as. |
| `password` | Yes | The user's password. The provider uses it exactly as written, with spaces. |
| `auth_provider` | No | `db` (the default) for users of the Superset database, or `ldap` for an instance with `AUTH_TYPE = AUTH_LDAP`. |
| `csrf_enabled` | No | `true` (the default) makes the provider send a CSRF token with every change. A CSRF (cross-site request forgery) token proves that a change comes from a signed-in client. Set it to `false` only if your Superset runs with `WTF_CSRF_ENABLED = False`. |

The provider rejects keys that it does not know, so a typing mistake such as `user` shows up as
an error instead of being ignored.

Create the Secret with `kubectl`:

```shell
kubectl create secret generic superset-creds -n crossplane-system \
  --from-literal=credentials='{"host": "https://superset.example.com", "username": "admin", "password": "change-me"}'
```

### Keep the URL out of the Secret

You can put the URL into the ProviderConfig instead of the Secret. Then the Secret holds only
the username and the password, and the URL is visible in the ProviderConfig.

```yaml
apiVersion: superset.m.crossplane.io/v1beta1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  host: https://superset.example.com
  credentials:
    source: Secret
    secretRef: {namespace: crossplane-system, name: superset-creds, key: credentials}
```

If both places set the URL, they must be the same. Otherwise the provider reports an error.

## Which user to use

The provider can only do what its user may do. Most teams create a dedicated user with the
`Admin` role, so that every kind works. A user with fewer permissions works for the kinds that its
roles allow. For example, a user that may only manage charts and dashboards cannot create users
or database connections. Superset then answers with HTTP 403, and the managed resource shows the
message.

Users, roles, groups and permissions use the Flask-AppBuilder security API. Superset only offers
that API when `superset_config.py` has:

```python
FAB_ADD_SECURITY_API = True
```

Without it, the Role, RolePermissions, User and Group kinds fail with a message that names the
setting.

## LDAP

If your Superset authenticates users against LDAP (`AUTH_TYPE = AUTH_LDAP`), set
`auth_provider` to `ldap`:

```json
{"host": "https://superset.example.com", "username": "svc-crossplane", "password": "...", "auth_provider": "ldap"}
```

## OAuth and single sign-on

The login API of Superset accepts only `db` and `ldap`. An instance that signs in users only
through OAuth or OpenID Connect (`AUTH_TYPE = AUTH_OAUTH`) does not accept a username and a
password at `/api/v1/security/login`. To manage such an instance, run a second Superset web server
for the provider:

1. Start a second deployment of the same Superset image, with the same `SECRET_KEY` and the same
   metadata database (`SQLALCHEMY_DATABASE_URI`).
2. Set `AUTH_TYPE = AUTH_DB` in the `superset_config.py` of this deployment only.
3. Create a user for the provider in the Superset database, for example with
   `superset fab create-admin`.
4. Point the `host` of the ProviderConfig at this deployment. Do not expose the deployment to
   people.

Both deployments share every object, so the changes of the provider appear in the main Superset.

## Superset with a certificate from your own authority

The provider trusts the certificate authorities of its container image. If your Superset uses a
certificate from your own authority, give the provider pod the certificate with a
`DeploymentRuntimeConfig`. Go programs read extra certificates from the directory in
`SSL_CERT_DIR`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: company-ca
  namespace: crossplane-system
data:
  company-ca.pem: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
---
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: provider-superset
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              env:
                - name: SSL_CERT_DIR
                  value: /etc/ssl/certs:/etc/company-ca
              volumeMounts:
                - name: company-ca
                  mountPath: /etc/company-ca
                  readOnly: true
          volumes:
            - name: company-ca
              configMap:
                name: company-ca
---
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-superset
spec:
  package: ghcr.io/hayone1/provider-superset:v0.1.0
  runtimeConfigRef:
    name: provider-superset
```

## Other credential sources

A `ProviderConfig` in the group `superset.crossplane.io` and a `ClusterProviderConfig` can also
read the JSON object from the environment of the provider pod, or from a file in the pod. Set
`source: Environment` and `env: {name: SUPERSET_CREDENTIALS}`, or `source: Filesystem` and
`fs: {path: /path/to/credentials.json}`. A `DeploymentRuntimeConfig` adds the variable or mounts the
file into the pod, for example from an external secret store. The variable must not be one of
the `SUPERSET_*` variables of the Terraform provider, which the next section lists.

## One ProviderConfig per Superset

Each ProviderConfig points at one Superset as one user. To manage several Superset instances,
create one ProviderConfig for each and set `providerConfigRef` in the managed resources.

There are three kinds of provider configuration:

* `ProviderConfig` in the group `superset.crossplane.io` is for cluster-scoped managed
  resources. It may read a Secret in any namespace.
* `ClusterProviderConfig` in the group `superset.m.crossplane.io` is for namespaced managed
  resources in every namespace.
* `ProviderConfig` in the group `superset.m.crossplane.io` lives in a namespace and is for the
  namespaced managed resources of that namespace. It reads its Secret from its own namespace,
  whatever namespace its `secretRef` names. It only supports the `Secret` source, because the
  `Environment` and `Filesystem` sources would let the users of one namespace read the
  environment or the files of the provider pod.

This ProviderConfig serves the namespace `analytics`. The schema requires a namespace in
`secretRef`, but the provider always reads the Secret from the namespace of the ProviderConfig:

```yaml
apiVersion: superset.m.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: team-superset
  namespace: analytics
spec:
  credentials:
    source: Secret
    secretRef: {namespace: analytics, name: team-superset-creds, key: credentials}
```

A namespaced managed resource names its configuration with both a kind and a name:

```yaml
spec:
  providerConfigRef:
    kind: ProviderConfig      # or ClusterProviderConfig
    name: team-superset
```

Without `providerConfigRef`, a namespaced managed resource uses the `ClusterProviderConfig`
named `default`, and a cluster-scoped one uses the `ProviderConfig` named `default`.

## Environment variables

The Terraform provider reads `SUPERSET_HOST`, `SUPERSET_USERNAME`, `SUPERSET_PASSWORD` and
`SUPERSET_AUTH_PROVIDER` when its configuration leaves a setting empty. This provider always
passes every setting from the ProviderConfig, so these variables have no effect in the provider
pod. If the pod has them, the provider logs a message at start-up.

## How often the provider signs in

The provider signs in once for each ProviderConfig, and keeps using the access token for ten
minutes. Then it signs in again, and it also signs in again when Superset rejects an expired token.
A changed password or URL takes effect at once, because the provider signs in with the new values
at the next check.

Superset limits how often a client may sign in. Without this reuse, many managed resources that
the provider checks at the same moment, for example after the provider starts, would exceed the
limit and fail with HTTP 429 "Too Many Requests" until a later try. Superset also records every
sign-in as the user's last login.
