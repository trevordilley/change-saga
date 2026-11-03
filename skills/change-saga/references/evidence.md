# Exact evidence and claims

Read this reference when attaching code to an Item or landmark, repairing stale
references, or recording a claim and its verification.

## Evidence discipline

Code references are pinned to a commit and digest of the exact bytes. Pure
line movement may remap them; changed bytes make them stale. Repair or remove
stale evidence through its public command while retaining its history.
`references --stale`, `reconcile`, `review list`, and `review refresh-coverage`
show each stale reference's proposed range (its start and end mapped through
the diff, widened over lines inserted inside) with the diff inside it and the
nearest hunk on each side. After reading it, `repin --accept-proposed --record
FILE [--reference N]` (or `--target URN`, `--review ID`) keeps the note and
owner and pins the proposal, also on `apply-slide` slides. A reference with
nothing proposed (deleted file, rewritten range, only braces or blank lines
left) is refused: re-cover it by hand.

- Attach code to the narrowest Item or landmark that actually explains it.
- Each reference note says both what changed and why this target owns it.
- Keep one file and coherent reason per record; split different reasons.
- Preserve deletions, migrations, generated files, snapshots, lockfiles, and
  vendored changes as explicit evidence rather than hiding them in broad
  ranges.
- Use changed-lines coverage only when every changed line in that file belongs
  to the same focused target. `cover --batch -` may plan several focused
  records atomically, and `--dry-run` previews supported mutations. Never widen
  a selector merely because a batch makes broad coverage convenient.
- Treat overlap as intentional only for distinct reviewer journeys. Repair an
  existing record with `replace-coverage`, not a duplicate.
- Use `query mappings --sort scrutiny` after coverage. Never hand-edit a
  record to make stale evidence pass.

A landmark target may be passed back as its returned URN or, where the command
accepts it, as `<fragment>#<landmark-id>`. Use returned targets rather than
deriving storage paths.

An Item may relate to a story or criterion with a pinned, self-scoped relation.
Prefer a criterion when the visual explains one obligation; use the story only
when it genuinely applies to every criterion. A slide's summary is derived
from Item relations; deck or slide links do not apply automatically to every Item.

## Claims and verification

Use `add-claim` for a falsifiable assertion such as an invariant,
compatibility promise, measured performance result, security property, or
test outcome. Claim evidence is separate from coverage. Append a result with
`verify-claim` only after performing its named method; record a reproducible
command where possible. Use `unverified` when it was not checked. Claims and
results are append-only.
