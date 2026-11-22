# Plan CLI compatibility and diagnosis

Phased Plans do not inherently require an unreleased CLI. v0.7.0 authors
phased Plans; v0.8.0 adds `query plans`, `query plan`, and `query task`.
That does **not** make either release a reader for every Saga in this checkout.
The Git-owned-history refactor introduces current-record component layouts
while the root manifest remains version 5. Released readers reject those
layouts. See [the migration contract](git-owned-history.md).

## Capability matrix

| Build/data family | Plan layout | Plan authoring | Plan queries | Native checkpoint fields | Whole-Saga current records |
| --- | --- | --- | --- | --- | --- |
| Observed `0.5.1-local.removal` binary | Legacy waves/work items | Old workflow; not phased Plan authoring | No | No verified support | No |
| v0.7.0 (`0b399de788e8`) | Phases | Yes | No | Rejected | Rejected |
| v0.8.0 (`cd8a55c74ffe`) | Phases | Yes | Yes | Rejected | Rejected |
| Git-owned-history base `6536dafb9affaa8318a22adfcfddcf48aea48de7` and diagnostic build `ce8d796d` | Phases | Yes | Yes | Rejected | Yes; legacy reads retained |
| This branch after checkpoint integration | Phases with optional checkpoints | Yes | Yes | Supported, with held readiness and explicit passes | Yes; legacy reads retained |
| Unmerged `plan-live-activity` branch | Phases with optional checkpoints | Branch-specific | Verify exact commit | Branch-specific support | Not established; do not assume compatibility with normalized Sagas |

Native fields include `phases[].checkpoint` and `checkpoint_passer`. Even an
empty checkpoint object carries semantics. Strict JSON decoding is intentional:
never strip unknown fields, replace checkpoints with ordinary phases, or retry
through a reader that silently drops data. Wave/work-item data needs an explicit,
reviewed migration; changing field names is not a supported conversion.
The current reader supports native checkpoints, rejects unknown fields inside
them, and adds a specific recovery hint for incompatible legacy waves.

## Edit scope without replacing identities

Use `plan set` to update an open Plan's title and deck objective. A title edit
updates both the Plan record and its deck; omitted fields, Tasks, phases,
checkpoint expectations and passes stay unchanged. The objective must contain
1 to 240 characters. Delivered Plans remain immutable.

```sh
change-saga plan set --plan PLAN_ID --title "Revised scope" \
  --objective "Explain the approved operational access boundary." --dry-run --json PATH
change-saga plan set --plan PLAN_ID --title "Revised scope" \
  --objective "Explain the approved operational access boundary." PATH
```

`--checkpoint-passer person|agent` remains a separate optional flag. Changing
display text does not change that authority. `--json` reports the changed fields
and paths; `--dry-run` writes nothing. A failed edit restores any files written
by the operation. Git retains committed earlier content.

Story wording already has an edit command; it does not require new revision IDs:

```sh
change-saga story revise --story urn:change-saga:SAGA:story:STORY_ID \
  --title "Revised story title" --statement "As a user, I can ..." PATH
change-saga story revise --story urn:change-saga:SAGA:story:STORY_ID \
  --overview-source revised-story.md PATH
```

Omitted story fields retain their current values, including criteria, personas
and citations. A structured `--from` supplies complete content instead. Editing
an already current story no longer normalizes unrelated legacy packages, so an
independent legacy whiteboard fork does not block that edit or get resolved as
a side effect. The requirements domain still loads strictly and validates the
authored story and its references. Unknown fields and invalid requirements
remain errors. A legacy target still requires pin-preserving normalization;
competing legacy values or missing pin sources continue to block it explicitly.
This is not a bypass for unresolved target content or malformed JSON.

## Diagnose the actual invocation

