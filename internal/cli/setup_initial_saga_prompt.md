# Establish the initial app Saga

Use this workflow only once for a new repository, or during the explicitly
requested documentation overhaul named above. A repository has one app Saga.
It captures the application's durable requirements, design, implementation,
and quality model; future pull-request reviews update that same Saga as the
application changes.

This is a guided product interview followed by evidence gathering. Tell the
user that incomplete answers are welcome: the goal is to record what they
currently understand, expose inconsistencies, and improve the model as the
application evolves. Do not invent the universe merely to make the Saga look
complete.

## Establish repository maturity

Make a bounded, read-only orientation pass before the interview. Inspect the
top-level files, Git history and status, build manifests, runnable entry
points, tests, prototypes, existing documentation, and any Saga reported
above. Determine whether the repository is blank, prototype-only, a scaffold
or partial implementation, an established application, or documentation for
an application implemented elsewhere. Tell the user what you found and ask
them to confirm the larger context.

That context determines the evidence hierarchy:

- In a blank repository, the interview is almost the entire evidence base.
  Stories added solely at the user's request are fully valid requirements.
  Record them as user-confirmed product intent; do not demand code evidence.
- Treat prototypes as explored or intended experience, not proof of production
  behavior. Ask which prototype decisions should carry forward.
- In partial code, distinguish scaffolding and experiments from intended
  foundations and committed behavior.
- In an established application, code and observable behavior prove what is
  implemented. They can expose a mismatch with the user's requirements, but
  they do not silently overrule those requirements.
- If the real application is elsewhere, ask for the authoritative repository,
  artifact, or access boundary and state what cannot yet be evidenced.

Do not manufacture empty implementation or quality sections when there is no
implementation to document.

## Interview the user in short rounds

Start with personas: the kinds of people who receive value from the app and
the outcome each seeks. A persona is the beneficiary in “As a …”, not an
agent, service, database, or internal component. Do not require a persona to
exist as a role or enum in code.

Then ask the user to describe durable features in their own words. A feature
is a lasting product capability, not a pull request, page, source directory,
or implementation layer. For each feature, ask what outcome it enables, which
personas receive that value, where its boundary lies, and whether the user
wants to provide one representative story, several stories, or none yet.

Stories must be persona-focused and deliver observable value. Keep technical
detail out unless it is essential to the requirement. Acceptance criteria are
independent, pass/fail one-liners. Agent-drafted stories and criteria remain
drafts until the user confirms them; user-requested or user-confirmed stories
are authoritative product requirements even when no code exists.

Do not present a long questionnaire all at once. Summarize each short round
and let the answer determine the next useful question.

## Confirm and record the starting model

Before broad investigation, show a compact inventory of the app purpose,
personas and their value, durable features, supplied stories and criteria,
uncertain boundaries, and intentionally empty areas. Separate the user's
assertions from agent interpretations and ask for corrections.

Once confirmed, create or update the one app Saga through the installed
`change-saga` CLI. Never create a second Saga because the first is incomplete.
Use `change-saga --help`, command-specific `-h`, and
`change-saga spec --json` rather than guessing commands or editing metadata
directly.

Validate the initial records. If the investigation will use parallel
workspaces, explain that they need a common baseline, inspect Git status, and
ask before committing only the Saga files produced by this workflow. Never
include unrelated user changes. If the user declines a baseline commit,
continue sequentially or pause rather than branching from ambiguous state.

## Investigate at the maturity the repository supports

Where implementation exists, validate the supplied stories first and trace
each claimed feature end to end through entry points, authorization, domain
logic, state, persistence, integrations, background work, output, and failure
paths. Connect confirmed stories through design and implementation Items to
exact code references. Survey roles, permissions, routes, copy, ownership, and
policies that corroborate personas without requiring a one-to-one code enum.

Read existing READMEs, architecture notes, ADRs, API descriptions, diagrams,
issues, and operational notes as leads and rationale. Treat current code and
observable behavior as implementation truth, while keeping user-confirmed
requirements as product intent. Record mismatches instead of rewriting intent
to match accidental behavior.

After tracing the supplied model, work backward from unexplained code to
propose missing product stories and design. Ask the user before recording
inferred stories as authoritative. Do not turn every endpoint, function, or
table into a feature.

For a greenfield or prototype-only repository, replace code-tracing work with
bounded product-boundary, prototype, and intended-design investigation. The
absence of code is not a defect in the requirements and should not produce
pretend implementation evidence.

## Review before quality expansion

Build the first coherent product, design, and implementation model that the
available evidence supports, validate it, open the Saga, and present it to the
user. Summarize confirmed understanding, corrections, gaps, and questions
that still require product judgment. Stop for user review before expanding
quality and test coverage.

After that review, inspect existing test strategy and coverage. Model useful
positive, negative, and edge test cases; relate them to acceptance criteria;
and attach exact test evidence when it exists. Distinguish behavior that is
implemented from behavior that is actually verified.

The setup is complete when the repository has one valid app Saga, confirmed
personas and durable features are recorded, user-confirmed stories are clearly
requirements, implementation claims have exact code evidence where code
exists, intended design is distinguished from implemented behavior where it
does not, and the remaining gaps form an honest queue. Future work updates
this Saga through ordinary Change Saga authoring and pull-request reviews;
do not run initial setup again for each change.
