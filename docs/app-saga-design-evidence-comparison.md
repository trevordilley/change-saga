# App Saga design: evidence, currency, and comparison

Status: canonical UX and technical design for the evidence and comparison
requirements accepted on `main` at `f3474a0`. This work changes design and
design-owned relations only. It does not add code references, quality evidence,
implementation slides, requirements, stories, or personas.

## Inputs and scope

The design reconciles three sources of truth:

- the unique current accepted requirement heads loaded from `app.saga` on
  `main`;
- the merged capability audit at `4f4d5b1`, especially its separation of human
  outcomes, design invariants, and replaceable implementation mechanics; and
- the merged code-and-test audit at `3d9f11a`, especially its evidence,
  comparison, negative-path, and traceability findings.

The requirements audit included by the accepted-requirements merge was used to
avoid linking new design to retired technical stories. The design targets only
current criteria from `evidence-traversal`, `evidence-repair`,
`claims-verification`, `companion-repositories`, `observe-or-compare`, and
`why-things-changed`.

All Saga mutations were made through the repository's current CLI. Relations
are stored with their source design: cross-feature evidence-traversal links from
evidence design live under `code-evidence`, while impact traversal links from
comparison design live under `comparison`.

## Authored design

| Feature | Chapter | Addressable design |
| --- | --- | --- |
| `code-evidence` | **Evidence, currency, and repair** | End-to-end forward and reverse evidence traversal; hop-level health; impact and visible gaps; current/remapped/stale/missing states; stale diagnosis; scrutiny-ranked mappings; atomic repair; post-merge refresh; claims and independent append-only verification; verified companion checkouts, sync state, dual-repository comparison, existing-code adoption, and later relocation. |
| `code-evidence` | **Code evidence technical model** | Preserved and expanded reference identity, source-side and file-event rules, stable resolver outcomes, landed-source rebinding, and an explicit boundary around replaceable resolution algorithms. |
| `comparison` | **Current state, comparison, and history** | Persistent mode identity; one merge-base opening context for browser, status, and queries; reachable current documentation; derived related reviews; Changed/Affected/Code layers; cross-layer evidence traversal; conservative replacement pairing; effect-local commit reasons; node history; and squash-safe reason preservation. |

Thirty-seven heading landmarks make the consequential decisions independently
addressable. The text diagrams in the traversal and layer fragments are used
only where flow benefits from a visual overview; no implementation deck or
review slide was added.

## Canonical interaction decisions

### Evidence is a navigable argument

A reviewer can start at a person, story, criterion, design landmark, test, or
source location and traverse in either direction. The UI exposes authored
adjacent hops and derived longer paths separately. Context survives a jump and
back navigation returns to the same expanded criterion or selected source
range.

Each hop, not just the overall chain, is current, missing, or stale. A stale
path remains inspectable at its last-confirmed endpoint and stops claiming
current support. Test result and link currency remain independent dimensions.

### Remapped is current, but visible

The public evidence states are **Current**, **Current · remapped**, **Stale**,
and **Missing**. A content-preserving move to one unambiguous target remains
current and reveals both old and resolved locations. Changed, ambiguous, or
unverifiable content is stale. Missing means no link was authored; it never
causes the system to guess.

The resolver's implementation may change. Durable behavior is defined by the
four outcomes, digest verification, unambiguous equivalence, bounded work, and
reproducible provenance—not by today's Git-diff or digest-search sequence.

### Repair is diagnostic and atomic

A stale evidence drawer shows captured source, the viewed candidate, the
smallest safe comparison, pin and viewed revisions, and a concrete reason. The
mapping queue raises broad, overlapping, stale, or thinly justified evidence
as scrutiny signals rather than truth scores.

Replacement previews the complete new evidence record and commits replacement
plus retirement atomically. Post-merge refresh previews unchanged, remapped,
stale, and unresolvable records before rebinding safe ones to landed source.
Squashes and deleted branches retain branch reasons; ambiguous records keep
their old pins and remain stale.

