# The mandatory contract at length

`SKILL.md` states these rules tightly; this is their full wording, for a case the
short form leaves in doubt.

A Change Saga is Git-native: the recommended idiom is one Saga per
repository, `change.saga` at its root. It always holds the pull requests'
review decks. When a team grows it, it also documents the application: durable
product domains (features) and their requirements, design, quality, work, and
implementation explanation. A monorepo of several apps also keeps one
`change.saga` at its root and documents each app through its own features. A
pull request compares the Saga and code between commits; its review deck
explains that transition. Author the thing submitted for human review, not the
review verdict.

- Follow the user's requested scope. Do not turn one story, term, diagram, or
  lightweight review into a complete-lifecycle project. If the user requests
  the complete lifecycle, do not silently stop after its easiest part.
- Speak as the change author while authoring. Record approvals, change
  requests, withdrawals, or review comments only when the user explicitly
  asks for a review. Never act or decide on another person's behalf.
- The installed `change-saga` CLI is the source of truth. Start with
  `change-saga --help`; use command `-h`, `change-saga spec --json`, and
  `change-saga query schema <operation>` instead of guessing a command or
  response shape. If a reference disagrees with the CLI, follow the CLI and
  report the mismatch.
- Read real Saga metadata only through `change-saga query`, or read one slide
  compactly with `change-saga diagram describe`, or inspect a named whiteboard
  with `change-saga whiteboard describe --id ID`. Never glob, grep, or open
  metadata files to infer Saga state. Page every result to completion,
  keep one snapshot across a multi-query read, and restart if it changes.
- Make Saga mutations only through public CLI authoring commands. Do not edit,
  invent, rename, or delete metadata files. Narrative and visual source files
  are installed or replaced through the CLI commands that own them.
- Delete what should not exist, and keep the Saga clean; Git keeps the
  history. Every `remove` deletes the record and the links that only pointed
  at it, then lists what else lost coverage. Run it with `--dry-run` first and
  show the user its summary; confirm before a removal that reaches beyond what
  they asked for. Then fix, or report, each listed adjustment. See
  "Deleting records" in [reorganizing existing content](reorganize.md).
- Preserve history and uncertainty. Revisions and lifecycle events are
  immutable; decisions, comments, claims, verification results, and merge
  evidence are append-only. Preserve competing heads and reported conflicts;
  never fabricate a winner. Reconcile only with explicit parents and the
  user's supported intent.
- Preserve provenance. Keep requirement citations, relation rationales and
  pins, review identity, source comparison identity, code-reference digests,
  and lifecycle reasons. Omit an identity you cannot verify rather than
  recording a guess.
- Keep evidence exact. Code references belong on the narrowest semantic Item,
  landmark, test case, or other supported target that explains them. Never
  widen a selector merely to reach complete coverage. A feature implementation
  deck and a pull-request review deck have different coverage roles; do not
  substitute one for the other.
- Treat coverage as an omission check, not proof. Do not invent personas,
  stories, criteria, definitions, designs, test results, or novelty to fill a
  gap. Report genuine uncertainty and preserve conflicts.

The references describe current public contracts only. Do not infer a command
from a planned capability or another branch. Discover new CLI or query support
from the installed binary before using it.
