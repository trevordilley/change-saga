# Deprecation annotations

A deprecation is an authored assertion with its own stable identity, immutable
revisions and lifecycle events. It does not edit or remove the content it names.
Use it to record why something should no longer be used, with an optional
replacement. It is independent of feature flags, technical-record retirement,
review verdicts and changed-line coverage.

## Choose the exact scope

Every assertion names one content target and explicitly chooses a scope:

- `content` deprecates that explanation, diagram, Item or depicted appearance.
  Any `--ref` values are related evidence, **not deprecated code**.
- `code` asserts deprecation of exactly the supplied `--ref` locations. At
  least one is required. Other code attached to the content target, reached
  through a story, or contained in its diagram is not implicitly deprecated.

A reference pins a full Git commit, repository-relative path, inclusive line
range (or an explicitly selected whole file), and digest of the exact bytes.
Moved unchanged code can resolve as current; changed, missing or unverifiable
bytes remain uncertain. These are authored claims, not proof of runtime behavior.

Supported targets use the existing canonical URNs for report fragments and
landmarks; decks, slides and Items (including review-scoped records); and
Components, Systems, data entities, ERDs and ERD overlays. Named whiteboards
use `urn:change-saga:SAGA:whiteboard:BOARD`. For a native or repeated whiteboard
appearance, add `--element PATH` using the path returned by `whiteboard describe`.
That path is local to the pinned board revision. To deprecate a reusable depicted
entity everywhere it is shown, name its technical identity instead.

Targets are **self-scoped**: no containment, story, documentation or replacement
link expands an assertion. A slide's reader may display annotations for its Items,
but each annotation continues to name its own target. A whiteboard may display
annotations for depicted source identities with their original pins.

## Author through the CLI

First discover identities with normal queries or `whiteboard describe`.
The writer pins the target's unique current revision or authored-content digest.
It refuses missing or conflicted targets. It never rewrites the target.

```sh
change-saga deprecation add --id old-explanation \
  --target urn:change-saga:APP:fragment:old-guide \
  --scope content --reason 'This guide describes the retired protocol.' \
  --replacement urn:change-saga:APP:fragment:new-guide change.saga

change-saga deprecation add --id legacy-parser \
  --target urn:change-saga:APP:slide:parsing:item:legacy \
  --scope code --ref 'HEAD:src/parser.go#L40-L61' \
  --reason 'Use the versioned parser for new integrations.' change.saga
```

Use `--repo PATH` when code lives in a separate checkout. Replacement links are
optional, independently pinned and checked for stale/missing/conflicted state.
For whiteboard replacement appearances, use `--replacement-element PATH`.
`--request-id ID` supports exact retries; `--json` returns created URNs and heads.
A source change may require a fresh revision rather than replaying an earlier
request against a different automatically selected target pin.

Stories are optional. Link an existing story or criterion with the normal
relation writer and traverse it with the normal relation query:

```sh
change-saga relation add --feature FEATURE --id parser-migration \
  --type explains --from urn:change-saga:APP:deprecation:legacy-parser \
  --to urn:change-saga:APP:story:migrate-parser \
  --rationale 'This migration explains the compatibility decision.' change.saga

change-saga query relations --saga change.saga \
  --from urn:change-saga:APP:deprecation:legacy-parser
```

The relation pins both revisions. An explicit historical story pin is allowed;
a story does not need to be active. Changed wording, competing heads, removed
criteria and retired stories remain visible. Do not invent a story just to make
an annotation, and do not delete a story to withdraw one.

## Discover and inspect

```sh
change-saga query schema deprecations
change-saga query deprecations --saga change.saga
change-saga query deprecations --saga change.saga --target TARGET_URN
change-saga query deprecations --saga change.saga --ref 'HEAD:src/parser.go#L45'
change-saga query deprecations --saga change.saga --story STORY_ID
change-saga query deprecations --saga change.saga --deprecation legacy-parser --history
```

The default includes active, withdrawn and conflicted records. Use `--state`
to narrow deliberately; do not exclude conflicts when auditing uncertain
assertions. Follow `page.next_cursor` until `page.has_more` is false and keep one
snapshot. `--scope content|code` filters the authored scope. `--head REV` selects
the source view for evidence health; `--ref` uses its own explicit commit.

Each row includes identity, current heads and lifecycle, every competing head's
assertion, authored target pins and health, optional replacement health, exact
code evidence/provenance and story relation currency/lifecycle. `--history`
requires one annotation and includes all immutable revisions and events.

In a code query, `deprecated_code` names explicit code-scope evidence;
`related_code` and `related_target_code` never assert deprecation. The latter
comes from direct evidence on the target, without following containment or
other links. `match: exact` means the selected bytes overlap verified current
evidence. `uncertain_path` deliberately retains stale references to the same
recorded path; it does not establish a match to the current lines. A provisional
resolution, stale target, conflicting head or withdrawn lifecycle must not be
reported as a confirmed current code deprecation. Consult every field together.
No rows means **no recorded deprecation**, not proof of non-deprecation.

Global and target discovery work without a source checkout and report that code
was not inspected. Code-focused queries fail explicitly when source inspection
is unavailable. `references` also lists active, unconflicted deprecation evidence;
use `deprecation revise`, not in-place repinning, to correct it.

Readers show a compact **Deprecations · N** badge beside content. Hover or focus
previews the reason; clicking opens the shared details drawer. Full scope,
replacement, target health, exact code evidence and provenance load on demand.
Whiteboards keep the badge beside their title and mark individual appearances
when the annotation names that saved board revision. A whole-board assertion
does not mark every element. Multiple assertions at one appearance share a marker.

A page's Deprecations link lists its records, including content farther down the
page. The **Deprecations** project destination searches the whole Saga, filters
by content/code scope and active/withdrawn/conflicted state, and pages through
50 records at a time. Entries link to affected content; appearance links retain
the authored board revision. Withdrawn and competing assertions remain discoverable.

The reader is read-only. Historical content is still available; annotation pins
and current-target health are reported separately. Hidden slides do not fetch
annotations, and hover does not resolve code or load full details. The CLI query
continues to enumerate the complete set for agents.

## Correct or withdraw an assertion

`revise` appends a complete replacement assertion; explicitly repeat the desired
scope, target, reason, code and optional replacement. Omitting optional values
removes them from the new snapshot only. Supply **every** observed revision head
with `--parent`; multiple parents explicitly reconcile a merge conflict.

```sh
change-saga deprecation revise \
  --deprecation urn:change-saga:APP:deprecation:legacy-parser \
  --revision r2 --parent urn:change-saga:APP:deprecation:legacy-parser:revision:r1 \
  --target urn:change-saga:APP:slide:parsing:item:legacy --scope content \
  --reason 'Correction: only the explanation is obsolete.' change.saga

change-saga deprecation set-state \
  --deprecation urn:change-saga:APP:deprecation:legacy-parser \
  --event withdrawn --parent urn:change-saga:APP:deprecation:legacy-parser:event:active \
  --state withdrawn --reason 'This was a mistaken deprecation.' change.saga
```

Lifecycle parents are event URNs, not revision URNs. Restoration uses a new event
with `--state active`, all current event parents and an authored reason. Neither
operation deletes history or changes historical code/story links. A corrected
revision does not silently restore a withdrawn annotation.

`change-saga validate` checks strict schemas and history graphs. Missing or changed
targets after authoring are query health, not a reason to erase the annotation.
Older readers may reject the new `___deprecations` reserved root or deprecation
relation endpoints and must be upgraded before reading these records.
