# Hybrid Report + Slide Sagas {#hybrid-report-slide-sagas}

This proposal gives one Change Saga two complementary documents with different
lifecycles:

| Surface | Durable subject | Lifecycle |
| --- | --- | --- |
| Living report | User stories, acceptance criteria, design intent, and work history | Revised as product intent changes |
| Review decks | Visual explanations of a specific implementation comparison | Replaced or extended as implementations change |

They share a Saga identity, but neither contains the other semantically. A
revision-pinned `explains` relation says which accepted story or criterion a
deck, slide, or Item covers. Exact code evidence stays on Items.

## Accepted stories {#accepted-stories}

- **Separate lifecycles, one Saga:** compose report and deck records without
  mechanically turning requirements into slides.
- **Focused implementation review:** expose one sidebar disclosure per authored
  deck, with subtle section dividers among its rendered slide thumbnails,
  exact Item evidence, surprise callouts, and full-main-area slide review.
- **Close the loop to user intent:** traverse from accepted criteria to code and
  from an exact diff or current committed head back to the story.

The requirements records are the normative statements and acceptance criteria.
The decks are deliberately selective: they explain the implementation model,
the traceability path, and the reviewer experience where a visual relationship
is more useful than prose. This Saga uses one **Implementation review** deck;
Architecture and storage, Traceability, and Reviewer experience are sections
inside it, not three separate decks.

## Review route {#review-route}

1. Read the accepted stories and criteria in the requirements surface.
2. Expand **Implementation review** once in the Saga sidebar.
3. Use the **Architecture and storage** and **Traceability** dividers to inspect
   the document boundary, Git layout, and code-to-story loop.
4. Under **Reviewer experience**, open **Decks expand into rendered thumbnails**
   to inspect the sectioned previews, deck-local navigation, and full main-area
   review state.
5. Use linked Items to open the exact implementation and tests.

## Deliberate limits {#deliberate-limits}

- Prototype records remain internal; public prototype CLI, query, and UI are
  staged.
- Story-to-code traceability is available through the query API; story badges
  and click-through paths in the reviewer UI are staged.
- `--commit` means the current committed comparison head. It rejects
  `WORKTREE` and does not perform intermediate-commit blame analysis.
- A deck- or slide-level relation intentionally applies to descendant Item
  evidence, so broad links must be authored carefully.

## Verification {#verification}

The implementation and this Saga are checked with the repository-wide Go test
suite. The browser-facing hybrid deck path also has a focused Playwright test.
Coverage completeness remains an omission check, not proof of correctness.