### Claims are not verification

An author claim contains one falsifiable assertion, one addressable explanation
target, and exact supporting source. An independent verification appends one
verified, failed, or inconclusive result with its method and summary. Earlier
results, including failures, remain visible. A new result neither rewrites the
claim nor silently becomes a single authoritative status.

### Companion repositories share the same evidence model

Every source-derived operation verifies the selected checkout against the
Saga's declared canonical repository before reading or writing. The UI shows
the documented source revision separately from each evidence record's health.
Comparison aligns the Saga range and source range through the sync cursor and
stops when the alignment is missing or ambiguous.

Current-state mode can document existing source without manufacturing a code
change. Moving the Saga beside its source preserves Saga identity, URNs, source
pins, digests, and history; only checkout selection becomes implicit.

### Current state and comparison are explicit modes

Current state is the default living product truth. Comparison is a selected
merge-base-to-head range. A persistent accessible label, resolved commits, and
deep-link state prevent one mode from silently inheriting the other. The same
resolved opening context scopes the reviewer, status, and structured query
surfaces.

Comparison de-emphasizes unchanged material but never makes current
documentation unreachable. Documentation remains read-only; review discussion
and decisions stay on review slides.

### Changed, Affected, and Code answer different questions

- **Changed** is the semantic delta of authored Saga records, including adds,
  revisions, moves, retirements, and replacement pairs.
- **Affected** is unchanged knowledge put at risk by changed source or mutable
  endpoints. Every row explains the originating change and whether impact is
  current or uncertain behind a stale edge.
- **Code** groups each changed atom under every current explanation that
  references it and separately lists unexplained atoms. Stale evidence remains
  context but does not count as coverage.

Selection and filters survive movement between layers. “Show code” and “Show
impact” use the same evidence graph as forward and reverse traversal.

### History explains transitions without polluting current truth

Replacement inference requires one unambiguous shared traceability role;
otherwise an explicit replacement relation is required. Title similarity,
filesystem proximity, and timestamps are insufficient.

Commit reasons sit beside the records or source groups they touched. Every
addressable node shows when it was introduced, what it replaced when known, and
the frozen comparison that introduced it. Post-merge metadata retains ordered
branch commit reasons beside landed source identity after a squash.

## Traceability and gap results

The CLI created 63 new pinned `addresses` relations. Criterion targets defaulted
to the unique current story revision and design sources defaulted to the exact
current content digest. The one retained chapter-to-story relation is genuinely
cross-cutting: the code-reference technical model still addresses the complete
evidence-repair story. It was re-pinned after the chapter was reorganized; the
criterion-specific behavior lives on landmarks.

Targeted traceability queries reported:

| Current accepted story | Criteria queried | Criteria without design |
| --- | ---: | ---: |
| `evidence-traversal` | 10 | 0 |
| `evidence-repair` | 7 | 0 |
| `claims-verification` | 9 | 0 |
| `companion-repositories` | 6 | 0 |
| `observe-or-compare` | 10 | 0 |
| `why-things-changed` | 7 | 0 |
| **Total** | **49** | **0** |

Before this work, design status covered 4 of 17 accepted stories. Afterward it
covers 8 of 17: the four newly closed story gaps are `evidence-traversal`,
`claims-verification`, `companion-repositories`, and `why-things-changed`.
`evidence-repair` and `observe-or-compare` were already counted through broad or
adjacent design, but now have criterion-level design. The remaining nine story
gaps belong to other feature owners.

## Validation

The final checks used the repository's current `0.2.0-dev` CLI:

```text
/tmp/change-saga-design-evidence validate --json app.saga
/tmp/change-saga-design-evidence relation status --json app.saga
/tmp/change-saga-design-evidence query traceability --saga app.saga --requirement <each-target-story> --limit 100
/tmp/change-saga-design-evidence status --json app.saga
/tmp/change-saga-design-evidence query gaps --saga app.saga --kind uncovered --limit 100
go test ./internal/cli ./internal/requirements ./internal/saga
```

