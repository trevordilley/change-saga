# Evidence currency and repair {#evidence-currency-repair}

Currency answers one question: does this evidence still identify the same
source meaning at the source revision being viewed? It is computed for the
selected source checkout and revision. It is not an approval, test result, or
confidence score.

## Current, remapped, stale, and missing {#currency-states}

| State | What the reader sees | Consequence |
| --- | --- | --- |
| **Current** | The captured bytes still occur at the captured location. | The link opens normally. |
| **Current · remapped** | The same captured bytes moved to one unambiguous location. The old and resolved locations are both available. | The link opens at the resolved location and still counts as current. |
| **Stale** | The captured bytes changed, disappeared, became ambiguous, or the pin cannot be trusted. | The last-confirmed evidence remains inspectable but does not count as current support. |
| **Missing** | No evidence link was authored. | The gap is explicit; no source is guessed. |

A rename with unchanged content is a remap. A change inside the referenced
range, a non-unique fallback match, or a content-digest mismatch is stale.
Changes that affect only Saga documentation do not move source evidence.

## Inspect what became stale {#stale-inspection}

A stale badge opens a diagnosis drawer with the captured source, the viewed
source candidate, and the smallest available diff between them. It names the
reason—content changed, source removed, ambiguous match, missing commit, or
repository mismatch—and shows the pin and viewed revision. If no safe candidate
exists, the drawer says so instead of opening unrelated code.

The drawer preserves the explanation and requirement context. A reviewer can
continue reading the last-confirmed evidence; an author can enter repair from
the same place.

## Scrutiny-ranked mappings {#mapping-scrutiny}

The mapping view is an author queue ordered by review value, not a truth score.
It raises broad whole-file evidence, large ranges, one explanation covering
many unrelated areas, overlaps, stale mappings, and thin rationales. Each row
shows the exact target, source span, reasons for scrutiny, and affected current
requirements. Filters never hide stale records by default.

Scrutiny is advisory. A broad mapping may be intentional; dismissing or
replacing it requires a rationale so future reviewers can distinguish a choice
from an unexamined link.

## Atomic replacement {#atomic-replacement}

Repair starts from one existing evidence record and previews the complete
replacement: target, repository, source side, commit, range or file event,
digest, note, and the resulting current/stale state. Confirmation writes the
new record and retires the old one as one operation. If any path, revision,
digest, target, or repository check fails, neither record changes.

The success view links to the replacement and keeps the prior record in
history. Repeating the same request is idempotent. A repair never steals ranges
from another explanation or rewrites independent claims and verifications.

## Refresh after merge {#post-merge-refresh}

After landing, the author refreshes branch-pinned evidence onto the landed
source revision. The preview distinguishes unchanged, remapped, stale, and
unresolvable records before writing. A squash merge or deleted branch remains
repairable from captured content and merge metadata; unresolved records stay
stale rather than receiving a guessed pin.

The refresh records the landed revision and retains the branch commit reasons
for later history. Success means every selected record opens against the landed
source; it does not imply that unrelated stale records were repaired.

## Documentation-only changes preserve source currency {#saga-only-stability}

A commit that changes only the Saga can change relation currency, because a
requirement or design endpoint may have changed, but it cannot by itself stale
an unchanged source reference. The UI names these as separate checks—design
relation health and source evidence health—so a documentation edit never looks
like a code change.
