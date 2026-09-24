# Overview and project vocabulary

Read this reference for the Saga title, pitch, description, or terms. Read
[query.md](query.md) first for an existing Saga and use public CLI commands for
every mutation.

## Overview

The overview consists of the Saga title, an elevator pitch, a short project
description, and the vocabulary. Keep the pitch concise and the description
oriented around the product's purpose and boundaries. Missing overview areas
are growth suggestions, not automatic blockers.

Recover wording from the user and authoritative project material. Do not
invent positioning, audiences, definitions, or promises merely to remove an
overview gap.

## Terms

A term is a domain word the team uses routinely that a newcomer cannot decode
without product or code context. It may carry:

- its stable ID, display name, definition, and aliases;
- the stories or other records that use it; and
- an exact pinned code reference to a defining enum, constant, type, or other
  focused declaration.

Term code references are watched for staleness but do not count toward
changed-line implementation coverage. From a term, query its current revision,
lifecycle, links, and code health. From a story or code location, use the
public term and traceability queries to find relevant vocabulary.

Definition maturity and implementation-evidence availability are independent:

- definition maturity is `unknown`, `proposed`, or `accepted`;
- implementation evidence is `unknown`, `absent`, `partial`, or `present`;
- `term add` and `term revise` accept `--definition-maturity` and
  `--implementation-evidence`; both default to `unknown`; and
- lifecycle state (`active` or `retired`) remains a separate axis.

Evidence availability never proves that the implementation is correct or
complete. `absent` is an observed gap; `unknown` means not verified or not
assessed. Current and stale code-link health also remain separate from the
availability value.

The terms query always projects both semantic axes. Legacy revisions that omit
them project as `unknown`. When revision heads conflict, preserve
`revision_heads`, omit `current_revision`, and treat both axes as `unknown`
rather than fabricating a merged state or choosing a winner.

Use `query term-references --term ID|URN` for direct incoming and outgoing
typed uses with exact owners and selectors. Its completeness metadata excludes
free-form prose, embedded SVG text, history, and transitive expansion; never
describe those classes as searched. Plain `query terms` preserves the legacy
complete collection. Add `--limit` to opt into bounded enumeration and follow
every cursor at one snapshot.

A generated stale-term revision action carries the current values of both
axes forward while asking for the repaired code reference. Preserve those
values unless the task also has evidence for a semantic change; repairing a
stale selector must not reset either axis to `unknown`.

A rename, changed definition, or changed semantic axis creates a new immutable
revision with explicit parentage; update its exact defining reference when
needed rather than rewriting the old record. There is no guarded name-only
rename command yet, so do not invent one or edit metadata directly. Preserve
competing revision or lifecycle heads until intent is known. Do not infer a
winner from timestamps, filenames, or branch order.

Status may suggest vocabulary for added enum values or typed constants. Treat
the suggestion as a question: confirm that the identifier represents a word
the team actually uses and confirm its meaning. Never turn a mechanically
derived name into a fabricated domain definition.

## Focused workflow

1. Query the current overview or term set and page it completely at one
   snapshot.
2. Confirm wording and provenance from the user or authoritative source.
3. Use the focused overview or term command, preserving stable identity,
   explicit parents, aliases, record links, and exact defining code.
4. Re-query current heads and stale references, then validate.

Overview and term work does not obligate the user to add unrelated stories,
designs, tests, or decks.
