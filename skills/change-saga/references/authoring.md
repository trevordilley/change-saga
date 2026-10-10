# Authoring references

Read only the reference needed for the task.

- For diagrams, decks, narrative, Items, exact evidence, claims, or visual QA,
  read [diagrams.md](diagrams.md).
- For requirements and lifecycle authoring, read [stories.md](stories.md).
- For overview and vocabulary, read [terms.md](terms.md).
- For comparison, pull-request integration, repinning, or recovery, read
  [integration.md](integration.md).
- For inspection and all existing-Saga reads, read [query.md](query.md).

Return to the route table in `SKILL.md` when a task crosses these boundaries.

## Shared data-model overview and ERD documents

Author each database or cross-store diagram as its own named `erd add` record;
reuse exact data-entity pins across their directories. Use
`overview set-data-model --source FILE <saga>` for shared Markdown context.
`query inventory --kind erd` lists the independent documents and returns
`data_model_overview` with the overview fragment target (or null). Read the prose
with `query fragment --target URN` and Git history with `query history --node URN`.
There is no reserved primary ERD ID. The optional overview slot requires a reader
that supports it; existing ERDs and immutable pins need no migration.
