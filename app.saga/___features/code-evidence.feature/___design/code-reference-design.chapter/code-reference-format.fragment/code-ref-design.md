# Code reference format {#code-reference-format}

Evidence never stores a diff. A code reference says "this node explains these
lines, as of this commit", and a diff is a way of viewing references against
two commits.

## Fields {#fields}

| Field | Meaning |
| --- | --- |
| `commit` | a full commit object name, never a symbolic ref |
| `path` | the repository path |
| `start`, `end` | a 1-based inclusive line range; both absent for a whole-file reference |
| `digest` | `sha256:` and the hex digest of the exact referenced bytes |
| `note` | the reviewer-facing what-and-why, shown before the ranges are expanded |

References do not repeat the repository; it is the Saga's declared
`source.repository`. The compact location form, used by CLI flags, query
arguments, URLs, and traceability results, omits the digest:
`<commit>:<path>[#L<start>[-L<end>]]`.

## Deletions and file events {#deletions-and-file-events}

Deletions reference the base side of a comparison, since removed lines exist
only there. Renames, mode and type changes, binary changes, and file additions
or deletions are referenced by whole-file references.

## Viewing a reference at another commit {#viewing}

A reference pinned at commit P is viewed at commit V by diffing P against V,
with renames followed and `.saga` paths excluded:

1. The pinned digest is verified first; a mismatch makes the reference stale.
2. Insertions and deletions entirely before the range shift it: the reference
   is remapped and stays current.
3. Any change touching the range, including an insertion inside it, makes the
   reference stale, with the reason and the diff since the pin available.
4. A whole-file reference remaps only on a pure rename and is stale on any
   content, mode, or binary change.
5. Commits that change only `.saga` paths leave the code identical, so they
   never move or stale a reference.
6. If the pinned commit is no longer available, the reference is resolved by
   searching for its digest in the same path at V; only a unique match counts.

A reference that is current but covers no changed line is not stale: it
explains unchanged code.

## Re-pinning at merge {#re-pinning}

When a change lands, `change-saga repin --onto REV [--branch REV]` re-pins
coverage references from branch commits to the landed commit, following moved
lines exactly as viewing does, and records the branch's commit messages in
`___merges/<landed-commit>.json`. A squash merge or a deleted branch therefore
loses nothing.

Source: SPEC.md, sections 5, 5.1, and 6.2.
