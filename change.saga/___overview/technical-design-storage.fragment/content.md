# Storage and evolution {#storage-and-evolution}

The repository owns authored truth. `change.saga` holds the app identity, Product records, design essays, whiteboards, feature decks, tests and review history. The [ERD](/technical/erd) shows selected logical associations rather than an exhaustive file listing. There is no required database service.

## Identity and revisions {#identity-and-revisions}

A stable identity answers which story, entity, board or review is being discussed. A revision answers which definition was read. Story revisions include complete acceptance criteria and optional authored overviews; a criterion ID is never reused after removal. Lifecycle is separate: a new explanation does not silently accept a proposed story. Relations and whiteboard sources retain explicit revision pins.

Whiteboard revision parents are IDs within one named board. Story and inventory references use canonical URNs. Updating a board requires the current parent and creates a complete new document. Readers can reopen the original revision; source adoption is explicit. Concurrent heads remain a conflict until an author supplies an intentional reconciliation.

## Publication and recovery {#publication-and-recovery}

CLI authoring validates before publishing through `internal/store`. Cooperative domain locks protect mutations from simultaneous writers. File replacement and new-directory publication use staged filesystem operations. A multi-file batch prepares its entries and attempts rollback if publication fails, but it does not promise atomic visibility of the entire batch or crash-atomicity across every file. Recovery errors identify retained recovery content.

A busy writer should retry after contention; a mismatched parent requires rereading current state. Do not bypass the boundary by editing generated metadata. Git then records coherent changes with the source, retaining the reason for a design decision and allowing ordinary branch merges.

## Stored evidence and derived answers {#stored-evidence-and-derived-answers}

Code references store commit, path, selected lines and a digest of the selected bytes. Their resolution at a later source revision is derived. A diff is a comparison selected by the reader, not the permanent identity of the evidence. Coverage reports, impact projections, query pages and cache generations can be rebuilt from the Saga and Git.

The current repository contains older evidence pins. An old pin remains historical evidence; it is not repaired merely because a new overview or diagram was published. Reconcile and repin only after checking that the explanation still applies. [Return to the approach](/technical).
