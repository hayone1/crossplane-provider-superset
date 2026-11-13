# Handoff

This file is the memory between work sessions. Read it at the start of a session, and update it
before you stop (see the `session-handoff` skill). Never put secret values here.

## Last updated

2026-11-13 (third session, later): `main` is protected by the ruleset "Protect main" (id
27182339), a copy of Hostinger's. Changes go through pull requests (squash or rebase, resolved
threads, no approvals needed). The branch must be up to date, and these checks must pass: lint,
unit-tests, check-diff, local-deploy, e2e (cluster), e2e (namespaced), govulncheck, gitleaks and
codeql. No force pushes, no deletion. Push a branch and use `gh pr merge --auto --squash`.

2026-11-13 (third session): the upstream maintainer reviewed #90 to #98 on 2026-11-13. #92, #94,
#95, #97 and #98 are approved. #91, #93, #96 and #90 had golangci-lint findings in their tests
(forcetypeassert, usetesting); each now has a fix commit, and a reply is on each pull request.
#96 was rebased onto the fixed #93 (force-pushed). The fixes touch only tests, so
`crossplane-integration` and the `go.mod` pseudo-version are unchanged.

2026-11-11 (second session): publishing is on. `RELEASE_ENABLED=true`; the secrets
`XPKG_MIRROR_ACCESS_ID` and `XPKG_MIRROR_TOKEN` are the Upbound robot of the Hostinger setup, whose
team has Write on `codejourney/provider-superset`. v0.1.0 is released to
`ghcr.io/hayone1/provider-superset` and `xpkg.upbound.io/codejourney/provider-superset` (signed,
with an SBOM and attestations). Every push to `main` that passes CI and the end-to-end test makes
a release.

2026-11-10 (first session): the provider is generated, configured, documented, reviewed, and
passes the end-to-end test against Superset 6.1 in both API variants. The Hetzner lab is
destroyed.

## State

* **Generated:** Upjet v2.5.1 generates all 24 resources of the Terraform provider in both scopes.
  `make generate` from the fork reproduces the committed files exactly. Crossplane runtime and
  tool versions follow `../crossplane-provider-wso2is`.
* **Terraform provider:** `github.com/platacard/terraform-provider-superset` v1.0.0 (2026-11-08,
  the most complete Superset provider: 24 resources, plugin framework). `go.mod` replaces it with
  the branch `crossplane-integration` of the fork `github.com/hayone1/terraform-provider-superset`
  (pseudo-version of commit `e68d1dc`), which holds all nine fixes.
* **Hand-written configuration:**
  * `config/external_name.go`: numeric IDs (the `"0"` sentinel before a create, a number-aware
    external name, `idAsString` conversion for `status.atProvider.id`, `stateWithout` empty-state
    check), import kinds keyed by `content_hash`, parent IDs from the external name, sync
    operations except imports, DatasetRefresh and DatabasePermissionsSync (`runsInBackground`).
  * `config/resources.go`: groups and kinds, references (by-name references read
    `status.atProvider`), import kinds take `files` only (no `source`), DashboardEmbedding
    publishes `uuid` as a connection detail.
  * `internal/clients/configcheck.go`: runs the framework's `ValidateResourceConfig` before
    writes, fills empty required lists (`EmptiableCollections`), and caches the configured client
    per configuration for 10 minutes with singleflight (`configurecache.go`).
  * `internal/clients/superset.go`: credentials (`host`, `username`, `password`,
    `auth_provider`, `csrf_enabled`), `spec.host`, namespaced ProviderConfig rules.
* **Unit tests:** `go test ./config/... ./internal/clients/...` and `golangci-lint run ./...`
  pass (2026-11-10, on the lab).
* **End-to-end tests** (`test/e2e/run.sh`, final build, 2026-11-10): cluster scope 25/25 and
  namespaced scope 26/26 manifests (every example plus `test/e2e/getting-started.yaml`) passed:
  create, update-loop check, update verified in `status.atProvider`, import by external name,
  delete. Logs: `/root/work/e2e-*.out` on the lab.
* **In-cluster:** `make local-deploy` installed the package in kind with Crossplane 2.4.3
  (Healthy, leader lease taken) and `make uptest` passed 10 manifests (apply, import, delete).
