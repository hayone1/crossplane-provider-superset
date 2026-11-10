---
name: session-handoff
description: Pick up work from HANDOFF.md at the start of a session and update it at the end. Use at the start of every session in crossplane-provider-superset, when the user says "continue", "pick up where you left off" or "what is the status", and before you finish any session.
---

# Session handoff

HANDOFF.md at the repository root is the memory between sessions. Any agent (or person) must be
able to continue the work from it alone.

## At the start of a session

1. Read `HANDOFF.md` completely.
2. Check that the environment it describes is still true before you rely on it:
   - The Hetzner lab server, if HANDOFF.md says one runs: `terraform -chdir=hack/lab output
     -state=../../_debug/lab/terraform.tfstate`. It costs money; it may have been destroyed.
   - The Superset in Docker: `curl -s http://localhost:8088/health` (on the machine that runs it).
   - Local clusters: `kind get clusters`, `docker ps`.
   - Tree state: `git status` and `git log --oneline -5`.
3. Tell the user in one or two sentences what the last session left open, then continue with
   the user's request. If the request conflicts with an open item, say so.

## At the end of a session

Update `HANDOFF.md` before your final message, even when the work is unfinished. Keep it short
and factual. Replace stale content instead of appending a diary.

- **Last updated**: the date (absolute, for example 2026-11-07) and a one-line summary.
- **State**: what works and how it was verified (the command and its result).
- **Environment**: the Superset in use, the lab server, clusters, what is running, what costs money, where
  secrets live (name the file and key, never paste secret values).
- **Done this session**: the changes that matter to the next person, with file paths.
- **Open items**: what is unfinished, in priority order, with enough detail to start.
- **Known problems**: bugs and quirks that are not fixed, and where they are documented.

Never write passwords, tokens, keys or other secret values into HANDOFF.md. It is committed to
git.
