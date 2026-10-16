# Format quick reference

Use this reference only to orient around resource shapes and stable target
identities. The installed CLI remains authoritative: use `change-saga spec
--json` for resources and legal relation endpoints, command `-h` for mutation
syntax, and `change-saga query schema <operation>` for response shape. Do not
infer a storage filename or edit metadata from this summary.

## App shape

There is one Saga format, version 5. The recommended idiom is one Saga per
repository, `change.saga` at its root, which `change-saga init` creates by
default; any `<name>.saga` directory is equally valid. A monorepo keeps one
`change.saga` and documents each app through its own features:

```text
change.saga/
  saga.json
  theme.css           optional token overrides; see theme.md
  ___overview/
  ___personas/
  ___designsystem/
  ___whiteboards/<board-id>/revisions/<revision-id>.json
  ___onboarding/
  ___featureflags/
  ___features/<feature>.feature/
    ___requirements/
    ___design/
    ___quality/
    ___workplan/
    ___slides/<deck>.deck/
  ___reviews/<review>.review/
  ___claims/
  ___verifications/
  ___merges/
```

The layout is descriptive, not an authoring API. Use queries to discover
resources and public commands to change them.

## Windows path portability

Deck records retain a 64-byte filename limit and a 240-byte absolute-path
budget on every platform, including macOS and Linux, to keep Sagas compatible
with Windows. Creation reserves space for a full 64-byte filename; validation
measures each existing filename. The diagnostic reports the directory, filename
allowance, total length, and how many bytes must be removed.

When this guard fails, shorten the checkout/worktree or Saga location by at
least the reported excess. Preserve uncommitted work and managed workspaces;
use a separate checkout at a shorter path when appropriate instead of moving
an active managed workspace behind its manager. Before creating a new record,
a shorter deck or review ID can also reduce its directory path. Keep descriptive
titles in the content. Do not rename existing IDs, hand-edit or rename generated
storage files, disable the guard, or use a symlink to hide the real path length.
Run `change-saga validate --json <saga>` at the shorter location, then retry the
original command. Test fixtures should likewise use a short temporary root.

## Technical design layout

Technical design combines `___overview/technical-design.fragment`, optional
`technical-design-<page>.fragment` supporting essays, and three whiteboards with
IDs `data-model`, `infrastructure`, and `data-flows`.

Named technical-design whiteboards use the shared diagram schema and independent
immutable histories. Their directory placement determines the format; there is
no kind discriminator. Publish and inspect them with the public `whiteboard`
commands described in [diagrams.md](diagrams.md#named-whiteboards).

For technical definitions, use the [inventory authoring guidance](diagrams.md#reusable-components-and-systems).

## Stable targets

Query responses and mutation results return the URNs to reuse. Common forms
are:

```text
urn:change-saga:<saga>:saga
urn:change-saga:<saga>:persona:<persona>
urn:change-saga:<saga>:story:<story>
urn:change-saga:<saga>:story:<story>:criterion:<criterion>
urn:change-saga:<saga>:term:<term>
urn:change-saga:<saga>:deck:<deck>
urn:change-saga:<saga>:slide:<slide>
urn:change-saga:<saga>:slide:<slide>:item:<item>
urn:change-saga:<saga>:chapter:<chapter>
urn:change-saga:<saga>:section:<section>
urn:change-saga:<saga>:fragment:<fragment>
urn:change-saga:<saga>:fragment:<fragment>:landmark:<landmark>
urn:change-saga:<saga>:review:<review>:slide:<slide>:item:<item>
```

No listed story, deck, slide, or Item URN includes its feature. Do not parse a
URN to infer a resource's current feature or lifecycle head; query it.

## Authoritative discovery

- `change-saga --help` lists top-level commands.
- `change-saga <command> -h` and family help list exact flags and subcommands.
- `change-saga spec --json` lists resources, relations, and command shapes.
- `change-saga query schema <operation>` describes response paths and
  pagination without opening a Saga.
- `change-saga status --json <saga>` supplies the ordered work queue but no
  verdict.
- `change-saga validate --json <saga>` checks the authored result.

Use `--json` where a mutation offers a machine-readable result. When the Saga
is separate from its source repository, consult
[integration.md](integration.md) before using `--repo`. For reading,
pagination, and conflict-preserving projections, consult [query.md](query.md).
