# Change Saga format quick reference

## The Saga

There is one Saga format, version 5. `change-saga init` creates it; readers
reject any other version. Always use CLI commands and query targets; never
invent, rename, nest, glob, or infer meaning from storage files.

```text
app.saga/
  saga.json                        # identity and repository only
  ___overview/                     # pitch, description, and terms
  ___personas/  ___designsystem/  ___onboarding/  ___featureflags/
  ___epics/<epic>.epic/
    epic.json
    <chapter>.chapter/             # narrative content
    ___requirements/               # prototypes, stories, citations, relations
    ___design/                     # UX, UI, and technical design
    ___quality/                    # test cases and policies
    ___workplan/
    ___slides/<deck>.deck/         # the epic's implementation deck
  ___reviews/<id>.review/          # one pull request's review deck and decisions
  ___claims/  ___verifications/  ___merges/
```

`change-saga spec --json` publishes the resources, the legal relation endpoint
matrix, and every command shape. `change-saga status --json` returns the
coverage report by area (`coverage.areas`), per-criterion axis coverage, the
stale set, changed-source accounting, overview gaps, reviews, and ordered
`next_actions`; it has no verdict (see [ci.md](ci.md) for team rules).

No URN names its epic, so story, deck, slide, and Item IDs are unique across
the app and reorganizing epics breaks no link.

## Decks

Each `___slides/<deck>.deck/` bundle is an independently mergeable unit with
compact category-prefixed records: `10-d` decks, `20-s` slides, `30-i` Items,
and `40-e` evidence. Titles, parentage, and source paths live inside records,
never in filenames. Each slide owns one self-contained SVG, image, or HTML
file.

```sh
change-saga add-deck --epic checkout --objective "Explain the retry failure path." app.saga retry-flow
change-saga add-slide --deck retry-flow --intent trace --layout sequence --title "Retry sequence" --takeaway "The second write is conditional." app.saga retry-sequence
change-saga set-slide-content --target retry-sequence --source ./retry.svg app.saga
change-saga add-item --slide retry-sequence --kind callout --id hidden-retry --element-id hidden-retry --description "The retry reviewers may not expect." --body "The second write is conditional." app.saga
change-saga cover --against main --target hidden-retry --path internal/retry.go --side new --lines 40-52 --note "Makes the second write conditional." app.saga
change-saga query slide --saga app.saga --target retry-sequence
change-saga query slide-diffs --saga app.saga --target hidden-retry
```

Intents are `orient`, `explain`, `compare`, `trace`, `prove`, `risk`, and
`conclude`; layouts are `hero`, `diagram`, `before-after`, `sequence`,
`evidence`, `risk`, and `custom`. Item kinds are `node`, `edge`, `region`,
`transition`, `statement`, `risk`, `metric`, `example`, and `callout`.

Link a deck, slide, or Item to a story or criterion with a relation pinned to
the revision it relied on (an omitted pin defaults to the current head):

```sh
change-saga relation add --epic checkout --id retry-explains-safe-write --type explains \
  --from urn:change-saga:app:slide:retry-sequence \
  --to urn:change-saga:app:story:safe-write:criterion:no-duplicate \
  --rationale "The sequence explains how the criterion is implemented." app.saga
```

## Commands

