# Technical inventory: records and authoring implementation

Status: implemented on `feature/inventory-records-authoring-claude`. This note
covers the canonical record model, format gate and public writers only. Query,
path/coverage/reconcile projections and the renderer/ERD UI are owned by the
sibling slices and consume these records. The design and policy decisions are
in [the design](technical-inventory-design.md) and
[policy contract](technical-inventory-policy-contract.md); the persisted
contract is the "Technical inventory format 2" section of [SPEC.md](../SPEC.md).

## What a user can do

```sh
change-saga inventory adopt-format --format 2 app.saga            # explicit, once
change-saga component add --id store --from proposed.json app.saga # intent: proposed, baseline: none
change-saga component revise --id store --revision r2 \
  --parent urn:…:component:store:revision:r1 \
  --from implemented.json --delivery HEAD app.saga                 # evidence checked at delivery
change-saga data-entity add --id pdf-report --from report.json --delivery HEAD app.saga
change-saga erd add --id application --from erd.json --visual erd.svg app.saga
change-saga erd-overlay add --id pdf-jobs --from overlay.json --visual overlay.svg app.saga
change-saga add-item --slide S --kind node --element-id n --description … \
  --documentation urn:…:system:x --documentation-revision urn:…:system:x:revision:r1 \
  --selections selections.json app.saga                           # exact subset through the System
change-saga add-item … --documentation-revision urn:…:component:store:revision:r1 \
  --documentation-view <saved commit> app.saga                     # historical pin, explicit view
```

The same Item options work on review decks (`--review ID`) and in `apply-slide`
item JSON (`documentation_view`, `selections`).

## Delivered behavior

- **Format gate** (`internal/requirements/inventory_validate.go`): the
  `___inventory/format.json` marker is the only switch. Without it, intent,
  evidence IDs, new kinds and format-2 Item content are refused on read and
  write. Adoption rewrites nothing; after it, new Component/System revisions
  must state intent. `saga.SagaVersion`, review records and record `version`
  fields are unchanged.
- **Records** (`internal/requirements/inventory.go`): one `TechnicalDefinition`
  with kind-disjoint fields; `Evidence{ID, coderef.Reference}` for all code.
  `LoadInventory` returns every kind in deterministic order (kind, then id)
  and validates each record structurally, including baseline ancestry and the
  pinned SVG's digest and safety. SVG safety is an allowlist of static elements
  and presentation attributes because the renderer inlines the visual.
- **Writer** (`internal/requirements/inventory_mutation.go`,
  `WriteTechnicalRevision`): under the Saga lock it checks all-head parents,
  idempotent replay, candidate history resolution, new-pin currency (retained
  and self pins excepted), ERD/overlay bindings against exact saved revisions,
  then `technicalpolicy.ValidateCandidate` for any revision with intent. System
  interactions check both member endpoints. A refusal writes nothing; a
  written visual is removed if its revision cannot be written.
- **CLI** (`internal/cli/inventory.go`): resolves symbolic commits and computes
  digests, resolves `--delivery` once and binds it to the manifest's
  repository, and binds `--visual` to its content address.
- **Item links** (`internal/cli/inventory_item.go`, `internal/savedview`): a pin
  is admitted by `technicalpolicy.AdmitPin`, current by default or through a
  loaded saved view. Selections are prechecked structurally (so a wrong hop or
  evidence ID is named), their omitted commit defaults to the containing
  evidence's commit, the selected bytes are authored, and
  `Inventory.ResolveSelection` is re-run under the lock. Unchanged pins, views
  and selections are retained on revision. `validate` reports selections that
  stop resolving and format-2 Item content without the marker.
- **Consumer API**: `ResolveSelection` (reason codes `invalid_selection`,
  `path_too_long`, `missing_pin`, `undeclared_hop`, `missing_evidence`,
  `outside_evidence`, `whole_file_selection`; `VerifySelectedBytes` adds
  `selected_digest_mismatch`), `ComposeOverlay`, `EffectiveIntent`,
  `EvidenceByID`, `Pinned`, `savedview.Load`/`Facts`/`GlobalHealth`, and
  `testfixture.WriteInventoryFixture`, which writes a realistic format-2
  inventory through the real writers.

## Tests

`internal/requirements/inventory_format_test.go` (legacy read, opt-in refusal,
succession/baseline, per-kind rules, SVG safety, selection resolution, overlay
composition, schema conformance), `internal/cli/inventory_lifecycle_test.go`
(proposed → implemented → proposed successor → slide pin; drift, pure move,
forged digest, foreign repository, mixed edge intent, all-head parents, retry,
no partial writes; data entities and ERDs), `internal/cli/inventory_item_test.go`
(review and implementation selections, every structural refusal, saved-view
admission and refusals, old pins preserved across definition succession,
transaction selections and replay) and `internal/testfixture/inventory_test.go`.
Every test uses temporary Git repositories; none mutates `app.saga`.

## Known limits and gaps

- **Companion repositories:** a saved view needs the Saga committed inside its
  canonical source repository; otherwise `view_saga_missing` is returned and no
  historical pin can be authored.
- **Retries with symbolic revisions:** `--delivery HEAD` and symbolic evidence
  commits are resolved at run time; a retry after HEAD moves is new content and
  is refused as a conflicting revision. Pass the full OID for exact retries.
- **Evidence ID continuity** across revisions is an author convention, not
  enforced. Legacy evidence has no IDs, so selecting it needs a new format-2
  revision first.
- **Holders** are resource pins; their intent is not required to match the
  entity's. **ERDs** carry no intent; entities do. An overlay removal proposes
  removal but does not retire the entity.
- Saved views re-extract the inventory from Git per admission (bounded to 200k
  files / 256 MiB); no cache or benchmark is claimed.
- `app.saga` has not adopted format 2. Adoption is a coordinated, explicit
  decision; until then its inventory stays format 1 and the new capabilities
  are exercised in fixtures.
- Coverage, query filters, newness, reverse usages and the ERD UI are not in
  this slice.

## Saga documentation and checks

- Implementation deck: four transaction-managed slides in the
  `technical-inventory` deck, section "Records and authoring implementation"
  (`ti-records-impl-format`, `-transition`, `-data-model`, `-selections`,
  ranks 160–190). 27 Item-to-criterion relations prefixed `ti-records-impl-`
  read back active with current pins. The rationales state where display,
  query and coverage belong to the sibling slices.
- Living implementation coverage of `da5101a6..a0cb6cbe`: 6143 of 6164 atoms.
  The 21 uncovered atoms are whole-file addition events; transaction-managed
  Items refuse whole-file evidence, so they stay a visible gap.
- Review `inventory-records-authoring` explains exactly `da5101a6..a0cb6cbe`
  in four slides and covers all 6164 changed atoms (0 stale; 13 intentional
  overlaps where a callout re-cites `formatAdmits`). It records no decisions.
- `validate`: valid, with only the two existing warnings for code-free proposal
  slides. `reconcile --against da5101a6`: 50 evidence references in other
  slides became stale because this slice rewrote code they cite
  (`technical-inventory-identity`, `-history`, `-authoring`, `-verification`,
  `-navigation`, the proposal slides' pinned extension-point links, and
  `authoring-sequence`/`slide-state` in the visual-implementation deck). Those
  slides are shared or owned elsewhere and were reported for reassessment, not
  edited here. The 20 pre-existing stale references are unchanged.
- Raw slide assets were rendered and inspected. Reviewer-surface `visual-qa`
  and browser checks were not run: Playwright is not installed in this
  workspace and installing it needs the network.
