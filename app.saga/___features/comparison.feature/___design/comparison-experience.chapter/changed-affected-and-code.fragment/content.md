# Changed, Affected, and Code {#changed-affected-and-code}

Comparison mode is one range projected into three complementary layers. The
layers share filters and selection: choosing a record or source range in one
layer keeps it selected while moving to another.

```text
resolved merge base ─┬─ Saga record delta ───────────────→ Changed
                     ├─ invalidated evidence graph ──────→ Affected
selected head ───────┴─ source diff + evidence overlap ─→ Code
```

## Changed: authored documentation delta {#changed-layer}

Changed lists each Saga record added, revised, moved, or retired between the
resolved base and head. A row names the record kind, owning feature, lifecycle
action, before and after revision, and commits that touched it. Opening a row
shows a semantic before/after view; unchanged storage churn is not promoted as
a product change.

Replacement pairs appear together rather than as unrelated add/remove rows.
Records changed only in source code do not appear here; they belong in Affected
and Code.

## Affected: unchanged knowledge at risk {#affected-layer}

Affected begins with changed source and changed mutable endpoints, then follows
pins and current adjacent relations outward. It lists unchanged records whose
code evidence became stale, whose related requirement/design/test revision
changed, or whose derived evidence path crosses such a boundary.

Each row states **why affected**, shows the originating change, and distinguishes
current impact from uncertain impact behind a stale edge. Code-only changes can
therefore surface designs, criteria, stories, and people even when no Saga file
changed. An affected record is not asserted to be wrong; it is a focused review
obligation.

## Code: explanation ownership and gaps {#code-layer}

Code groups every changed line or whole-file event under all explanations whose
current evidence overlaps it. Groups lead with the explanation and requirement
context, then show the diff hunk. If several explanations overlap, the line is
shown once with every owner; coverage is not stolen by the first match.

A separate **Unexplained** group lists every changed atom with no current
explanation. Stale evidence is shown beside the atom as stale context but does
not count as coverage. Deleted content renders from the base side; additions
from the head side; binary, rename, mode, and type events remain whole-file
atoms.

Coverage is recomputed when the selected comparison changes. It is never an
author-maintained total, and unexplained legacy code outside the current diff
does not block complete per-change coverage.

## Cross-layer evidence traversal {#layer-traversal}

From any Changed or Affected record, **Show code** opens the contributing atoms.
From any code atom, **Show impact** opens its requirements and people with the
full evidence path. From an unexplained atom, **Explain this change** starts an
authoring flow scoped to that exact atom.

The browser, human-readable status, and structured layer query expose the same
record IDs, source atoms, reasons, and comparison range. Presentation may differ;
membership and state may not.
