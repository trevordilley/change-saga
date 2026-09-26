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
  ___overview/
  ___personas/
  ___designsystem/
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
