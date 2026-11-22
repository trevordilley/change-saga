# Storage and evolution {#storage-and-evolution}

The repository owns authored truth. `change.saga` holds the app identity, Product records, design essays, whiteboards, feature decks, tests and review history. The [ERD](/technical/erd) shows selected logical associations rather than an exhaustive file listing. There is no required database service.

## Identity and revisions {#identity-and-revisions}

A stable identity answers which story, entity, board or review is being discussed. Current records store the editable authored definition and product state. Stable criterion IDs retain their meaning across edits. A new explanation does not silently accept a proposed story. References retain stable targets and exact content digests; source commits remain separate evidence pins.

Whiteboards store one complete current document. Publication replaces that value; relative diagram edits require its current digest. Git owns earlier versions, attribution, branches, merging and conflict resolution. Historical readers use real Git content and preserve its original pins. Legacy competing heads stop normalization without deleting content; an author explicitly preserves and combines them in Git.

## Publication and recovery {#publication-and-recovery}

CLI authoring validates before publishing through `internal/store`. Cooperative domain locks protect mutations from simultaneous writers. File replacement and new-directory publication use staged filesystem operations. A multi-file batch prepares its entries and attempts rollback if publication fails, but it does not promise atomic visibility of the entire batch or crash-atomicity across every file. Recovery errors identify retained recovery content.

A busy writer retries after contention; a stale digest on a relative edit requires rereading current content. Normalization converts exact legacy pins before atomically publishing current records, then removes obsolete revision/event metadata. Reads never persist normalization. Git records coherent documentation and source changes, preserving design reasons and ordinary branch merges.

## Stored evidence and derived answers {#stored-evidence-and-derived-answers}

Code references store commit, path, selected lines and a digest of the selected bytes. Their resolution at a later source revision is derived. A diff is a comparison selected by the reader, not the permanent identity of the evidence. Coverage reports, impact projections, query pages and cache generations can be rebuilt from the Saga and Git.

The current repository contains older evidence pins. An old pin remains historical evidence; it is not repaired merely because a new overview or diagram was published. Reconcile and repin only after checking that the explanation still applies. [Return to the approach](/technical).

## Shared feature organization {#shared-feature-organization}

[Roadmap and feature organization](/features/feature-organization) keeps immutable feature identity separate from editable planning metadata. A feature's `plan.json` stores built/planned status, optional priority, label URNs and an optional milestone URN. Features and Roadmap filter these same feature records by status and use the same reading pages. Moving between them preserves the feature URL and all associated content; delivery status is independent of requirement acceptance.

Shared label definitions live in `___labels/<id>.json`; milestone definitions live in `___milestones/<id>.json`. Each definition owns its ID and display title. Features persist references, so renaming a definition changes no feature assignment. Both catalogs use the same Classification type, strict loader and public add/rename/query path. Public mutations reject duplicate names within a kind and resolve same-Saga IDs or URNs before publishing canonical references. Missing or cross-Saga assignments fail before the plan is replaced. Legacy free-text milestones remain literal, readable values rather than silently becoming references.

Labels permit many memberships; a feature has at most one milestone. Theme records remain separate because they own an Essay, rationale deck and feature membership. Built/planned is a dedicated delivery-status field, not a user-created label catalog. This small set of distinct meanings lets the directory and sidebar reuse one grouping projection without making a milestone name behave like a delivery state.

The reader resolves titles from shared records when building presentation data and keeps the canonical plan unchanged. Features and Roadmap have separate grouping preferences; each sidebar and directory projects only its own delivery category. A multi-label feature can have several presentation rows with one canonical destination. Unassigned features remain visible, and shared definition edits invalidate the relevant outline inputs so subsequent reads see renamed titles.

Verification includes CLI reference-preservation and invalid-assignment tests, classification loader tests, grouping and navigation tests, and Chromium coverage for rename visibility, independent preferences, canonical links and promotion. The tests at implementation commit `59098190c1cf` passed. They do not implement dependency scheduling, a Gantt view or the deferred hosting work.
