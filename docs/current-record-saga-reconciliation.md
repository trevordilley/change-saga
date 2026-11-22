# Saga reconciliation for the current-record release

This upkeep reassesses the canonical Saga against the implementation inherited by
`fix/plan-cli-compatibility-support-access`. The starting release health report
had 382 findings: 241 code references, 76 quality evidence records, 39 relations,
20 test definitions and 6 terms. A currency check alone does not establish that
an explanation is correct, so the repair also changes obsolete explanations and
assertion mappings.

## Semantic decisions

| Area | Reassessment |
| --- | --- |
| Storage and story writes | A stable package contains current definition/state; Git owns versions and ordinary same-file conflicts. Already-current story edits avoid unrelated legacy normalization. Other writers retain their existing normalization boundary. |
| Legacy ambiguity | A compatibility loader may expose competing legacy candidates. Normalization refuses ambiguity and converts required exact pins before removing legacy sources; a current author never supplies a new Saga reconciliation child. |
| Relative edits | `diagram edit` targets diagram-sourced slides and requires the observed current digest. Repeating an old relative move is refused. Complete whiteboard publication is a separate full replacement. |
| Quality definitions | Enrichment preserves current definition bytes. Editing or reintroducing a step does not inherit old evidence. Superseding mappings retain earlier immutable evidence; current coverage describes verification intent, not execution or delivery. |
| Actual facts | Recorded runs, quality policy decisions, reviews and comments retain their own provenance. Their real parent/supersession relationships are not mistaken for a content revision ledger. |
| Prototypes | Definitions are current; copied HTML assets are content-addressed. An annotation preserves exact endpoint digests and never follows changed endpoints implicitly. |
| Inventory | Current technical content/state and exact Git-backed historical content are separate. A proposal baseline must resolve exact implemented content of the same identity; no current parent ancestry is inferred. |
| Whiteboard evidence | Native element ownership remains independent of review coverage. Current-page actions reject missing/invalid digests with 400 and changed content with 409. Explicit saved Git context may retrieve original content; unavailable content is 404. |
| Compatibility | Wave/phase Plan layout, phased authoring, Plan queries, native checkpoint fields and whole-Saga current-record storage remain separate capabilities. This upkeep does not change those capabilities or make an older released reader compatible. |

The term `revision-head` remains retired. Its compatibility explanation points to
the legacy refusal boundary, not a new current-record workflow. Human acceptance,
term maturity/evidence axes, run results and review decisions are not inferred or
advanced by this work. Test-definition automation distinguishes the manual Git
merge scenario from executable assertion mappings; a legacy fork test is not
presented as an automated Git merge test.

Canonical Saga writes use the public authoring paths: term/test-case revision,
full `apply-slide` requests, legacy Item/asset updates, scoped coverage
replacement, immutable evidence addition with explicit supersession, relation
repinning with a new rationale, overview/design authoring and complete
whiteboard publication. All 365 original evidence records are unchanged in the public historical projection. All 182 case identities and ordered step-ID lists are preserved. Existing evidence and unrelated exact pins are retained.
The Architecture board keeps its original name, IDs, geometry, source imports
and unrelated elements; only the inspected presentation elements are changed.

The CLI repair guidance itself still suggested removed whiteboard parent flags.
`bbd0521f` fixes that route and the managed-slide head-reconciliation wording.
A regression assertion checks that the public guidance describes complete
current publication and Git history.

## Verification and limits

A synthetic local repository exercised the public authoring commands and real
Git merges: independent story edits merged cleanly; conflicting edits produced
one actual `current.json` conflict; deliberate combined resolution retained both
original branch commits. The public history query exposed both branches and the
resolution. Stable story identity and criterion content survived, and the final
tree contained no Saga revision/event directories. This is integration evidence
for ordinary Git composition, not a claim about every possible conflict.

Serial Go race checks cover `currentrecord`, `quality`, `requirements` and focused
CLI/server tests for complete publication, repeated relative edits, current-page
digest refusal and shared whiteboard page snapshots. Rendered assets were inspected directly. The actual reader graphic was then
loaded and inspected at 1280px and 1024px with all six Item elements visible and
no horizontal overflow. The representative graphic loaded in about four seconds.
The stock `visual-qa` runner timed out twice waiting for network idle; those are
failed tooling attempts, not reported as successful visual-qa runs. The explicit
reader/iframe check supplied the screenshots and loaded-element assertions. The final pre-commit health check passed all 2,319 records with zero findings.
Validation passed with 235 advisory warnings. Public overview snapshots before
and after health/validation were identical, confirming the checks did not
rewrite Saga content. Twenty-one current test definitions and 80 flat relations
were reassessed; three native board criterion links were deliberately updated.
Ninety-one new immutable mapping records explicitly supersede earlier records
(including four final draft-mapping corrections); no original evidence was
rewritten. Legacy run/history queries exposed no recorded runs in this Saga.

An all-Saga diagram renderer check also reports an inherited mismatch in
`pr-21:slide:timing`: its SVG was published by renderer version 1. The same
mismatch is present in an untouched committed snapshot. It is reported
separately from release health; this upkeep does not rewrite that review.

No deployment, IAM access, release publication or customer-data agreement is
performed here. The support-access proposal's approval owner, grant/revoke
owner, authorized scope and any broader audited access agreement remain human
decisions.
