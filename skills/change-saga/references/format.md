# Change Saga format quick reference

## The Saga

There is one Saga format. `change-saga init` creates it; readers reject any
other version.

```text
<name>.saga/
  saga.json                        # version 5
  overview.fragment/               # report content
  ___requirements/
    prototypes/  stories/  citations/  relations/  coverage-exceptions/
  ___design/                       # technical design
  ___slides/<deck-id>.deck/        # the implementation deck
  ___workplan/
  ___quality/
    policies/  test-cases/<id>.test/
  ___claims/  ___verifications/  ___merges/  ___reviews/
```

`change-saga spec --json` publishes the resources, the legal relation endpoint
matrix, and every command shape. `change-saga status --json` returns the gates,
per-criterion axis coverage, the stale set, changed-source accounting, and
ordered `next_actions`; follow them until no required gap remains.

## Implementation deck

The deck is the Implementation section of the Saga. Keep requirements,
acceptance criteria, prototypes, design, and work-plan history in their own
surfaces; the deck explains the implemented change. Each
`___slides/<deck-id>.deck/` bundle contains one `role: change` deck and its
flat slide/Item/evidence records, and every target URN uses the Saga ID:
`urn:change-saga:<saga>:deck:<deck>`, `urn:change-saga:<saga>:slide:<slide>`,
and `urn:change-saga:<saga>:slide:<slide>:item:<item>`.

```sh
change-saga add-deck --epic checkout --objective "Explain the retry failure path." checkout.saga retry-flow
change-saga add-slide --deck retry-flow --intent trace --layout sequence --title "Retry sequence" checkout.saga retry-sequence
change-saga set-slide-content --target retry-sequence --source ./retry.svg checkout.saga
change-saga add-item --slide retry-sequence --kind callout --id hidden-retry --element-id hidden-retry --description "The retry reviewers may not expect." --body "The second write is conditional." checkout.saga
change-saga cover --target hidden-retry --path internal/retry.go --side new --lines 40-52 --note "Makes the second write conditional." checkout.saga
change-saga query slide --saga checkout.saga --target retry-sequence
change-saga query slide-diffs --saga checkout.saga --target hidden-retry
```

Only an Item may own slide coverage. Items include `callout`; a callout can
name another Item with `about` and can own its own exact diff atoms. Approval
decisions target slides only. Items remain valid targets for comments,
annotations, evidence, claims, and deep links, but not approvals; deck status
is derived from its slides. Read deck content with `query slide` and Item
evidence with `query slide-diffs`, never through `query fragment`.

Requirements are the traceability root. Link a deck, slide, or Item to a story
or criterion with an active relation pinned to the story revision it relied
on. The source may be broad, but exact code evidence remains Item-owned. Story
links apply to every criterion in the pinned revision.

```sh
change-saga relation add --id retry-covers-safe-write --type explains \
  --from urn:change-saga:checkout:slide:retry-sequence \
  --to urn:change-saga:checkout:story:safe-write:criterion:no-duplicate \
  --to-revision urn:change-saga:checkout:story:safe-write:revision:r1 \
  --rationale "The sequence explains how the criterion is implemented." \
  checkout.saga
change-saga query traceability --saga checkout.saga --ref '<commit>:<path>#L<start>-L<end>'
```

The traceability response includes paths from accepted criteria through review
targets to code references and `unlinked_code_evidence` for Item evidence that
has no current story path. `--ref` looks up the targets that reference a code
location, and `--commit` selects evidence pinned at that commit.

## Report content

### Layout

Report content belongs to an epic (or to the application's `___overview` and
`___designsystem`):

```text
<name>.saga/
  saga.json
  ___claims/
    <claim-id>.json
  ___verifications/
    <verification-id>.json
  ___reviews/<id>.review/
  ___epics/<epic>.epic/
    epic.json
    overview.fragment/
      fragment.json
      content.md
      ___code/
    <chapter>.chapter/
      chapter.json
      overview.fragment/
      <section>/
        section.json
        <demo>.fragment/
          fragment.json
          index.html
          app.js
          ___landmarks/
            submit-action.landmark/
              landmark.json
              ___code/
```