Ask the person seeing the failure to run these in the **same shell/environment**
as the failing command (including an IDE or agent's environment if applicable):

```sh
type -a change-saga
change-saga version
change-saga query schema plans
```

Capture the exit codes, exact failing command, exact error, and whether the Saga
has been normalized or contains native checkpoints. A missing query operation
does not prove authoring is absent. An unknown field is a data/reader mismatch,
not a reason to delete content. On PowerShell use
`Get-Command change-saga -All` for executable resolution.

This follow-up adds:

```sh
/absolute/path/to/change-saga doctor --json
/absolute/path/to/change-saga doctor --json /path/to/change.saga
```

`doctor` reports the executing file and symlink resolution, its SHA256, version,
Go toolchain, available VCS revision/dirty state, and separate Plan and Saga
capabilities. VCS fields can be unavailable; a version label by itself is not
proof of source identity. A checksum identifies file bytes, not their provenance.
The JSON schema is `change-saga.doctor/v1`. With a Saga path, it runs the same
whole-Saga validation as `validate`, including requirements, prototypes, quality,
inventory and whiteboards. Invalid or unreadable data exits 1 with its findings
in the report; without a path it reports capabilities only. Warnings do not fail
validation. It does not normalize, repin, repair, contact services, execute other
PATH candidates, or establish semantic correctness of the documentation.

Run `query schema plan` and `query schema task` too when those operations matter.
Then validate the **whole** Saga with the selected absolute binary and exercise
the needed queries. A schema probe alone never certifies a particular Saga.
Use that same binary for authoring and serving, including any long-lived reader
process. Restart a reader after deliberately changing builds.

## Pin one build for the data

For a legacy-layout Saga without checkpoints, the v0.8.0 release is a candidate
when queries are needed. Use its pinned installer/archive and checksum procedure
in [the release](https://github.com/trevordilley/change-saga/releases/tag/v0.8.0),
then validate that actual Saga before adopting it. Do not use `latest` or install
over a working binary as a diagnostic step.

For normalized Sagas without checkpoints, the known compatible source baseline
is `6536dafb9affaa8318a22adfcfddcf48aea48de7`; this branch adds diagnostics and
tests on top of it. Pin an exact reviewed commit of this follow-up when its
diagnostics are needed. For checkpoint-enabled Sagas, pin the later checkpoint
integration on this branch: it combines the native fields and their behavior
with current records. Build in a separate checkout/output directory so neither
PATH nor the working Saga is changed. For example, from a clean checkout of the
reviewed commit, with this repository's pinned Go toolchain installed:

```sh
BUILD_COMMIT=$(git rev-parse HEAD)
test -z "$(git status --porcelain)" || exit 1
BUILD_DIR=$(mktemp -d)
GOTOOLCHAIN=local go build -trimpath \
  -ldflags "-X github.com/twentyideas/changesaga/internal/cli.Version=0.8.0-checkpoints-dev -X github.com/twentyideas/changesaga/internal/cli.Commit=$BUILD_COMMIT" \
  -o "$BUILD_DIR/change-saga" ./cmd/change-saga
"$BUILD_DIR/change-saga" version
"$BUILD_DIR/change-saga" doctor --json /path/to/change.saga
"$BUILD_DIR/change-saga" query plans --saga /path/to/change.saga
```

Record the full reviewed commit, toolchain, binary path, SHA256 and validation
result together in the handoff. The `checkpoints-dev` label is a local build label,
not a release. Keep the selected file at a stable absolute path for ongoing use.
The baseline predates `doctor`; use `version`, `validate --json` and the schema
probes there. Neither that baseline nor `ce8d796d` supports checkpoints. This
branch now integrates them; the old diagnostic binary must be rebuilt from the
later commit before it can read such a Plan. Do not downgrade normalized data
or delete gates. See [checkpoint integration and Git migration](checkpoint-integration.md).

Agree on the build **before writing**. Writers can normalize encountered legacy
packages; upgrade all readers first. Git owns content history; preserve stable
identities, actual review facts, exact commit/digest evidence and stale pins.
No diagnostic should silently refresh evidence. To inspect a pre-normalization
layout, use its Git commit in a separate checkout.

## Audit evidence and limits (2026-11-20)

- The authoritative [v0.8.0 release](https://github.com/trevordilley/change-saga/releases/tag/v0.8.0)
  was published 2026-11-03 at 03:43:18 UTC. Its tag resolves to
  `cd8a55c74ffe109f3452071d6eb317081c98e53c`. The tagged changelog and
  `internal/cli/query.go` add the three Plan queries; v0.7.0 already defines
  `PlanPhase`, `phases`, `plan add-phase` and `task --phase`.
- Downloaded the Darwin ARM64 release archive and verified it against its
  release `SHA256SUMS`:
  `1c213962e845843534ed5310d8efb9d75861b4caa0b7c131612ad12a3658f4b8`.
  It reports `0.8.0 (cd8a55c74ffe)`, answers `query schema plans`, and exits 1
  validating this normalized repository Saga, rejecting current-slide `$schema`
  and unfamiliar current records. Release provenance was not independently
  attestation-verified in this audit.
- In an isolated legacy-layout Saga, the installed v0.7.0 successfully created
  a Plan and added a phase; the checksummed v0.8.0 release then read that phase
  through `query plan`. Adding `phases[].checkpoint: {}` made both v0.8.0 and
  `ce8d796d` reject the query, retaining the checkpoint bytes. That diagnostic
  build's whole-Saga `doctor` also validated the repository's normalized
  Saga successfully (235 warnings, no errors); it did not refresh any pins.
- On this machine, `type -a` resolves `~/.local/bin/change-saga` first:
  `0.7.0 (0b399de788e8)`, which rejects `query schema plans` as an unknown
  schema operation. `/opt/homebrew/bin/change-saga` reports
  `0.5.1-local.removal (4ba5fcad71d4-dirty)`; its schema catalog contains
  `waves`, `work-items`, `work-events`, and `work-conflicts`, not Plan queries.
  These observations concern this environment, **not Greyson's machine**.
- Inspected `plan-live-activity` at
  `ec1aff418dcc241056f544a87bbaddb1a9948b93` for checkpoint types. It is not
  an ancestor of this branch. The native fields are absent from v0.7.0,
  v0.8.0, and the Git-owned-history baseline. The initial diagnostic build did
  not import checkpoints; the later integration was explicitly authorized.

Greyson's resolved executable, version/build, exact failing command and exact
error are still unknown. Request those together with the three diagnostic
outputs above before diagnosing his case or selecting his pinned build. Do not
attribute this machine's observations to him or contact him without authorization.
