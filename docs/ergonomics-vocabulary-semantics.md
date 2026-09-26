# Vocabulary semantics: meaning is not implementation

A Change Saga term can describe product intent before code exists. Its revision therefore records two independent assessments:

- `definition_maturity`: `unknown`, `proposed`, or `accepted`
- `implementation_evidence`: `unknown`, `absent`, `partial`, or `present`

These fields do not replace the term's `active` or `retired` lifecycle. A term can be active with a proposed definition and absent implementation evidence, or retired with any historical combination.

`implementation_evidence` describes observed evidence availability, not implementation state. `absent` is an explicit observation of a gap. `unknown` means no assessment has been recorded, so callers must not infer either a gap or an implementation. `partial` and `present` likewise do not prove that the concept is implemented.

Exact `code` references remain separate evidence links. Their current or stale resolution is reported independently. A stale link does not rewrite the authored availability assessment, and a current link does not promote a term to implemented.

## CLI

Both complete-revision commands accept the axes independently:

```text
change-saga term add \
  --id review-annotation \
  --name "Review annotation" \
  --definition "A note pinned to an exact review Item." \
  --definition-maturity accepted \
  --implementation-evidence absent \
  --story urn:change-saga:app:story:review-annotations \
  change.saga
```

`--ref` remains optional. `term revise` accepts the same two flags because a term revision is a complete snapshot. Omitting either flag writes `unknown`. Invalid enum values are rejected without creating a package or revision.

## Query and UI

`change-saga query terms` always returns both fields. Older revisions that omit them project as `unknown`; they are never automatically declared accepted or implemented. When revision heads conflict, the query also returns both axes as `unknown`, keeps every `revision_heads` value, and omits `current_revision`.

The term directory and detail page use deliberately different wording:

- `absent` is labelled an **observed gap**;
- `unknown` is labelled **unverified** or **not assessed**;
- current and stale exact code links are shown separately from availability;
- conflicting revision heads make the current assessments unavailable until reconciled.

This preserves provenance: the authored assessment, exact Item-level links, link currency, lifecycle history, and conflict heads remain distinct facts.