`.chapter` directories directly inside an epic are independently reviewable chapters.
Ordinary directories inside them are recursive sections. `.fragment` directories are atomic content
packages. A fragment manifest declares `version`, stable `id`, `media_type`,
`entrypoint`, and optional `title`/`order`. Supported content includes Markdown,
plain text, HTML, SVG, and raster images. Bundle HTML dependencies inside its
fragment directory; do not rely on network access in the sandboxed viewer.

Give every Markdown heading an explicit stable anchor:

```markdown
## Request validation {#request-validation}
```

Anchors begin with a lowercase letter and contain only lowercase letters,
digits, and hyphens. They are unique within a fragment and remain unchanged when
the visible heading is edited. The renderer combines the fragment target with
the authored anchor to create a collision-free permalink.

Addressable subparts use independent landmark packages:

```json
{
  "version": 2,
  "id": "submit-action",
  "label": "Submit action",
  "description": "The validated request crosses into persistence.",
  "selector": { "type": "element", "element_id": "submit-action" }
}
```

Store the record at `___landmarks/<id>.landmark/landmark.json`. Selector types
are `heading` for an explicit Markdown anchor, `element` for an HTML/SVG element
ID, `text` for an exact Markdown/plain-text quote, and `region` for normalized
image coordinates. Put each code association in its own `___code/*.json`
inside the package. SVG element landmarks infer their on-canvas hover controls
from the rendered element bounds. A normalized `hotspot` overrides that
geometry when needed; raster regions use normalized coordinates directly.
Meaningful visual landmarks should include a semantic `description`; query
clients receive it so they do not need to interpret raw SVG or HTML geometry.

## Commands

```sh
change-saga install-skill
change-saga init --repo <source-checkout> --title "Title" <name>.saga
change-saga epic add --id <epic> --title "Title" <name>.saga
change-saga add-chapter --epic <epic> --title "Title" <name>.saga backend
change-saga add-section --title "Title" <name>.saga backend.chapter/path/to/section
change-saga add-fragment --section path/to/section --type markdown --title "Context" <name>.saga
change-saga add-fragment --section path/to/section --type html --source ./demo-package --entrypoint index.html <name>.saga
change-saga set-fragment-content --target path/to/context.fragment --source ./overview.md <name>.saga
change-saga add-landmark --target path/to/demo.fragment --element-id submit-action --label "Submit action" --description "The validated request crosses into persistence." <name>.saga
change-saga add-landmark --target path/to/context.fragment --heading-id request-validation --label "Request validation" <name>.saga
change-saga add-landmark --target path/to/context.fragment --id lease-renewal --text "Renewal is triggered from the heartbeat path before the lease midpoint." --label "Lease renewal evidence" <name>.saga
change-saga cover --repo <source-checkout> --target path/to/demo.fragment --path file.go --side new --lines 4-9,12 --note "Adds request validation so malformed input fails before persistence." <name>.saga
change-saga cover --repo <source-checkout> --target path/to/demo.fragment --path file.go --changed-lines --note "This focused file exists only to implement the demonstrated request flow." --json <name>.saga
change-saga cover --target path/to/demo.fragment --ref '<commit>:<path>#L<start>-L<end>' --note "Implements the behavior explained by this fragment." <name>.saga
change-saga cover --target path/to/demo.fragment/___landmarks/submit-action.landmark --ref '<commit>:<path>#L<start>-L<end>' --note "Connects the diagram action to its exact submit handler." <name>.saga
change-saga cover --target path/to/context.fragment#lease-renewal --ref '<commit>:<path>#L<start>-L<end>' --note "Connects the prose citation to the renewal scheduling path." <name>.saga
change-saga add-claim --target path/to/demo.fragment#submit-action --kind invariant --statement "Only one request can enter persistence for this key." --ref '<commit>:<path>#L<start>-L<end>' <name>.saga
change-saga verify-claim --claim <claim-id> --status verified --method test --summary "The concurrent request test passed." --command "go test ./..." <name>.saga
change-saga query mappings --saga <name>.saga --repo <source-checkout> --sort scrutiny
change-saga replace-coverage --record <evidence_file> --batch replacements.jsonl --repo <source-checkout> <name>.saga
change-saga remove-coverage --record <evidence_file> <name>.saga
change-saga references --stale --diff --repo <source-checkout> <name>.saga
change-saga repin --onto <landed-commit> --branch <branch> --repo <source-checkout> <name>.saga
change-saga query claims --saga <name>.saga --status unverified
change-saga validate --json <name>.saga
change-saga status --json --repo <source-checkout> <name>.saga
change-saga status --json --repo <source-checkout> --against <base> --head <head> <name>.saga
change-saga query layers --saga <name>.saga --against <base> --layer affected
change-saga query history --saga <name>.saga --node <urn>
change-saga open --repo <source-checkout> <name>.saga
change-saga serve status <name>.saga
change-saga serve stop <name>.saga
change-saga review create --id pr-<n> --pr <n> --url <url> --base <branch> --head <pr-branch> <name>.saga
change-saga add-slide --review pr-<n> --intent explain --layout sequence --title "Why the queue moved" <name>.saga queue-move
change-saga add-item --review pr-<n> --slide queue-move --kind node --id table --element-id table --record <story-urn> --description "The new queue table." <name>.saga
change-saga cover --against <branch> --target <review Item URN> --path queue.go --changed-lines --note "Replaces SQS with a table." <name>.saga
change-saga review approve --review pr-<n> --slide queue-move --reviewer-kind human <name>.saga
change-saga review request-changes --review pr-<n> --slide queue-move --reviewer-kind ai --reviewer-name "Codex 1" --agent codex --model gpt-5.6-sol --body "Explain the migration." <name>.saga
change-saga review list --review pr-<n> <name>.saga
```

