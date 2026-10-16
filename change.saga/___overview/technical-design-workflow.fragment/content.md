# Authoring and review workflow {#authoring-and-review-workflow}

Use `bin/change-saga` built from the current source tree for this repository's authoring. The [Data flows board](/technical/whiteboards/data-flows) distinguishes the write pipeline from the read and comparison pipelines.

## Start from the reader's question {#start-from-the-reader-s-question}

Use `query overview` and `query context --feature ID` to discover existing identities. Read current stories, personas and exact criterion wording. Follow every cursor and keep one snapshot across a multi-query read. A stale cursor requires restarting that read, not combining pages from different states.

Product prose explains purpose and observable outcomes. The Technical design essay explains the general approach and links to the ERD, Infrastructure and Data flows boards. Feature implementation decks explain mechanisms through semantic Items and exact source evidence. Review decks explain the transition someone is being asked to review.

## Author a coherent revision {#author-a-coherent-revision}

Use `overview set-technical-design` for the approach or a supporting page, `feature set-overview` for a feature, and `story revise` or `criterion revise` for existing Product records. A criterion's title is a short label; its full statement remains the pass/fail obligation. Its overview explains rationale, examples and boundaries. Changes to that content can stale related evidence, so never treat copyediting as an automatic evidence approval.

Use `whiteboard publish --id ID --revision REV --parent PARENT --from FILE` for a subsequent complete board publication. The CLI checks geometry, source references and parentage before storage. A whiteboard source reference is not review coverage. Put exact code evidence on the explanatory target that owns it.

## Verify and hand off {#verify-and-hand-off}

Run `validate`, each board's `whiteboard check`, and the relevant code and browser tests. Inspect the rendered page at a normal reading size and after focusing a board section. Check that links and crow's-foot endpoints mean what the prose claims. Validation proves structural consistency; a reader must still assess the explanation.

When a change has stale evidence, inspect the reconciliation proposal and source diff before accepting a remap. Reviews retain their own comparison and explicit decisions. Publishing a story, diagram or deprecation does not record a review verdict. [Return to the approach](/technical).
