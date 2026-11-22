# Component and System documentation

Components identify units of logic; Systems explain their directed interactions.
They are reusable technical definitions, separate from vocabulary terms and
feature ownership. Technical decks link these definitions while retaining their
own evidence and explanations.

## Authoring

Use `component|system add --id ID --from FILE --repo CHECKOUT PATH` for a new
record, or `revise` with the same stable ID to replace current content.
Do not supply revision, event or parent IDs. `set-state --id ID --state
active|retired --reason TEXT PATH` edits current state and its reason.

Initialize inventory format 2 with `inventory adopt-format --format 2 PATH`.
A complete definition follows
[technical-definition.schema.json](../schema/v5/technical-definition.schema.json).
Specify proposed or implemented intent. Proposed content names an implemented
baseline digest, or `none` for an initial proposal. Implemented content records
an actual delivery commit and exact scoped evidence; each interaction has its
own stable ID, explanation, endpoint identities and evidence.

The CLI resolves source commits and computes byte digests before publication.
System members use stable Component targets and exact content digests. Changing
a definition does not mutate existing links, evidence, slide content or review
decisions. Reassess stale links explicitly.

Implementation and review Items can carry `documentation: {target, revision}`;
the compatibility field `revision` contains a SHA-256 content digest. The CLI
flag is `--documentation-content-digest`. A documentation link supplies no code
coverage. Explicit selections name evidence IDs and exact byte subsets.

## Reading and storage

`query inventory --saga PATH [--kind component|system] [--target URN]
[--repo CHECKOUT] [--head COMMIT]` returns current definitions, state, code health
and member-pin health with snapshot-bound pagination. `query history --node URN`
reads actual Git commits; view the Saga at a Git commit for earlier content.
The reader's documentation drawer opens exact pinned content and reports stale
or unresolved sources instead of substituting current definitions.

Each package retains its stable `component.json` or `system.json` identity and
one editable `current.json` containing authored content and state. Assets stay
content-addressed. Writers publish atomically under the Saga lock. Git owns
versions, attribution, branches, merges and conflict resolution.

## Compatibility

Readers accept the former layout without writing on reads. A writer or explicit
`normalize PATH` converts unambiguous legacy content and source pins before
removing obsolete revision/event storage. Ambiguous heads preserve every file
and require explicit resolution. Older readers reject the new current-record
layout rather than ignoring it. See [Git-owned history](git-owned-history.md)
and [technical inventory format 2](../SPEC.md#technical-inventory-format-2).
