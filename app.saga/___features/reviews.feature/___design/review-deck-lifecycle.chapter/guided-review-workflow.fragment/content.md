# A review deck is a guided argument {#guided-review-workflow}

One pull request has at most one review deck. The deck is not a gallery of
screens or a reformatted diff: it is the ordered argument for what changed, why
the chosen design is appropriate, where risk lives, and which exact code each
claim explains.

## Deck spine and session flow {#deck-spine}

The deck opens with scope, base, head, and a concise objective. Slides then
orient, explain, compare alternatives, trace cross-cutting effects, prove
important behavior, surface risks, and conclude. Each slide has one reviewer
job, one takeaway, and addressable items that connect the visual argument to
lasting documentation and exact changed source.

A review session resumes at the earliest slide that is unread, undecided, has an
open thread, or became out of date. The overview shows quiet progress by slide
and one primary resume action. Direct slide and item URLs, browser history, and
the selected comparison remain authoritative.

## Decisions and comments {#decisions-comments}

A decision belongs to one slide and records approve, request changes, or clear,
plus the exact reviewed head. It never declares the pull request approved.
Comments target a slide or item; replies and resolution remain within the same
thread. A decision dialog and comment composer state their target, preserve
draft text on failure, and return focus to the invoking context on success.

Documentation pages expose neither decisions nor comment composers. A review
item may link to the documentation it changes, but all change-specific
discussion stays in Review.

## Currency model {#review-currency}

A decision is current only while both the slide content and every linked code
target match what was reviewed. A slide edit, changed evidence, repin, or new
head makes the earlier decision visibly out of date without deleting it. New
decisions append to history; clearing a decision is also an event.

Comments remain historically accurate even when their target changes. The UI
shows the target snapshot, current availability, and any newer thread activity
rather than silently relocating meaning.

## Omission model {#review-omissions}

Coverage is an omission lens, not a verdict. The review identifies every changed
line without an explaining item, every item whose code is stale or unavailable,
and every slide whose cited lasting documentation is stale. Empty or partial
evidence never renders as success. Each omission links to its exact scope and
offers the smallest safe authoring action.

A complete line mapping does not prove that the argument is correct. Reviewers
retain the judgment; the product records their individual decisions and
comments.

## Merge and historical resumption {#merged-history}

Before merge, the review follows the pull request's current base and head. After
merge, that range, the final deck, all decision versions, comments, omissions,
and currency transitions freeze as read-only history. The merged review remains
reachable from documentation it changed and can be resumed at the same slide or
item, but it cannot absorb later product edits as if they had been reviewed.

If history is unavailable or rewritten, the application says attribution or
currency cannot be established. It does not invent a reviewer, a reviewed head,
or a current decision.
