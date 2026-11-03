# Record deprecation precisely

Read this reference to record, correct, withdraw, or answer questions about a
deprecation.

Use `deprecation add` for an authored annotation on an existing content target,
with an explicit `--scope content|code` and `--reason`. Only code scope's exact
`--ref` values assert code deprecation; explanation links and coverage never do.
Use `query deprecations` globally, by `--target` or by exact `--ref` before answering
whether something is deprecated. Page to completion, preserve conflicting heads
and report stale/missing/provisional evidence. Absence means no recorded deprecation.
Optional replacements never expand scope. Optional stories use normal pinned
`relation add --type explains`; retain historical and retired story context.
Correct through `deprecation revise` with all revision parents, or withdraw through
`deprecation set-state --state withdrawn` with all event parents and a reason.
Never delete content, rewrite immutable history, or invent stories for this task.
Discover details with `deprecation --help` and `query schema deprecations`.
