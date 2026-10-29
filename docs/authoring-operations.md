# Reorganize an existing Saga

Use public commands to change presentation or ownership while retaining stable
identities. Discover selectors with `change-saga query`, inspect command `--help`
and `change-saga spec --json`, and use returned IDs or URNs. Never rename metadata
files to rename content. A title, position, parent, and navigation role are
different properties.

## Entity and capability matrix

| Entity | Create and read | Rename or revise | Move or order | Remove or retire |
| --- | --- | --- | --- | --- |
| Feature | `feature add`; `query overview`, `query context` | `feature rename`, `feature set-overview` | `feature set-plan --priority`; built/planned placement uses `--status` | `feature remove` refuses contained content and dependencies; `feature archive` hides a feature with content (and its coverage gaps) without removing anything, and `feature restore` brings it back |
| Product theme | `feature-theme add`; `query feature-themes` | `feature-theme rename`, `set-overview` | `set-features` replaces membership; features retain their identities | `feature-theme remove` refuses decks and inbound dependencies |
| Label, milestone | `label add`, `milestone add`; corresponding queries | Existing `rename` changes the shared title once | Assign through `feature set-plan`; clear assignments explicitly | `remove` refuses referenced records |
| Story, persona, criterion | Existing family `add`; corresponding queries | Existing `revise` appends immutable revisions | `story move` changes its feature without changing its URN | Story/persona lifecycle; `criterion remove` appends a story revision and preserves historical criteria |
| Component, system, data entity, ERD, overlay, term | Existing family `add`; inventory and term queries | Existing `revise` appends a revision with explicit parents | Revise relationships or membership through the owning entity | Existing `set-state` retires records; no destructive history deletion |
| Flag, prototype | Existing `flag add`, `prototype add` / `add-external` / `add-html`; corresponding queries | Existing `revise` preserves identity and revision history; `prototype update` records a changed prototype file | Revise associations through the owning API | `flag set-state`; `prototype revise --state retired` |
| Work wave, work item | Existing `plan add-wave`, `add-item`; work-plan queries | `plan revise-wave`, `revise-item` | `revise-wave --order`; `revise-item --wave`; explicit dependencies | Progress events preserve work and merge evidence; no destructive removal |
| Technical or UX deck | `add-deck` (`--role ux` for a feature's UX flows); `query children`, `query overview --deck` | `revise-deck --title`, `deck overview` | `move-deck --feature`; `revise-deck --rank` | Existing `remove-deck` reports contained targets and affected relations |
| Slide, Item | `add-slide`, `add-item`, `apply-slide`; `query slide`, `query children` | Existing `revise-slide`, `revise-item`, content editing, or complete-slide transactions | `move-slide --deck`; rank edits or complete-slide transactions | `remove-slide` preserves transaction history; removing a transaction Item alone requires an `apply-slide` update |
| Chapter, section, fragment | Existing `add-*`; `query children`, `query fragment` | Existing `revise-* --title`; `set-fragment-content` | `move-* --target --parent`; existing `revise-* --order` | Existing `remove-*` accounts for contained content and dependent relations |
| Whiteboard | `whiteboard publish`, `get`, `describe`, `list`; `query whiteboards` | `whiteboard rename` changes display metadata; `publish` appends content history | `assign-role`, `unassign-role` change Technical Design placement | `archive`, `restore` change discovery without deleting history |
| Evidence, relation, review, claim, verification | Existing typed authoring and query APIs | Exact coverage replacement/repinning; immutable relation/review/claim history | Ownership changes do not confer coverage or approval | Use owning lifecycle APIs; never cascade through immutable records |

“Remove” is deliberately not universal. Requirements and technical inventory
carry immutable history; lifecycle transitions express retirement. Whiteboards
can be archived and restored. Feature and classification removal is conservative:
resolve dependencies first, then remove the unused container or shared record.

## Naming and ordering

Use `--title` for existing title-based entities and `--name` for whiteboards,
matching `whiteboard publish`. A rename does not rename the ID, URN, directory,
or historic revision. Omitted fields stay unchanged.

Ranked deck content uses `--rank`; narrative uses `--order`; planned features use
`feature set-plan --priority`. These existing controls also support shuffling:
assign the desired explicit positions. There is no random shuffle command.
Each invocation is a transaction; a sequence of rank edits is not one atomic
batch. Complete-slide content uses its existing snapshot/parent transaction
contract rather than rewriting a published revision.

## Move content

```sh
change-saga move-deck --deck DECK_URN --feature destination --dry-run --json change.saga
change-saga move-slide --slide SLIDE_URN --deck DESTINATION_DECK_URN --dry-run --json change.saga
change-saga move-section --target SECTION_URN --parent CHAPTER_URN --dry-run --json change.saga
change-saga move-fragment --target FRAGMENT_URN --parent SECTION_URN --dry-run --json change.saga
```

Moves resolve the destination, validate its kind and ownership, reject cycles
and collisions, and retain stable targets. They do not copy entities or recreate
evidence. Review content cannot be transferred into a different review or a
documentation deck: its identity and evidence belong to its review.
Stable URN references continue to resolve. Arbitrary prose hyperlinks containing
filesystem paths are not rewritten; inspect those authored links when relocating
content. A slide cited by its source deck's overview must be removed from that
overview explicitly before moving; dry-run also reports this dependency.

`move-slide` supports ordinary implementation slides. A complete-slide
transaction's history is bound to its original deck; moving it individually is
refused. Move the whole deck with `move-deck` to retain that history unchanged.

Inspect the dry-run result, repeat without `--dry-run`, then query the source
and destination and validate the Saga. Unsupported moves fail with no changes;
do not work around a refusal by editing generated files.

## Remove published technical decks and slides

```sh
change-saga remove-deck --deck DECK_URN --dry-run --json change.saga
change-saga remove-deck --deck DECK_URN --json change.saga
change-saga remove-slide --slide SLIDE_URN --expected-snapshot SNAPSHOT --dry-run --json change.saga
```

Use the returned deck or slide ID/URN, not its feature's ID. These commands
support slides authored with `apply-slide`. They append removal revisions and
retain historical assets; removed content disappears from active queries and
Technical Design. Removing a deck with transaction history retains its directory
with a deletion marker. Existing unrelated content stays active. External
relations left dangling are listed in the result.

For individual slides, obtain `authoring_snapshot` from `query slide` and pass
it as `--expected-snapshot` to refuse a newer edit. Omit that optional flag to
remove the current state under the Saga lock. Reconcile divergent histories
first, and remove any references in the deck overview before removing a slide.
Review slides remain protected. Upgrade readers before adopting deletion markers;
older readers may reject the retained history. Use a new ID for replacement content.

## Assign the existing board to ERD

First run `change-saga whiteboard list --json change.saga` and inspect the board
with `whiteboard describe --id ID`. Use the existing ID, regardless of its title:

```sh
change-saga whiteboard assign-role --id existing-data-model --role erd --expect CURRENT_ID_OR_none --dry-run --json change.saga
change-saga whiteboard assign-role --id existing-data-model --role erd --expect CURRENT_ID_OR_none --json change.saga
change-saga whiteboard list --json change.saga
change-saga validate --json change.saga
```

Technical Design shows the four role entries **ERD**, **Infrastructure**,
**Data flows**, and **Architecture**. ERD opens the assigned board. The board does not also appear as
a fourth sidebar entry. Other authored boards remain discoverable from the
Technical Design overview and retain their named URLs. Supporting essay pages
also remain in that overview and keep their direct links. Role assignment does
not publish a new board revision, duplicate its identity, or delete the board
previously occupying the role.

Replace `CURRENT_ID_OR_none` with the observed role owner, or `none` when empty.
A canonical whiteboard URN also identifies the observed owner, including a
board whose literal ID is `none`.
A stale expectation fails without changes. A board already assigned to another
role must be unassigned there first, or moved explicitly using
`--move --expected-snapshot SNAPSHOT` from the listing.

Use `unassign-role --id CURRENT_ID --role erd` to clear an assignment explicitly. Renaming a
board is independent of its role. Archiving hides an unassigned board from
ordinary discovery while preserving content, revision history, and exact links;
restore reverses it. Reassign or unassign a role before archiving its board.
Listings include archived boards with their archival state so authors can
find and restore them.

## Safety and concurrency

New metadata edits and moves accept `--dry-run` and `--json`. Mutations acquire
the Saga lock and use the existing transactional validation/rollback machinery
or atomic metadata replacement. Invalid containment, foreign Saga selectors,
and unmet dependencies are errors, not permission to cascade deletion.

Inspect removal output for contained targets and dangling relations. Resolve
the listed dependencies explicitly; never invent replacement references.
Coverage stays on the same exact semantic targets and review state remains
review-owned. History-bearing APIs retain their observed-parent or snapshot
checks; a lock serializes writes but does not replace those checks.
