# crossplane-provider-superset

Crossplane provider for Apache Superset, generated with Upjet from the Terraform provider
`platacard/superset` (`github.com/platacard/terraform-provider-superset`; local copy:
`../terraform-provider-superset`, remote `fork` for our pull request branches).

## Every session

1. **Start:** read [HANDOFF.md](HANDOFF.md). Use the `session-handoff` skill.
2. **End:** update HANDOFF.md before you stop, even when the work is unfinished.

## Rules

- Ask before outward-facing actions that the user has not approved in this session: creating
  repositories, publishing images or packages, enabling releases, creating or destroying cloud
  servers, and changing account settings. Pushing to GitHub is approved.
- Follow `../crossplane-provider-wso2is` and `../crossplane-provider-hostinger`, the user's other
  Upjet providers, for structure, CI, release pipeline and docs.
- Keep the template's `make`-based build. Scratch files go in the git-ignored `_debug/` folder.
- Docs: clear, tutorial-like, ninth-grade English in complete sentences
  (`contributing/docs-style.md`, `write-docs` skill).
- Test against a real Superset (`make superset.up`, or the Hetzner lab in HANDOFF.md). Something is
  done only when `test/e2e/run.sh` passes for it.
- The local machine has about 5 GB of memory. Build the generated code on the lab server, or limit
  the compile jobs (`GOFLAGS=-p=2`).
- Fix Terraform provider bugs in `../terraform-provider-superset`, with a test, on a branch per
  fix, and propose them upstream. Document every bug that you cannot fix.
- Never print, commit or write secret values into HANDOFF.md. Credentials are in `.env`.

## References

- Upjet docs: https://github.com/crossplane/upjet/tree/main/docs
- Superset docs: https://superset.apache.org/docs/ (API: https://superset.apache.org/docs/api)
- Superset source: https://github.com/apache/superset
