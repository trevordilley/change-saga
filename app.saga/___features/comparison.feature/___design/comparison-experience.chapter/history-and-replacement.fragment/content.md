# History, replacement, and reasoning {#history-replacement-and-reasoning}

Living documentation shows the current answer. Git-backed history supplies the
transition and its reasoning without keeping obsolete alternatives in the
current body.

## Pair replacements conservatively {#replacement-pairing}

Comparison proposes a removed/add pair only when both records occupy the same
traceability role—for example, they address the same criterion or replace the
same explanation target—and the match is unambiguous. The view shows old and
new together with the shared path that justified the inference.

If zero or several candidates fit, records remain separate until an author
creates an explicit replacement relation. The UI labels inferred and explicit
pairing differently. Similar titles, nearby files, or adjacent timestamps are
never sufficient on their own.

## Put commit reasons beside their effects {#reason-attribution}

A commit reason appears on each documentation record or code group the commit
actually touched. The card shows the subject, author, commit, and the relevant
record or source paths; expanding it reveals the full message. A flat commit log
may be available as secondary navigation but is not the explanation for a node.

Merge commits and mechanical commits remain visible and may say that no useful
reason was recorded. The system does not synthesize intent from a diff.

## Node history answers three questions {#node-history}

Every addressable record exposes:

1. **Introduced** — the first commit containing its stable identity.
2. **Replaced** — the prior record it superseded, when a current unambiguous or
   explicit pair exists.
3. **Open the change** — a link to the frozen comparison that introduced the
   current revision or replacement.

History is chronological and preserves revisions and lifecycle events without
placing retired content in the current page. Renames or feature moves retain the
same node identity. When Git evidence is incomplete, the field says unavailable
rather than selecting the nearest commit.

## Preserve branch reasoning through merge {#merge-reason-preservation}

Post-merge refresh stores the ordered branch commit identities and messages
with the landed change before a branch can disappear. A squash merge therefore
shows the landed commit as source identity while the history panel retains the
individual reasons attributed to their original effects.

The preserved messages are immutable provenance, not new decision records.
Duplicate refresh is idempotent, and an unverifiable branch-to-landed mapping is
reported rather than attached to the wrong comparison.
