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
| Remove unused mutable entities | `feature remove`, `feature-theme remove`, `label remove`, `milestone remove`; dependencies must be resolved first |
| Hide a feature that holds content | `feature archive --feature ID [--reason TEXT]` keeps every record and its status; `feature restore --feature ID` brings it back |
| Remove ordinary content | Existing `remove-deck`, `remove-slide`, `remove-item`, `remove-chapter`, `remove-section`, `remove-fragment`; inspect contained targets and dependencies |
| Remove transaction-managed technical content | `remove-deck` and `remove-slide` append removal history and hide content; use `--dry-run --json` first and optionally `remove-slide --expected-snapshot SHA256`. Review slides remain protected; removing a transaction Item alone requires an `apply-slide` update. |
| Retire a requirement or inventory entity | Its existing lifecycle command; preserve revisions, events, pins and evidence |
| Remove a criterion or retire a prototype | `criterion remove` appends a story revision; `prototype revise --state retired` appends a prototype revision |
| Change whiteboard title or discovery | `whiteboard rename --id ID --name TITLE`, `archive`, `restore`; content and history remain intact |

New moves and metadata edits support `--dry-run` and `--json`. Preview changes,
then repeat without `--dry-run`. Query the resulting ownership and run
`change-saga validate --json change.saga`. A refusal is a model boundary, not
permission to move files manually. Whole-deck moves retain complete-slide
history; moving one transaction-authored slide between decks is unsupported.
Use explicit positions to reorder; a sequence of commands is not an atomic
shuffle operation. Do not change identity just to change a title or position.

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
