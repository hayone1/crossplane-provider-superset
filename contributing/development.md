# Development

This page explains how the provider is built, how to regenerate it, and how to test it. Read
[docs-style.md](docs-style.md) before you write documentation.

## How the provider works

[Upjet](https://github.com/crossplane/upjet) generates the provider from the Superset Terraform
provider, `github.com/platacard/terraform-provider-superset`. The Terraform provider uses the
plugin framework, and this provider runs it in its own process ("no-fork"): each controller
calls the Terraform provider's Go code through the plugin protocol. The package needs no
Terraform binary.

`go.mod` decides which build of the Terraform provider ships. While fixes wait for an upstream
release, a `replace` directive points at a build with the fixes. [The Terraform provider
inside](../docs/terraform-provider.md) lists the fixes, and [HANDOFF.md](../HANDOFF.md) names their
branches. The schema (`config/schema.json`) and the documentation (`config/provider-metadata.yaml`)
come from that same build, so the CRDs always match the code that runs.

If the `replace` directive names a local folder, such as `../terraform-provider-superset`, clone
the Terraform provider there and check out the branch that HANDOFF.md names, before you build.

Most files are generated. The hand-written parts are:

| Path | Purpose |
|------|---------|
| `config/provider.go` | Builds the cluster-scoped and the namespaced provider configuration. |
| `config/external_name.go` | External names: numeric IDs, the import hashes, the empty-state check and the conversion of `id` to a string. |
| `config/resources.go` | Groups and kinds, references, and the import kinds' `files`. |
| `config/docs.go` | Rewrites Terraform wording in the CRD descriptions. |
| `internal/clients/superset.go` | Turns a ProviderConfig and its Secret into the Terraform provider's configuration. |
| `internal/clients/configcheck.go` | Validates each configuration before creates and updates, as Terraform does when it plans. |
| `apis/*/v1beta1/types.go` | The ProviderConfig types (`spec.host`). |
| `cmd/provider/main.go` | The controller manager. |
| `examples/`, `hack/*.py` | Examples and the scripts that derive some of them. |
| `test/e2e/run.sh`, `test/superset/` | The end-to-end test and its Superset. |

## Tools

* Go (the version in `go.mod`), GNU Make, Docker, kind, kubectl, jq and Python 3 with PyYAML.
* `goimports` in `PATH` for `make generate`.
* `make` downloads Terraform 1.5.7, uptest, chainsaw and the Crossplane CLI into `.cache/tools`.

The generated code is large. Building all of it needs about 4 GB of memory. On a small machine,
limit the parallel compile jobs, for example with `GOFLAGS=-p=2`.

## Regenerate the provider

```shell
make submodules    # once, for the build/ submodule
make generate
```

`make generate` builds the Terraform provider from the module that `go.mod` resolves, asks
Terraform for its schema, copies its documentation, and runs the Upjet pipeline. Run it after you
change anything in `config/` that changes the CRDs, and after you change the Terraform provider.

## Update the Terraform provider

1. Change the version of `github.com/platacard/terraform-provider-superset` in `go.mod`, or the
   `replace` directive, and run `go mod tidy`.
2. Set `TERRAFORM_PROVIDER_VERSION` in the `Makefile` to the release that the build is based on.
3. Run `make generate`. If the Terraform provider has a new resource, `make generate` fails until
   you add it to `config/external_name.go` and `config/resources.go`.
4. Run the unit tests and the end-to-end test.

Fix bugs of the Terraform provider in the Terraform provider, with a test, and propose them
upstream. Only work around a bug here if the fix cannot go upstream.

## Run the tests

### Unit tests

```shell
go test ./config/... ./internal/...
```

The unit tests check these points:

* Every resource is configured in both API variants.
* The external-name functions handle every form of an ID.
* Late initialization is off.
* The descriptions use Crossplane terms.
* Every kind has an example and a row in `docs/resources.md`.
* The credentials are parsed correctly.
* The validation wrapper rejects what Terraform rejects, and the client cache signs in once.

### End-to-end test

The end-to-end test runs the examples against a real Superset:

```shell
make superset.up                  # Superset 6.1 and PostgreSQL in Docker
cp example.env .env               # the Superset of make superset.up
test/e2e/run.sh                   # every example; or name some
test/e2e/run.sh examples/cluster/chart/chart.yaml
make superset.down
```

For each manifest, `test/e2e/run.sh` runs these steps:

1. It creates the objects and waits until they are ready.
2. It checks that the provider does not update them again, which would be an update loop.
3. It applies the update that the `# e2e-update:` line of the manifest describes, and checks that
   Superset has it.
4. It takes each object over again by its external name (the import test).
5. It deletes everything.

The script builds the provider and runs it outside a kind cluster, so a run needs no image. The
header of the script lists its options.

### Find update loops with Terraform

Most update loops come from the Terraform provider: a value that Superset reports differently
from the configuration. Terraform shows them as a plan that is never empty, which is faster to
read than the provider log. Apply a configuration of the resource with a build of the Terraform
provider (`dev_overrides`), and run `terraform plan`: it must report no changes.

### uptest

`make e2e` builds the package, deploys it to kind with Crossplane, and runs
[uptest](https://github.com/crossplane/uptest) on the examples in `UPTEST_EXAMPLE_LIST`. It reads
`UPTEST_CLOUD_CREDENTIALS` from `.env` (see `example.env`). The Superset of `make superset.up`
joins kind's network, so the provider pod reaches it at `http://superset:8088`.

```shell
make e2e UPTEST_EXAMPLE_LIST=examples/cluster/security/role.yaml,examples/namespaced/chart/chart.yaml
```

Do not test the cluster-scoped and the namespaced example of one kind together. They use the
same names in Superset, and the second create fails.

## Change an example

The examples in `examples/namespaced` and `examples/cluster/assets` are generated:

```shell
python3 hack/import-examples.py      # examples/cluster/assets from test/superset/export
python3 hack/namespaced-examples.py  # examples/namespaced from examples/cluster
```

Edit `examples/cluster` (or `test/superset/export`), run the scripts, and test both variants with
`test/e2e/run.sh`.

## Continuous integration

* `ci.yml` lints, runs the unit tests, checks that generated files are current, deploys the
  package to kind, and builds the artifacts.
* `e2e-docker.yml` runs every example against Superset 6.1 in Docker (`make superset.up`), in
  both API variants.
* `security.yml` runs govulncheck, CodeQL, gitleaks and the dependency review. `scorecard.yml`
  runs the OpenSSF Scorecard.
* `release.yml` publishes a release after CI and the end-to-end test pass for a commit on `main`.
  The version follows from the Conventional Commit subjects since the last tag
  (`hack/next-version.sh`).

## Release

Releases are automatic once the repository variable `RELEASE_ENABLED` is `true`. After CI and the
end-to-end test pass on `main`, the workflow creates a new version unless every commit since the last tag is of type
`docs`, `ci`, `chore`, `test` or `style`. A `feat` raises the minor version, a breaking change
(`type!:`) raises the major version (the minor version while the major version is 0), and anything
else raises the patch version. The workflow scans the image, pushes the package to
`ghcr.io/hayone1` and mirrors it to `xpkg.upbound.io/codejourney`, signs it with cosign, attests
the build provenance and the SBOM, and creates the GitHub release.

The mirror to Upbound needs the repository secrets `XPKG_MIRROR_ACCESS_ID` and
`XPKG_MIRROR_TOKEN`.
