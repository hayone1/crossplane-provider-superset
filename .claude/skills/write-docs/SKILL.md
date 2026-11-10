---
name: write-docs
description: Write or edit documentation for crossplane-provider-superset (README, guides, example comments, package metadata, field descriptions) in clear, tutorial-like, ninth-grade English. Use when adding or changing any Markdown file, example manifest comment or user-facing description.
---

# Write documentation

Follow `contributing/docs-style.md`. The short version:

1. **Complete sentences** with a subject and a verb. No fragments ("Defaults to X."), no chains
   joined by colons and semicolons.
2. **Name the subject** ("the provider", "the server", "Crossplane") instead of an unclear "it".
3. **Short sentences**, about 20 words or fewer, one idea each. Explain jargon on first use.
4. **Talk to the reader** in the active voice: "Set `spec.forProvider.name` to ...".
5. **Tutorials** say what the reader builds, what they need, numbered steps, how to check the
   result, and how to clean up.
6. **Limitations** say what the server does, what the provider does about it, and what the
   reader should do.

## Where text comes from

- Field and resource descriptions in the CRDs come from the Terraform provider's docs through
  Upjet's scraper (`config/provider-metadata.yaml`). Fix their wording in
  `../terraform-provider-superset` (its `write-docs` skill), then regenerate here.
- The README, guides, example comments and `package/crossplane.yaml` are written here.

## Checklist

- [ ] Every new or changed sentence follows the rules above.
- [ ] Examples are realistic and self-contained; comments explain *why*, in full sentences.
- [ ] Generated files are regenerated, not edited by hand.
