# Reorganize existing content

Read [query.md](query.md) and inspect the existing entities before changing
them. Use the installed CLI's help and `change-saga spec --json` to discover
operations; do not create a duplicate to simulate a rename or move.

## Choose the owning operation

| Task | Public operation |
| --- | --- |
| Rename a feature or optional Product theme | `feature rename --feature ID|URN --title TEXT`; `feature-theme rename --theme ID|URN --title TEXT` |
| Rename a shared label or milestone | Existing `label rename` or `milestone rename`; assignments retain the same identity |
| Rename or order ordinary deck content | Existing `revise-deck`, `revise-slide`, `revise-item` with their title/label and rank fields |
| Rename or order narrative | Existing `revise-chapter`, `revise-section`, `revise-fragment` with `--target`, `--title`, `--order` |
| Revise transaction-authored slides or Items | Existing `apply-slide` or `diagram edit`; retain observed snapshot and history |
| Reparent narrative | `move-chapter`, `move-section`, `move-fragment` with `--target` and `--parent` |
| Transfer an implementation deck to another feature | `move-deck --deck TARGET --feature ID` |
| Move an ordinary implementation slide | `move-slide --slide TARGET --deck TARGET`; transaction-authored and review slides are refused |
| Move a story, rank planned work, or change theme membership | Existing `story move`, `feature set-plan --priority`, `feature-theme set-features` |
| Delete a record that should not exist | `story remove`, `persona remove`, `term remove`, `flag remove`, `citation remove`, `prototype remove`, `quality test-case remove`, `feature remove --with-content`. See [Deleting records](#deleting-records). |
| Remove unused classification entities | `feature remove` (empty feature), `feature-theme remove`, `label remove`, `milestone remove`; dependencies must be resolved first |
| Add planned stories to an existing feature, then land them | `feature add --status planned --augments EXISTING` (or `feature set-plan --augments`); when built, `feature deliver --feature ROADMAP_ID --dry-run`, then without `--dry-run`, then `feature set-overview` on the recipient. See Delivering a Roadmap feature below. |
| Hide a feature that holds content | `feature archive --feature ID [--reason TEXT]` keeps every record and its status; `feature restore --feature ID` brings it back |
| Remove ordinary content | Existing `remove-deck`, `remove-slide`, `remove-item`, `remove-chapter`, `remove-section`, `remove-fragment`; inspect contained targets and dependencies |
| Remove transaction-managed technical content | `remove-deck` and `remove-slide` append removal history and hide content; use `--dry-run --json` first and optionally `remove-slide --expected-snapshot SHA256`. Review slides remain protected; removing a transaction Item alone requires an `apply-slide` update. |
| Retire an inventory entity | Its existing lifecycle command; preserve revisions, events, pins and evidence |
| Remove a criterion | `criterion remove` appends a story revision without it |
| Change whiteboard title or discovery | `whiteboard rename --id ID --name TITLE`, `archive`, `restore`; content and history remain intact |

New moves and metadata edits support `--dry-run` and `--json`. Preview changes,
then repeat without `--dry-run`. Query the resulting ownership and run
`change-saga validate --json change.saga`. A refusal is a model boundary, not
permission to move files manually. Whole-deck moves retain complete-slide
history; moving one transaction-authored slide between decks is unsupported.
Use explicit positions to reorder; a sequence of commands is not an atomic
shuffle operation. Do not change identity just to change a title or position.

## Deleting records

The Saga is the current documentation; Git keeps the history. When a story,
persona, term, flag, citation, prototype, test case, or feature should no
longer exist, delete it rather than retiring it. A remove deletes the record
and every record that exists only to link to it (relations and their repins,
prototype annotations, quality policies, coverage exceptions, theme
membership), so nothing is left pointing at dead data.

Records with content of their own that referenced it (slides, test cases,
stories, terms, flags, onboarding Items) are not touched. They lose what they
covered, and the removal lists each one with the command that adjusts it.
`status` and `check --covers health` keep reporting them as dangling
references until they are fixed.

Always preview first, and tell the user what will happen before you delete:

```sh
change-saga story remove --story URN --dry-run change.saga
```

The first line summarizes the size: how many files would be removed and how
many records elsewhere would need adjusting. If it removes more than the user
plainly asked for (a whole feature, many relations), or leaves records in
other features to adjust, show the user that summary and the list and confirm
before running it without `--dry-run`. After removing, work through the
listed adjustments, or tell the user which remain.

## Delivering a Roadmap feature

Planned stories for an existing built feature live in a separate Roadmap
feature that records `augments`, so the built feature never mixes planned and
built work. The reviewer's Roadmap shows "→ extends" the target, and the
target's page lists it under Planned extensions.

When the work is built, preview the delivery:

```sh
change-saga feature deliver --feature ROADMAP_ID --dry-run change.saga
```

`--into ID` names the recipient when the feature records no `augments`. The
delivery moves every story, prototype, test case, citation, relation, and deck
into the recipient with the same URNs, so links and slides still resolve, then
deletes the Roadmap feature, including its overview essay (Git keeps it).
Content it cannot place, such as design chapters, other report fragments, or a
work plan, is listed; the real delivery refuses until you move or remove it
(for example with `move-chapter`). Show the user that list before deciding.

The output prints the Roadmap overview and the delivered stories. Use them to
rewrite the recipient's overview essay so it describes the feature as it now
is, then run the printed `feature set-overview --feature RECIPIENT` command.
Until then `status` and `check --covers health` report the recipient's
overview as needing revision.

## Technical Design roles

Inspect `change-saga whiteboard list --json change.saga`. The result identifies
existing boards, effective role assignments and the metadata snapshot. Read
the chosen board with `whiteboard describe --id ID`; do not republish or copy it
to make it appear in the navigation.

```sh
change-saga whiteboard assign-role --id EXISTING_BOARD --role erd --expect OBSERVED_OWNER_OR_none --dry-run --json change.saga
change-saga whiteboard assign-role --id EXISTING_BOARD --role erd --expect OBSERVED_OWNER_OR_none --json change.saga
```

Replace `OBSERVED_OWNER_OR_none` with the current role's board ID, or `none` if
empty. The roles are `erd`, `infrastructure`, `data-flows`, and `architecture`. Assignment retains
the existing board ID and history, and consolidates its navigation placement:
Technical Design has ERD, Infrastructure, Data flows, and Architecture, with no duplicate
entry for the assigned board. Other boards remain reachable from the overview
and their named URLs. A display rename alone does not assign a role.

Without explicit presentation metadata, existing conventional IDs supply role
defaults: `data-model` owns ERD, and `infrastructure`, `data-flows`, and
`architecture` own their namesake roles. Publishing one can therefore fill an
empty role implicitly. Reread `whiteboard list --json` after publication; do not
blindly follow it with `assign-role --expect none`. If the desired board already
owns the role, no assignment is needed. If changing an observed owner named
`architecture`, the expectation is `--expect architecture`. Explicit metadata
may override these conventions, so always use the effective list result.

If the board belongs to another role, explicitly unassign it first using
`whiteboard unassign-role --id OBSERVED_OWNER --role ROLE`, or use the documented
move option with the observed metadata snapshot. Re-read after a concurrency
failure. Archive only an unassigned board; restore makes it discoverable again.
Archival never deletes its content, old revision URLs, or references.
