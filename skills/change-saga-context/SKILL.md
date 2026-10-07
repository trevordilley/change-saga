---
name: change-saga-context
description: 'Read existing Change Saga documentation to guide everyday repository work: answering product and architecture questions, planning features, implementing changes, debugging, and tracing intent or dependencies. Use proactively when the repository has a Saga and its documented context could inform the task, even when the user does not mention Saga. Revisit relevant records as new questions arise. This is a read-only context skill; use change-saga for requested authoring or end-of-work documentation updates.'
---

# Use Saga context during work

Reach for the repository's Saga early and throughout the task to understand
what the product should do, how the system fits together, and where the relevant
code lives. Read the smallest useful slice, then continue the user's work.
Reading context does not start a documentation project.

## When to look

- Before planning or implementing a feature, read its existing stories,
  criteria, design, and implementation explanation to discover constraints
  and reuse established concepts.
- When debugging unexpected behavior, compare the observed behavior with
  documented intent and follow linked code and tests.
- When a component, term, boundary, or dependency is unfamiliar, consult its
  definition and relationships before guessing or asking the user to repeat
  knowledge already recorded.
- When scope changes or a new question arises during implementation, return
  to the relevant records. Reuse context already read while it remains current;
  there is no need to query on every turn or reload the whole Saga.
- When explaining a system or evaluating a proposed change, use the documented
  model to locate responsibilities, decisions, and affected behavior. For an
  explicitly requested code review, inspect the diff independently before
  checking the author's explanation and evidence.

Skip lookups that cannot inform the task, such as a mechanical edit whose
context is already clear. A Saga lookup should answer a concrete question.

## Find and read a focused slice

Use the Saga path supplied by the user or repository guidance, usually
`change.saga` at the repository root. A directory existence check is enough
for initial discovery; do not crawl Saga metadata. If no Saga exists, continue
with code and other documentation without creating one. If the CLI is missing
or a query fails, state a material limitation and use available sources;
ordinary context gathering does not require installation or repair.

Use the installed CLI's help and `change-saga query schema <operation>` to
discover supported flags and response paths. Read Saga records through
`change-saga query` or `change-saga diagram describe`, never by opening,
grepping, or globbing metadata files.

| Question | Starting read |
| --- | --- |
| Where is the relevant feature or deck? | `change-saga query overview --saga PATH`, then `change-saga query children --saga PATH --parent URN` |
| What is the narrative for this deck? | `change-saga query overview --saga PATH --deck URN` expands only its overview report |
| What do we already know about this feature? | `change-saga query context --saga PATH --feature ID` |
| What is the exact intent behind one story? | `change-saga query context --saga PATH --feature ID --expand STORY_ID` |
| What behavior is required, and for whom? | `query requirements` and `query personas` for the relevant identities |
| What does this term or component mean? | `query terms` or `query inventory` |
| How does this part work? | `change-saga diagram describe --slide TARGET PATH`, or `query fragment` for design prose |
| What intent and evidence relate to this code? | `query traceability --ref LOCATION`; discover its supported location format from help |
| Which automated tests should run for a review? | `change-saga review test-plan --review ID --json PATH` returns tests, recorded commands, affected stories and gaps |
| Why did the documented intent change? | `query requirement-history`, `query citations`, or `query history` for the selected record |

Supply `--saga PATH` to queries. Use returned IDs and URNs instead of guessing
them. Start directly with a known feature or record; an overview is only needed
when its location is unknown. Expand exact prose and code references only for
records relevant to the question.

The default overview is a directory; it omits deck report bodies and their
annotations. Select one deck explicitly when its narrative is useful. A
Component named in prose is not necessarily a declared dependency: follow
Item documentation pins and `query inventory-uses` for the recorded links,
and preserve that query's completeness limits.

Check the JSON envelope's `ok`, `data`, `snapshot`, and `page`. Follow
`page.next_cursor` while `page.has_more` for the selected query; byte- or
offset-paged operations use the pagination described by their schema. Keep a
consistent snapshot across related reads and restart if it changes. Read
reported completeness limits, stale evidence, and competing heads before
drawing conclusions. Absence from a bounded projection is not proof that
something is undocumented.

## Apply what you learned

Use the results to guide the next code read, implementation decision, or
focused question. Cite useful record identities and their code locations in
an explanation or handoff so the reasoning can be followed.

Distinguish accepted intent, proposed work, historical review explanations,
and current implementation. Check material behavior claims against source and
tests; a linked or fully covered line does not prove the prose is correct.
Preserve conflicts and uncertainty. If documentation and code disagree, explain
the mismatch rather than silently deciding which one defines the desired behavior.

## Read throughout; update near completion

This skill performs reads only. Keep discovered documentation gaps in the
working context while investigating and implementing. Do not interrupt each
edit, commit, or answer to refresh decks, repin evidence, reconcile the whole
Saga, or create missing records.

Use the companion `change-saga` authoring skill when the user explicitly asks
to create or update documentation, including when that request arises during
a PR deck conversation. Otherwise do scoped upkeep near the end of the work,
normally when preparing or updating its PR, or during a final handoff whose
agreed workflow includes documentation. Finish implementation and relevant
verification first so the documentation describes the resulting behavior.
Ending an ordinary question-and-answer turn is not an upkeep trigger.

When authoring starts, update the affected records and preserve their identities;
a request to document one feature does not require onboarding the whole
repository. Reading a Saga never authorizes implementation, publication,
review decisions, or unrelated documentation changes.
