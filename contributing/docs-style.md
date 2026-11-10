# Documentation style

The provider's users range from data platform experts to people who are new to both Crossplane
and Superset. Write every page so that a ninth-grade reader can follow it. This guide applies to the
README and guides, the field descriptions that end up in the CRDs, the comments in `examples/`,
and every other Markdown file in the repository.

## Rules

1. **Write complete sentences.** Every sentence has a subject and a verb. Avoid fragments such as
   "Bootstrapping, labs." or "Defaults to X; see Y."
2. **Name the subject.** Say "the provider", "the server", "Crossplane" or "the managed resource" instead of
   "it" or "this" when the reader could guess wrong. Do not chain ideas with colons and semicolons.
   Split them into separate sentences.
3. **Keep sentences short.** Aim for 20 words or fewer. Put one idea in each sentence.
4. **Use everyday words.** Prefer "use" to "utilize", "change" to "mutate", "turn on" to "enable"
   when both work. Explain a technical term the first time a page uses it, for example
   "an M2M (machine-to-machine) application".
5. **Talk to the reader.** Use "you" and the active voice: "Set `auth_provider` to `ldap` for an
   LDAP sign-in", not "`auth_provider` may be set in order to ...".
6. **Show steps as numbered lists.** A tutorial says what the reader will build, what they need,
   the steps, how to check the result, and how to clean up.
7. **Say what happens, then what to do.** For a limitation, explain what the server does, what the
   provider does about it, and what the reader should do.

## Field descriptions

* Most descriptions come from the Terraform provider's docs, through Upjet's scraper. Fix wording
  problems there, in the Terraform repository, so both providers get the fix.
* A description that you write here starts with one sentence that says what the resource or
  field is for. Add the default and any limits as separate sentences: "The default is `true`."
* Use backticks for field names, values and API paths.

## Examples

Before:

> Pick one method. Credentials in the secret take precedence, **as a group**: a secret with
> `client_id`/`client_secret` makes the provider ignore `username`/`password`.

After:

> Choose one of these three sign-in methods, and put only its keys in the secret. For example, a
> secret for client credentials has `client_id` and `client_secret`, and no `username` or
> `password`.