`--repo` may be omitted when the saga is inside the source checkout. Flags
precede positional arguments. Without `--against`, a command observes the Saga
at its head; with it, the command compares the merge-base of the two revisions
through the head, as a pull request does. Coverage describes omission only, not
correctness or explanation quality.

## Claims and verification

`___claims/<id>.json` stores one falsifiable author assertion, its narrative
target, and the code references that support it. Claims never contribute to
coverage. `___verifications/<id>.json` stores one append-only result for a
claim: `unverified`, `verified`, `failed`, or `inconclusive`, plus the method,
summary, and optional reproducible command. Git attribution identifies who
committed each independent record.

## Portable identities

Targets are stable URNs:

```text
urn:change-saga:<saga-id>:saga
urn:change-saga:<saga-id>:chapter:<chapter-id>
urn:change-saga:<saga-id>:section:<section-id>
urn:change-saga:<saga-id>:fragment:<fragment-id>
urn:change-saga:<saga-id>:fragment:<fragment-id>:landmark:<landmark-id>
```

Evidence contains code references: `{commit, path, start, end, digest}`, where
`start` and `end` are absent for a whole file and `digest` is a SHA-256 of the
exact referenced bytes. A reference is the code as of that commit, never a
diff. When later commits only move the referenced lines, the reference is
remapped automatically; when the lines change, it is stale, and `change-saga
references --stale --diff` shows why. Never hand-edit a reference to make stale
evidence pass; re-author it with `replace-coverage`. After a change lands,
`change-saga repin --onto <landed-commit> --branch <branch>` re-pins references
to the landed commit and records the branch's commit messages.

Each evidence reference should include a concise `note` explaining what changed
and why the narrative target owns that code. The Saga drawer groups references
by source file and displays these notes before the reviewer expands the linked
ranges.

## Reviews

The Saga is documentation: its stories, designs, test cases, and decks carry no
approvals and no comments. A review is a pull request's slide deck under
`___reviews/<id>.review/`, one per pull request. Its base is what the pull
request merges into and its head follows the pull request's branch. The review
deck explains what the change did and why; its Items reference the code the
change touched, shown as a diff against the base, and may reference the records
it revised.

Decisions are per review slide (`approved`, `changes_requested`, or `none` to
withdraw) and comments attach to review slides and Items. Each decision and
comment is its own append-only file; never consolidate them into shared files or
rewrite them. A decision records the reviewer and the pull request head it was
given at. The reviewer is `human` or `ai`; an AI reviewer names a distinct seat,
the agent, and the exact model, so `Claude 1` and `Claude 2` stay independent
even on the same model. Git supplies the authoritative author identity, and a
reviewer's latest decision on a slide is current.

A decision goes out of date when its slide, or the code the slide's Items
reference, changed after the commit it was given at; `review list` and `status`
report each decision's currency. The tool never declares a review approved: it
records decisions and the team decides what it requires. After the change
lands, `repin` freezes the review at its exact base and head.
