# Native checkpoints with Git-owned content history

Checkpoints originated on `plan-live-activity`: `c02849f0` proposed checkpoint
review on 2026-11-05; `987f8a1d` made a checkpoint a phase that same day;
`8dd33a66` added expectations and recorded confirmations on 2026-11-07.
These commits follow the v0.8.0 release and were absent from the Git-owned-history
baseline. The first compatibility build, `ce8d796d`, intentionally excluded that
unmerged feature. Rebuilding it did not add the missing implementation.

This integration ports the checkpoint model, schema, authoring commands, readiness,
reports, query fields and reader/review views. It keeps current-record storage
and strict JSON decoding. It does not import live-activity logs or their polling
service. Unknown nested checkpoint fields and null checkpoints are rejected.
`doctor` now reports both `plans.checkpoints: true` and `saga.current_records: true`.

A checkpoint is an execution/review gate, not a version of documentation.
Its expectations and actual pass judgment remain in the current Plan. A widened
review boundary must be a full immutable Git commit, never a movable branch
name. Reads,
validation, normalization and slide regeneration never pass it. The default
passer is a person; configuring an agent passer is explicit. The tool's free-text
attribution is not an IAM approval mechanism or an agreement about support access.
The support-access proposal and its unresolved owners remain separate.

## Resolving a legacy whiteboard without restoring revision graphs

PR #266 in `twentyideas/daylight`, at
`16039967950a2131180b3ddc01ca9d6e0aeaa37b`, contains the reported confidentiality
Plan. Once its checkpoint fields could be read, full validation exposed a legacy
whiteboard fork in `hosted-erd-draft`:

- `r-candidate-4ff27-20261113` explains physical storage and transformations,
  including candidate design and its explicit deployment-evidence limitations.
- `r-source-adoption-20261112` explains conceptual relationships and includes
  structured links to exact source definitions.

Neither timestamp nor a preferred parent settles their authored content.
The repair evaluates and combines both views into one current document. Code
evidence and source pins retain their exact meanings; changed prose is not a
reason to refresh an old pin. Shared element names with different meanings are
disambiguated explicitly; earlier identities and values remain available in Git.

Ordinary reads and automatic normalization still refuse an unresolved legacy
fork. An author can publish a complete evaluated replacement through the existing
`whiteboard publish` command. Before deleting competing legacy files, the writer
checks that Git's committed bytes retain all of them. It then converts exact pins,
converts references in other legacy packages before their source files disappear,
publishes one `current.json`, and removes the legacy directory. No parent IDs,
head-selection flags or parallel content-history store are introduced.

The regression `TestExplicitWhiteboardEditResolvesLegacyForkWithHistoryOnlyInGit`
tests that an ordinary read refuses to guess, an uncommitted fork is preserved,
an explicit composed edit succeeds after committing the inputs, source pins stay
exact, and both earlier values can be loaded by digest from Git after all Saga
revision files have been removed. This exercises the intended parent refactor
directly rather than reinstating the storage it removed.

Testing against an isolated PR copy must leave both security checkpoints unpassed.
Any repair stays on its own local checkout for review; it is not authority to
deploy, grant access or admit customer data.

The isolated PR repair combines 118 physical-view and 187 conceptual-view elements
with two explanatory headings. It disambiguates 13 colliding names, retains all
149 exact code-evidence references and 51 structured source links, and removes
17 whiteboard revision files. Full Saga validation succeeds, retaining 359
warnings; stale evidence and deployment limitations remain visible. The Plan
manifest is byte-for-byte unchanged and both gates remain unpassed. Browser
inspection confirms the combined board and its section navigation render.

This is a scoped authored repair, not a claim that every legacy package in that
repository can be automatically normalized. An unrelated component fork still
requires its own authored evaluation before whole-Saga normalization. Earlier
whiteboard content and exact pins are recoverable from the committed PR data in
Git; no timestamp-based head choice or pin refresh is part of this repair.

After committing the actual repair, the reader also retrieved both former
whiteboard values by digest from Git alone: the 118-element physical view
(`sha256:662dd3da32c06c9989c5b87521f475e760588b334ddfd479f763bcf1086e942e`)
and the 187-element conceptual view
(`sha256:14a0770b315d19a0e9cd16a2664ac8add6b63857f7627855fc6a5ef879d544af`).
Their former files are absent from the working Saga. This checks the actual
repair as well as the smaller regression fixture.