* **Re-import:** deleting an imported dashboard in Superset made the DashboardImport import it
  again within one poll, and deleting the managed resource then deleted it (checked by hand).
* **Superset 5.0:** a Terraform apply and plan of 20 kinds (all but the Superset 6 ones) showed
  no drift.

## Upstream fixes

Pull requests to platacard/terraform-provider-superset, from branches of the fork
(local worktree `../tfp-superset-prs`):

| PR | Branch | Fix | Needs |
|----|--------|-----|-------|
| #90 | `pr/public-provider-package` | Full module path and a public `provider` package | |
| #91 | `pr/client-long-running` | 5-minute HTTP timeout; database list cache per client; no DEBUG prints | |
| #92 | `pr/inline-bundles` | `files` for the import resources; secret keys may be a file name | |
| #93 | `pr/read-missing-objects` | Role, user and role permissions that no longer exist are removed on Read | |
| #94 | `pr/dataset-cache-timeout` | Unset `cache_timeout` stays null (creating a dataset failed on Superset 6) | |
| #95 | `pr/action-triggers-in-place` | DatasetRefresh and DatabasePermissionsSync update in place on `triggers` | |
| #96 | `pr/delete-missing-objects` | Deleting objects that are already gone succeeds | #93 |
| #97 | `pr/retry-rate-limit` | Retry 429 Too Many Requests with backoff and jitter | #91 |
| #98 | `pr/import-outdated-hash` | An import keeps an `"outdated"` hash when an object is gone | #92 |

`../terraform-provider-superset` has the branch `crossplane-integration` (all fixes, on the module
path of #90, pushed to the fork) and the remote `fork`. The `feat/*` and `fix/*` branches are the
working copies that the integration branch merged; the `pr/*` branches (worktree
`../tfp-superset-prs`) are what the pull requests show. When a fix changes, update its `pr/*` branch and the
integration branch, push both, and bump the pseudo-version in `go.mod`. When upstream releases
all fixes, drop the `replace` and set `TERRAFORM_PROVIDER_VERSION` in the Makefile.

## Environment

* **Hetzner lab: destroyed** on 2026-11-10 (no servers, SSH keys or firewalls left in the project).
  `hack/lab/README.md` explains how to create it again. The token is `TF_VAR_hcloud_token` in
  `.env`. The last logs of the lab are in `_debug/lab-logs/` (git-ignored).
* The local machine (WSL) has about 5 GB of memory: build the generated code on a lab server, or
  use `GOFLAGS=-p=2`.
* GitHub repository settings match the Hostinger repository: Dependabot alerts and security
  updates (the `dependency-review` job needs them), delete branches after merge, auto-merge, no
  merge commits. The ruleset "Protect main" requires pull requests and the CI, Security and
  end-to-end checks.
* GitHub Actions: CI, Security, Scorecard and "End to End (Superset in Docker)" pass on `main`. The
  end-to-end workflow takes about an hour per API variant.

## Open items

1. Watch the upstream pull requests #90 to #98. The maintainer merges in this order: #91 then
   #97; #92 then #98; #93 then #96; #94 and #95; #90 last. **When all others are merged, rebase
   #90 onto `origin/main`** so the module path rename covers their files (it conflicts with #92 in
   `internal/provider/import_resource.go`). Check lint with the pinned golangci-lint v1.64.8
   built with Go 1.25 (`GOTOOLCHAIN=go1.25.8 go install
   github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8`; a build with Go 1.27 fails to
   type-check). Port code changes to `crossplane-integration` and bump the pseudo-version in
   `go.mod`. When a release has all fixes, drop the `replace` and set `TERRAFORM_PROVIDER_VERSION`.

## Known problems

* Superset refuses to delete objects that others use; the provider retries
  (`docs/behaviour.md`).
* A request that hangs for more than three minutes during a synchronous create leaves the managed
  resource at "cannot determine creation result" (`docs/behaviour.md`, "Requests that take long").
* A Secret that changed while the provider was stopped is not sent until it changes again (Upjet
  rebuilds the prior state from current Secrets).
* Fields that need a replacement in Terraform (for example the `databaseName` of a Dataset) are
  reported as errors; Upjet does not replace objects.
