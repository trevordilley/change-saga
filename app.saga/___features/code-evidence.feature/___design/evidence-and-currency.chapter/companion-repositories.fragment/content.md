# Companion repository evidence {#companion-repository-evidence}

A companion Saga and an in-repository Saga present the same evidence model. The
only additional context is that documentation Git history and source Git
history are separate, so every source operation names and verifies the selected
checkout.

## Verify source identity before reading {#source-identity}

Opening, querying, comparing, or repairing source evidence first compares the
selected checkout’s canonical repository identity with the repository declared
by the Saga. A mismatch blocks source-derived results before any mutation and
shows declared and observed identities without exposing credentials.

Once verified, every source link, digest check, remap, and stale diagnosis uses
that checkout. The UI keeps a persistent **Source checkout** indicator so a
reader cannot confuse the companion repository’s files with the documented
codebase.

## Sync state is explicit {#sync-state}

The companion header shows **Documented source revision** as a full source
commit plus one of: aligned with selected source, selected source is ahead,
selected source is behind, or unavailable. This sync cursor says which source
revision the current documentation describes; it does not claim every evidence
record is current.

Moving the cursor is an intentional documentation update. Observe mode can open
older documentation against its recorded source revision, while a newer
selected source separately exposes stale or remapped evidence.

## Compare two histories as one change {#dual-repository-compare}

A companion comparison has two coordinated ranges: documentation commits in the
Saga repository and source commits in the declared code repository. The base
Saga commit is the one whose sync cursor matches the source merge base; the head
uses the selected documentation head and source head. If alignment is missing
or ambiguous, comparison stops with a recoverable diagnostic instead of pairing
unrelated ranges.

Changed, Affected, and Code then behave exactly as in an in-repository Saga.
Every result identifies whether it came from documentation history, source
history, or the evidence graph joining them.

## Document existing code without manufacturing a change {#existing-code-adoption}

An author may open the companion Saga in current-state mode, select a verified
source revision, and attach evidence to existing code. No source commit or fake
comparison is required. Per-change coverage is absent because no change was
selected; accumulated evidence ownership and health remain visible.

This path is the default for adopting a client or legacy codebase. Growth gaps
are suggestions, not a demand to create a documentation-sized source change.

## Moving beside the source preserves evidence {#portable-evidence}

Moving the Saga into its source repository changes storage location, not Saga
identity, target URNs, declared repository identity, source commits, digests, or
history. Existing code links therefore need no rewrite. With `--repo`, source
operations resolve through an explicitly selected checkout; without it, they
start from the Saga root, and Git discovers the containing source checkout.

The relocation regression authors and queries a code link while the Saga is a
companion, moves the populated Saga beside its source, drops `--repo`, and
asserts that the same Item still resolves to the same current code reference.
The move also changes sync semantics automatically: an in-repository Saga
documents the commit at which it is read and no longer advances a companion
sync cursor.
