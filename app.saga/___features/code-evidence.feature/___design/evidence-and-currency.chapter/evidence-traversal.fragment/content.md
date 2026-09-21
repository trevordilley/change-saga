# Evidence traversal {#evidence-traversal}

Evidence is navigated as a connected argument, not as a folder tree. A reviewer
can begin with a person, story, criterion, design landmark, test case, claim, or
source location and move in either direction without losing the starting
context. Authored links remain adjacent; the application derives longer paths.

```text
person → story → criterion → design landmark → explanation → source
                       └────→ test case → run evidence
source → explanation → design landmark → criterion → story → person
```

## Forward path from a requirement {#evidence-path}

A story header names every person it serves. Each acceptance criterion owns an
Evidence panel with separate Design, Tests, and Code rows. A row shows the
nearest authored link first and expands the derived hops beneath it; it never
pretends the whole chain was authored directly.

Selecting a design result opens the exact fragment landmark and preserves the
criterion in a context rail. Selecting a test opens the test revision and its
latest applicable results. Selecting Code opens the exact repository, commit,
path, and range or whole-file event. Back returns to the same expanded
criterion, not merely the story top.

A criterion with several valid paths lists them independently. Ordering is
stable: current before stale, direct before derived, then human-readable label.
No path is collapsed into a score or a single “covered” badge.

## Reverse path from source {#reverse-evidence}

A source location exposes two distinct lists:

- **Explained by** lists the Items or design landmarks whose evidence contains
  the location.
- **Supports requirements** walks from those explanations through current
  design relations to criteria, stories, and people.

Each result shows the actual hops and stops at a missing or stale link. Shared
code may support several explanations or requirements; all are shown, and none
is treated as the exclusive owner. Opening a reverse result keeps the source
location in the context rail so the reviewer can retrace the path.

## Health belongs to every hop {#path-health}

Every hop is labeled **Current**, **Missing**, or **Stale**. Current means its
pins still identify the viewed revisions and content. Missing means the next
adjacent link does not exist. Stale means a link exists but one of its pinned
endpoints changed. A stale path remains navigable to the last-confirmed target
and offers the current candidate beside it; it never silently counts as current.

The overall path takes the least healthy state of its hops, but the UI still
shows which hop caused that state. Test outcomes remain separate from link
health: a current path may lead to a failed test, and a passing result attached
through a stale link does not repair that link.

## Change impact follows the same graph {#impact-path}

In comparison mode, a changed source range first finds the explanations whose
code evidence overlaps it. The application then walks only current adjacent
relations to criteria, stories, and people. The Affected layer groups results by
requirement and person, while retaining the originating changed range and the
path that produced the impact.

Potential impact is phrased as “may affect,” not as proof that behavior changed.
A stale evidence edge is shown as a boundary: the records on the far side are
listed as uncertain impact rather than omitted or asserted as current.

## Gaps are destinations, not dead ends {#visible-gaps}

An active person with no accepted story appears in the same evidence view as a
visible gap. Missing design, test, or code rows explain the absent adjacent link
and offer the focused authoring action for that one gap. The reviewer can still
navigate the surrounding records; absence is reported without inventing an
inferred link or turning growth into a release verdict.
