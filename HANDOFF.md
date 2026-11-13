# Handoff

This file is the memory between work sessions. Read it at the start of a session, and update it
before you stop (see the `session-handoff` skill). Never put secret values here.

## Last updated

2026-11-13 (fourth session): upstream merged #91 to #98, plus its own features #99 to #105
(SQL execution, user groups, chart dashboards, managed externally + uuid, more dataset settings,
two data sources). #90 was rebased onto upstream main (`199767d`, force-pushed, CI green) and
waits for the merge. This project now builds that commit (`go.mod` replace to the fork,
branch `pr/public-provider-package`), adds the kind SQLExecution (`sqllab`), the references
User.groups and Chart.dashboards, and docs for the new behaviour. Pull request on branch
`feat/upstream-main`. The branch `crossplane-integration` is no longer used.

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

* **Generated:** Upjet v2.5.1 generates all 25 resources of the Terraform provider in both scopes.
  `make generate` from the fork reproduces the committed files exactly. Crossplane runtime and
  tool versions follow `../crossplane-provider-wso2is`.
* **Terraform provider:** `github.com/platacard/terraform-provider-superset`, `main` after v1.0.0
  (25 resources, plugin framework). Until upstream merges #90 (module path + public `provider`
  package), `go.mod` replaces it with the fork `github.com/hayone1/terraform-provider-superset` at
  `199767d` (branch `pr/public-provider-package` = upstream main + #90).
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

All nine are merged upstream except #90 (2026-11-13). `../terraform-provider-superset` has the
remote `fork`; the `pr/*` branches (worktree `../tfp-superset-prs`) are what the pull requests
show. `crossplane-integration`, `feat/*` and `fix/*` are obsolete.

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

1. When upstream merges #90: replace the `replace` in `go.mod` by a pseudo-version of the upstream
   merge commit (`go get github.com/platacard/terraform-provider-superset@<sha>`), run
   `make generate`, and update `docs/terraform-provider.md` (drop "waiting to be merged" and the
   fork paragraph). When upstream releases a version with everything, require that version and
   set `TERRAFORM_PROVIDER_VERSION` in the Makefile. Lint upstream changes with golangci-lint
   v1.64.8 built with Go 1.25 (`GOTOOLCHAIN=go1.25.8 go install
   github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8`; a Go 1.27 build fails to
   type-check).

## Known problems

* Superset refuses to delete objects that others use; the provider retries
  (`docs/behaviour.md`).
* A request that hangs for more than three minutes during a synchronous create leaves the managed
  resource at "cannot determine creation result" (`docs/behaviour.md`, "Requests that take long").
* A Secret that changed while the provider was stopped is not sent until it changes again (Upjet
  rebuilds the prior state from current Secrets).
* Fields that need a replacement in Terraform (for example the `databaseName` of a Dataset) are
  reported as errors; Upjet does not replace objects.
