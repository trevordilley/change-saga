# Reference identity and resolution {#reference-identity-and-resolution}

## Durable evidence identity {#evidence-identity}

Each code evidence record identifies:

| Field | Contract |
| --- | --- |
| repository | Canonical declared source repository identity. In an in-repository Saga it may be inherited; a companion checkout must still verify it. |
| commit | Full source commit object name, never a branch or tag. |
| path | Repository-relative path on the side where the referenced content exists. |
| start, end | Optional 1-based inclusive range; both are absent for whole-file evidence. |
| digest | `sha256:` digest of the exact referenced bytes or defined whole-file content. |
| note | Reader-facing explanation of what the evidence establishes and why this scope is appropriate. |

The compact location form used at interaction boundaries is
`<commit>:<path>[#L<start>[-L<end>]]`. The persisted digest is not omitted from
the evidence record merely because a compact link hides it.

Evidence identity and explanation identity are separate. Reorganizing a feature
or moving a Saga does not change an Item or landmark URN, and it does not change
the source repository, commit, path, range, or digest that evidence names.

## Source side and scope {#source-side-and-scope}

Line evidence uses the side on which the bytes exist. Added or modified lines
use the head side; deleted lines use the base side. A line range is invalid when
that side has no such content.

Renames without content change may retain line evidence after resolution.
Renames with content change, additions or deletions of whole files, binary
changes, mode changes, and type changes use whole-file evidence. Whole-file
evidence describes the event without inventing lines that cannot be rendered.
The reader always sees the source side and scope before opening code.

## Resolution outcomes are stable {#resolution-outcomes}

Resolving a pin P at viewed revision V produces one of four public outcomes:

1. **Current:** the same bytes remain at the pinned location.
2. **Current · remapped:** the same bytes resolve to one different location.
3. **Stale:** content, identity, availability, or uniqueness no longer supports
   the pin; a reason and the smallest safe comparison are returned.
4. **Missing:** no evidence record exists. Missing is produced by traversal,
   not by the resolver pretending a malformed record is absent.

Resolution verifies the stored digest before using the evidence. A change that
touches a ranged reference is stale; a pure move before the range or a pure
rename may remap. Whole-file evidence remaps only across a content-preserving
rename. Documentation-only commits do not participate in source resolution.
A current reference may cover no line in the selected comparison; currency and
per-change coverage are different questions.

## The resolution strategy is replaceable {#replaceable-resolution-strategy}

The current implementation may combine Git history, rename detection, range
translation, and a digest search when the pinned commit is unavailable. Those
mechanics are replaceable if every implementation preserves the public
outcomes above and these safety constraints:

- verify the captured digest before asserting continuity;
- return remapped only for one unambiguous content-equivalent target;
- never select a candidate by path or proximity alone;
- exclude Saga-only paths from source movement;
- bound search and report an unavailable or ambiguous result as stale;
- return enough provenance to explain the outcome and reproduce it.

A unique same-path digest match is a permissible fallback, not the product
promise. A future syntax-aware resolver may replace it without changing stored
evidence or the four user-visible states.

## Rebind to landed source without erasing history {#merge-rebinding}

Post-merge refresh resolves eligible branch evidence against the landed source
revision, previews every outcome, and atomically records new pins only for safe
current or remapped results. Stale and ambiguous records retain their prior pins
and diagnosis. The operation records branch commit reasons with the landed
change so a squash or deleted branch does not erase the reasoning later shown
in history.

Rebinding changes where evidence opens; it does not rewrite the explanation,
claim, verification, or requirement the evidence supports. Repeating the same
landed revision is a no-op.
