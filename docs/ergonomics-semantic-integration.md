# Semantic pre-integration and proposal consolidation

Change Saga exposes semantic convergence as an explicit review step. It does
not infer that two proposals mean the same thing and it never merges Git.

## Read-only pre-integration

Run the check against two or more committed refs:

```sh
change-saga preintegrate \
  --repo . \
  --ref main \
  --ref feature/checkout-a \
  --ref feature/checkout-b \
  --json \
  app.saga
```

Every ref is required explicitly and is resolved to a full commit. The command
extracts the Saga from each commit into temporary storage; it does not use the
working-tree copy. The report contains:

- `stable_id_collisions`: the same record kind and stable ID has different
  immutable identity bytes in the compared refs;
- `competing_heads`: a story has different revision or lifecycle heads across
  refs; and
- `overlapping_intent_candidates`: different story IDs reach a token Jaccard
  score of at least `0.60` over their current title and statement.

Each finding carries ref, full commit, and Saga-path provenance. The overlap
score is only a deterministic candidate heuristic. It is not an equivalence
decision and no “winner” is selected. The Go API is
`semanticcheck.Check(context.Context, semanticcheck.Options)`.

## Withdrawing and consolidating proposals

Withdraw a proposal directly instead of routing it through a false acceptance
or defer event:

```sh
change-saga story withdraw \
  --story urn:change-saga:app:story:duplicate \
  --parent urn:change-saga:app:story:duplicate:event:proposed \
  --event withdrawn-as-duplicate \
  --reason "The canonical checkout proposal already covers this intent" \
  app.saga
```

Withdrawal accepts only a uniquely current `proposed` or `deferred` story and
appends a `rejected` event. It refuses accepted intent.

Consolidation is preview-first. Every criterion on the duplicate's unique
current revision must be mapped exactly once to a distinct criterion on the
canonical story's unique current revision:

```sh
change-saga story consolidate \
  --duplicate urn:change-saga:app:story:duplicate \
  --canonical urn:change-saga:app:story:checkout \
  --parent urn:change-saga:app:story:duplicate:event:proposed \
  --event consolidated-into-checkout \
  --map urn:change-saga:app:story:duplicate:criterion:fast=urn:change-saga:app:story:checkout:criterion:responsive \
  --reason "Both proposals express the same confirmed checkout behavior" \
  --json \
  app.saga
```

The preview validates the complete lifecycle and relation set and writes
nothing. Add `--apply` to commit that decision. Applying:

1. appends `rejected` for a proposed/deferred duplicate;
2. appends `retired` for an accepted duplicate only when the canonical story
   is also accepted, preserving accepted intent;
3. creates replacement relations whose exact non-requirement endpoints (for
   example, slide Item URNs) are unchanged and whose story/criterion endpoints
   and revision pins name the canonical story; and
4. marks only the replaced relations superseded, retaining their original
   endpoints and pins as history.

Missing mappings, many-to-one mappings, conflicted heads, terminal canonical
stories, replacement-ID collisions, invalid resulting relations, and any
attempt to move accepted intent into a non-accepted canonical story are
refused before writing. The lifecycle event and all relation replacements are
prepared and committed as one locked file batch; ordinary I/O failure is
rolled back before the command returns.

The Go APIs are `requirements.PreviewConsolidation` and
`requirements.ConsolidateProposal`.

## IDs for newly generated diagram records

For a newly created feature-owned implementation deck or slide, pass
`--feature-qualified-id` while omitting `--id`. The generated ID is
`<feature>--<local>`, normalized and bounded by the existing 128-character
stable-ID grammar. For example, an `architecture` deck under feature
`checkout` becomes `checkout--architecture`; subsequent commands use that
printed exact ID or URN.

Item IDs remain scoped by their containing slide identity. Explicit
IDs, the existing omitted-ID default, onboarding/review IDs, loaded records,
existing URNs, and every persisted historical ID remain unchanged. This is a
generation convention, not a schema or URN-format change. The helper is
`applayout.FeatureQualifiedID(feature, local)`.
