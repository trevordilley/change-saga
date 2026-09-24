# Change Saga AI instruction ergonomics

## Outcome

The shipped `change-saga` skill now loads a short mandatory safety/domain
contract and routes agents to focused references for bounded feature context,
feature handoff audit, visual QA, diagrams, stories, terms,
integration/recovery, CI, query navigation, and occasional format lookup. It
no longer requires every authoring task to preload the combined general,
query, format, and deck-authoring manuals.

The CLI packaging contract is unchanged: files under `skills/change-saga/`
are the authoritative sources embedded by `skills/embed.go`, and
`change-saga install-skill` emits those bytes with `SKILL.md` first. The
generated query-operation block in `references/query.md` remains owned by the
CLI query registry and its existing regeneration test.

## Mandatory contract and routing

`SKILL.md` retains the invariants that apply to every task:

- honor the requested authoring scope and separate authoring from review;
- use the installed CLI as the command and schema authority;
- read real metadata only through the paginated, snapshot-bound query API;
- mutate only through public commands;
- preserve immutable revisions, append-only events/evidence, competing heads,
  conflicts, relation pins, source identity, and requirement provenance;
- attach code to exact semantic Items or other supported focused targets; and
- treat coverage as omission detection rather than proof or permission to
  invent intent.

Conditional detail has one routed home:

| Task | Required files after the entrypoint |
| --- | --- |
| Existing Saga inspection | `references/query.md` |
| Compact feature context or feature handoff audit | `references/query.md` |
| Stories, criteria, personas, citations, relations, lifecycle | `references/query.md`, `references/stories.md` |
| Overview and terms | `references/query.md`, `references/terms.md` |
| Diagrams, decks, narrative, exact evidence, claims | `references/query.md`, `references/diagrams.md` |
| Mechanical visual QA without other Saga inspection | `references/diagrams.md` |
| Comparison reconciliation, PR integration, companion repositories, recovery | `references/query.md`, `references/integration.md` |
| The preceding integration task when visuals also change | add `references/diagrams.md` |
| CI policy without a real Saga read | `references/ci.md` |
| Resource/URN lookup not answered by CLI help/spec/schema | `references/format.md` |
| Parallel proposal comparison plus authorized withdrawal/consolidation | `references/query.md`, `references/integration.md`, `references/stories.md` |

The router explicitly rejects speculative capabilities: an agent discovers
support from the installed CLI and does not infer commands from plans or other
branches. The `apply-slide`, `preintegrate`, `story withdraw`, and `story
consolidate` guidance is marked as an integration dependency and cannot be used
unless the installed CLI advertises it. Its wording was checked against the
completed sibling branches, but the skill commit must land after those source
branches before release.

## Measured mandatory word counts

Counts below are `wc -w` results, not model token measurements. They support no
claim about measured token savings.

Baseline `7ec8e25` had 4,479 words in `SKILL.md`, 1,134 in `query.md`, 1,279 in
`format.md`, and 2,801 in `authoring.md` (9,693 combined). The routed skill
keeps a compatibility-only `authoring.md` under 100 words; no entrypoint route
loads it. Current source counts are: entrypoint 1,206; query 1,385; diagrams
1,607; stories 759; terms 536; integration 857; format 267; CI 662; and the
compatibility router 77.

| Realistic task | Baseline mandatory words | Revised mandatory words |
| --- | ---: | ---: |
| Inspect an existing Saga | 5,613 | 2,591 |
| Author a diagram/deck with exact evidence | 9,693 | 4,198 |
| Add or revise a sourced story | 6,892 | 3,350 |
| Revise project vocabulary | 6,892 | 3,127 |
| Reconcile and hand off a comparison without visual edits | 6,892 | 3,448 |
| Reconcile a comparison and revise its visual artifact | 9,693 | 5,055 |

Baseline story, term, and non-visual integration work required the entrypoint,
query reference, and format reference because all Saga changes were told to
read the format guide. Visual authoring additionally required the monolithic
authoring reference. Revised counts follow the explicit route table.

## Validation

The packaging-level routing test installs the embedded skill into a temporary
fixture and exercises realistic context, audit, visual-QA, diagram, story,
term, proposal-consolidation, integration/recovery, and CI requests. It verifies
that each route ships its required files and does not preload unrelated domain
references. It also bounds the compatibility-only `references/authoring.md`
router so it cannot grow back into a blanket manual.

Validation commands:

```sh
go test ./internal/cli -run 'Test(InstallSkill|Skill|InstalledSkill)' -count=1
python /path/to/skill-creator/scripts/quick_validate.py skills/change-saga
```

The broader repository test suite should also be run before merge. No Saga
metadata, lifecycle schema, CLI grammar, or query schema changes are part of
this work.