Results:

- validation: valid, with zero issues and zero fixes;
- relation currency: 90 total relations, 74 current, 0 stale; the remainder are
  intentionally superseded history;
- targeted traceability: 49 current criteria, 0 without design;
- design status: 8/17 accepted stories covered, 9 unrelated gaps remaining;
- implementation-gap query: 0 uncovered atoms in current-state mode;
- targeted CLI, requirements, and Saga package tests: passed; and
- no code reference was created or changed by this design work.

## Structured CLI dogfooding findings

| ID | Goal and exact command | Observed result | Impact and workaround | Smallest useful improvement |
| --- | --- | --- | --- | --- |
| CLI-1 | Validate before mutation: `change-saga validate --json app.saga` | The installed `0.1.1` binary returned `unsupported Saga version 5; expected 2, 3, or 4` plus unknown-root errors. The repository CLI reports `0.2.0-dev`. | **Blocked.** Built the checkout's CLI to `/tmp/change-saga-design-evidence` and used it for every mutation and final check. | When a checkout contains a newer local CLI, report the executable/version mismatch and the exact local invocation before format diagnostics. |
| CLI-2 | Read accepted heads as structured output: `go run ./cmd/change-saga query requirements --saga app.saga --state accepted --json` | The command returned `invalid flags for requirements: flag provided but not defined: -json`; query output is already JSON. | **Minor interruption.** Removed `--json` and re-ran. | Accept `--json` as an idempotent compatibility flag on `query`, or make the error say that query is always JSON. |
| CLI-3 | Query design coverage: `go run ./cmd/change-saga query schema design-coverage` and `go run ./cmd/change-saga query design-coverage --saga app.saga` | Both returned unknown-operation errors even though `docs/living-saga-api.md` advertises `query design-coverage`. | **Slowed gap analysis.** Parsed `.coverage.areas.design` from the much larger `status --json` response. | Implement the documented paginated operation, or remove it from the contract and add a supported `status --area design --json` projection. |
| CLI-4 | Query uncovered design work: `/tmp/change-saga-design-evidence query gaps --saga app.saga --kind uncovered --limit 100` | It returned zero gaps while status reported nine design gaps. `query schema gaps` clarifies that the operation means uncovered code atoms, stale selectors, and overlap—not documentation-area gaps. | **Ambiguous command vocabulary.** Used status for design gaps and traceability for criterion coverage. | Add `--area implementation|stories|personas|design|quality|health`, or rename this operation `mapping-gaps` and provide a dedicated design-gap query. |
| CLI-5 | Create 63 pinned design-to-criterion relations: repeated `/tmp/change-saga-design-evidence relation add --feature <owner> --id <id> --type addresses --from <landmark> --to <criterion> --rationale <text> --json app.saga` | Each mutation was clear and safely defaulted the current revision and content-digest pins, but no batch mode exists. | **Slow and interruption-sensitive.** Ran a fail-fast loop, then checked all relation currency, all target traceability, and full validation. | Add atomic `relation add --batch FILE|- --dry-run` with per-record results and all-or-nothing writes. |
| CLI-6 | Automate design creation: `design add-chapter`, `design add-fragment`, and `add-landmark` | These mutations emit human guidance only and expose no `--json`, while `set-fragment-content`, revisions, and relation mutations support structured output. | **Reduced scriptability.** Used explicit stable IDs, discarded success prose, and relied on validation/query checks for confirmation. | Give every mutation the common structured envelope, `--json`, request ID, dry-run, created paths, and defaulted values. |

The CLI's strongest behavior in this pass was relation pinning and currency: it
reported every defaulted revision/content pin, identified the one intentionally
staled broad relation after design edits, and recorded its confirmation as an
append-only re-pin rather than rewriting history.