```sh
change-saga install-skill
change-saga init --repo <source-checkout> --title "Title" app.saga
change-saga epic add --id <epic> --title "Title" app.saga
change-saga overview set-pitch --text "<pitch>" app.saga
change-saga overview set-description --source description.md app.saga
change-saga term add --id <term> --name "<name>" --definition "<definition>" --story <story> --ref 'HEAD:<path>#L<n>' app.saga
change-saga term revise --term <term URN> --revision r2 --parent <revision URN> --name "<name>" --definition "<definition>" --ref 'HEAD:<path>#L<n>' app.saga
change-saga add-chapter --epic <epic> --title "Title" app.saga <chapter>
change-saga add-section --epic <epic> --title "Title" app.saga <chapter>/<section>
change-saga add-fragment --epic <epic> --section <section> --type markdown --title "Context" app.saga
change-saga add-fragment --epic <epic> --section <section> --type html --source ./demo-package --entrypoint index.html app.saga
change-saga set-fragment-content --target <fragment> --source ./context.md app.saga
change-saga add-landmark --target <fragment> --element-id submit-action --label "Submit action" --description "The validated request crosses into persistence." app.saga
change-saga cover --against <base> --target <Item> --path file.go --side new --lines 4-9,12 --note "Adds request validation so malformed input fails before persistence." app.saga
change-saga cover --against <base> --target <Item> --path file.go --changed-lines --note "This file exists only to implement the demonstrated flow." --json app.saga
change-saga cover --target <fragment>#<landmark-id> --ref '<commit>:<path>#L<start>-L<end>' --note "Connects the citation to the renewal path." app.saga
change-saga add-claim --target <Item> --kind invariant --statement "Only one request can enter persistence for this key." --ref '<commit>:<path>#L<start>-L<end>' app.saga
change-saga verify-claim --claim <claim-id> --status verified --method test --summary "The concurrent request test passed." --command "go test ./..." app.saga
change-saga query gaps --saga app.saga --against <base> --kind uncovered
change-saga query mappings --saga app.saga --against <base> --sort scrutiny
change-saga replace-coverage --record <evidence_file> --batch replacements.jsonl app.saga
change-saga remove-coverage --record <evidence_file> app.saga
change-saga references --stale --diff app.saga
change-saga repin --onto <landed-commit> --branch <branch> app.saga
change-saga sync --repo <code-checkout> app.saga
change-saga query claims --saga app.saga --status unverified
change-saga validate --json app.saga
change-saga status --json app.saga
change-saga status --json --against <base> --head <head> app.saga
change-saga query layers --saga app.saga --against <base> --layer affected
change-saga query history --saga app.saga --node <urn>
change-saga query terms --saga app.saga --story <story>
change-saga open app.saga
change-saga open --against <base> app.saga
change-saga serve status app.saga
change-saga serve stop app.saga
change-saga review create --id pr-<n> --pr <n> --url <url> --base <branch> --head <pr-branch> app.saga
change-saga add-slide --review pr-<n> --intent explain --layout sequence --title "Why the queue moved" app.saga queue-move
change-saga add-item --review pr-<n> --slide queue-move --kind node --id table --element-id table --record <story-urn> --description "The new queue table." app.saga
change-saga cover --target <review Item URN> --path queue.go --changed-lines --note "Replaces SQS with a table." app.saga
change-saga review list --review pr-<n> --uncovered app.saga
change-saga review approve --review pr-<n> --slide queue-move --reviewer-kind human app.saga
change-saga review request-changes --review pr-<n> --slide queue-move --reviewer-kind ai --reviewer-name "Codex 1" --agent codex --model <exact model> --body "Explain the migration." app.saga
```

`--repo <checkout>` may be omitted when the Saga is inside the source checkout.
Flags precede positional arguments. Use `--json` for machine-readable mutation
summaries and `--quiet` when no successful output is needed.

## Identities

Targets are stable URNs:

```text
urn:change-saga:<saga>:saga
urn:change-saga:<saga>:deck:<deck>
urn:change-saga:<saga>:slide:<slide>
urn:change-saga:<saga>:slide:<slide>:item:<item>
urn:change-saga:<saga>:chapter:<chapter>
urn:change-saga:<saga>:section:<section>
urn:change-saga:<saga>:fragment:<fragment>
urn:change-saga:<saga>:fragment:<fragment>:landmark:<landmark>
urn:change-saga:<saga>:review:<review>[:slide:<slide>[:item:<item>]]
```

## Code references

A reference is `{commit, path, start, end, digest}`, where `start` and `end`
are absent for a whole file and `digest` is a SHA-256 of the exact referenced
bytes. It is the code as of that commit, never a diff; written as a location it
is `<commit>:<path>#L<start>-L<end>`. When later commits only move the lines,
it is remapped automatically; when they change, it is stale, and `change-saga
references --stale --diff` shows why. After a change lands, `change-saga repin
--onto <landed-commit> --branch <branch>` re-pins references to the landed
commit and records the branch's commit messages in `___merges/`, so a squash
merge keeps its reasoning. Each reference's `note` is shown, grouped by file,
before the reviewer expands its ranges.

## Claims and verification

`___claims/<id>.json` stores one falsifiable author assertion, its target, and
the code references that support it; claims never contribute to coverage.
`___verifications/<id>.json` stores one append-only result for a claim:
`unverified`, `verified`, `failed`, or `inconclusive`, plus the method, summary,
and optional reproducible command.

## Reviews

The Saga is documentation: its stories, designs, test cases, and decks carry no
approvals and no comments. A review is a pull request's slide deck under
`___reviews/<id>.review/`, one per pull request. Its base is what the pull
request merges into, and its head follows the pull request's branch until
`repin` freezes it after merge.

Decisions are per review slide (`approved`, `changes_requested`, or `none` to
withdraw), and comments attach to review slides and Items. Each decision and
comment is its own append-only file; never consolidate or rewrite them. A
decision records the reviewer and the head it was given at. The reviewer is
`human` or `ai`; an AI reviewer names a distinct seat, the agent, and the exact
model, so `Claude 1` and `Claude 2` stay independent on the same model. A
decision goes out of date when its slide, or the code its Items reference,
changed after the commit it was given at. Review coverage is computed over the
review's own range and never counts toward the documentation's coverage. The
tool never declares a review approved: it records decisions, and the team
decides what it requires.
