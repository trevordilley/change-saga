# App Saga capability audit

Status: research for a canonical living `app.saga`, based on repository commit
`8059a17`. This audit does not change the Saga or product code.

## Executive finding

The current Saga captures the product thesis well, but it is not yet a canonical
inventory of the product that exists. Its seven features and eighteen stories
mostly restate the decisions in `docs/repository-saga.md`: the format, traceability
model, compare model, review model, and incremental-adoption policy. The
repository exposes a much wider product surface: complete living-record
authoring, prototypes, design and quality evidence, claims and independent
verification, structured AI queries, feature flags, delivery planning, review
decisions, managed local serving, and lifecycle maintenance.

The central modeling problem is that the current stories often describe a
command, file model, UI region, or architectural invariant instead of a human's
problem and the value of solving it. Consequently, the Saga can report that
mapped code reaches stories and personas while still omitting whole
user-visible workflows.

The canonical Saga should:

1. Keep a small set of durable, outcome-oriented product domains.
2. Make stories about human value and observable behavior.
3. Move architectural rules into design invariants and exact CLI/file details
   into implementation notes.
4. Use acceptance criteria for observable contracts such as exit behavior,
   scope, currency, and safety boundaries.
5. Add the implemented but undocumented workflows before expanding design and
   quality evidence around the current eighteen stories.

## Scope and evidence

This audit reconciled four sources:

- the public product explanation and workflow in `README.md`;
- the public command grammar in `internal/grammar/commands.go` and
  `internal/grammar/commands_content.go`;
- the user-facing server and domain packages under `internal/server`,
  `internal/prototypes`, `internal/quality`, `internal/workplan`, and
  `internal/livingapp`;
- the current V5 records under `app.saga`, read through the repository's current
  `change-saga query`, `status`, and `validate` implementations.

The inventory below treats a capability as user-visible when a person or their
automation can invoke it through the supported CLI, structured query API, or
review application. Package internals are evidence, not product requirements.
The existence of a command does not by itself claim that its experience is
complete or polished.

## Product boundary

Change Saga helps a team preserve and review the intent of an application as
its code changes. It has three principal outcomes:

- an author can explain a change and grow durable product knowledge without an
  up-front documentation project;
- a reviewer can understand what changed, why it changed, and whether the
  explanation accounts for the code, while retaining the human verdict;
- a future reader can understand the current application and follow its
  requirements, design, verification, and code evidence in either direction.

Planning, CI, and AI interfaces support those outcomes. They are not separate
personas and should not displace the human value in stories.

## Primary human personas and value

| Persona | Current status | Primary value | Audit finding |
| --- | --- | --- | --- |
| Change author | Active | Prepare a coherent, reviewable explanation of a change; discover the next useful documentation step; keep existing knowledge current. | Correct primary persona, but currently overloaded with maintainer, release integrator, CI consumer, and delivery coordinator responsibilities. Stories should name the concrete job rather than rely on this broad label alone. |
| Reviewer | Active | Understand intent before reading isolated diffs, find the evidence for claims, see unexplained code, and record a human decision at the right scope. | Correct primary persona. Several assigned stories describe storage or graph mechanics rather than reviewer value. |
| Newcomer | Active | Learn what the application is for, its language, its domains, and how behavior maps to code without reconstructing history. | Correct primary persona. The current assignment to low-level code-reference mechanics is indirect; that mechanism should support a clearer comprehension or trust story. |
| Coding agent | Retired | No independent human value. It operates the product for one of the people above. | Correctly retired. The CLI/query/skill surface is a channel and usability constraint, not a persona. |

Two roles are visible in implemented workflows but not cleanly modeled. A
repository maintainer or process owner configures CI policy, companion-repo
sync, feature flags, and release-time re-pinning. A delivery lead coordinates
waves, work items, dependencies, contracts, assignments, progress, and merges.
These should become personas only if interviews confirm distinct goals and
decision rights; otherwise model them as contextual roles of the change author.

## User-visible capability inventory

| Capability | Human value and principal workflow | Repository evidence | Current Saga coverage |
| --- | --- | --- | --- |
| Bootstrap and incremental adoption | An author initializes one app Saga, explains a first change, checks it, and can install an agent skill without first defining the whole product. | `init`, `install-skill`, automatic first-feature behavior, README quick start. | Partly covered by `first-change` and `growth-not-debt`; installation, safe bootstrap, and recovery are not stories. |
| Application overview and vocabulary | A newcomer learns the pitch, description, people, terminology, onboarding, and feature flags; an author evolves those records over time. | `overview`, `term`, `persona`, `flag`, onboarding deck commands; app navigation and directory pages. | `overview-and-terms` covers only part. Persona lifecycle, onboarding, feature flags, aliases, and term lifecycle have no value stories. |
| Durable product requirements | An author creates and revises features, stories, criteria, citations, and lifecycle state while preserving identity and history. | `feature`, `story`, `criterion`, `citation`, relation commands; `internal/requirements`. | The Saga documents the model (`durable-features`, `traceability-chain`) but not the human workflows of discovering, revising, reconciling, moving, deferring, or retiring requirements. |
| Prototyping | A team can preserve an interactive or external experience, revise it immutably, and annotate elements back to story or criterion revisions. | `prototype add-html`, `add-external`, `revise`, `annotate`; `internal/prototypes`; prototype navigation. | Entirely absent: no story, prototype record, or feature-level explanation. |
| Design authoring | Authors capture UX, UI, and technical design as addressable chapters, sections, fragments, landmarks, and pinned relations. | `design` and narrative-content commands; renderer support for Markdown, SVG, images, HTML, landmarks, and deep links. | Only three design chapters exist, addressing four stories. The design-authoring workflow itself is undocumented. |
| Visual implementation explanation | An author builds implementation and onboarding decks from slides and semantic Items, with code-bearing visual nodes and callouts. | `add-deck`, `add-slide`, `add-item`, content/edit/remove commands; deck renderer. | Mentioned in feature descriptions and review stories, but authoring, semantic selection, accessibility, and coherent visual explanation have no persona-value story. |
| Exact code evidence and currency | Authors attach exact line/file events to the smallest explanation; readers see current, remapped, or stale evidence; maintainers repair, re-pin, or sync it. | `cover`, `replace-coverage`, `remove-coverage`, `references`, `repin`, `sync`; `internal/coderef`, `coderesolve`, `coverage`. | `code-references`, `changed-lines-accounted`, and `companion-repositories` cover the mechanism extensively, but reader trust, repair workflow, and failure recovery are weakly stated. |
| Claims and independent verification | An author records falsifiable claims against code and a reviewer records append-only verification without rewriting the claim. | `add-claim`, `verify-claim`, query `claims` and `verifications`. | Entirely absent, and the current Saga contains no claims. |
| Quality policy and evidence | A team defines test cases, required test kinds, automation mode, implementation-under-test evidence, run evidence, and lifecycle/currency. | `quality test-case`, `policy`, `evidence`, and `run`; `internal/quality`; quality pages and traceability projections. | The format is mentioned, but there is no quality workflow story. The Saga has one test case, covering two of sixty-nine criteria. |
| Coverage guidance and policy questions | An author sees separate implementation, story, persona, design, quality, and health findings; a team asks a bounded pass/fail question without the tool issuing a product verdict. | `status`, `check`, ordered next actions, stable JSON; `internal/livingapp`, `coverage`, `nextaction`. | Covered by four `agent-loop` stories, but mostly as command/output contracts. Error recovery, prioritization quality, and CI consumption are not expressed as user value. |
| Structured AI and CI access | An agent or automation reads bounded, deterministic, snapshot-bound views instead of scraping files, with pagination and explicit schemas. | `query` operations for overview, hierarchy, mappings, claims, requirements, relations, work, traceability, and readiness; `spec --json`; `docs/ai-facing-interface.md`. | Named only in the `agent-loop` feature description. No story or acceptance criteria capture safe, bounded, deterministic automation. |
| Observe current documentation | A newcomer or maintainer browses the application at a commit, searches/filter directories, opens exact records, code, history, and stale warnings. | `open`, `serve`; app navigation, directories, terms, requirements, design, quality, source catalog, history. | `open-observe-or-compare`, `sidebar`, and `two-sides` cover selected UI structure, not the full learning and maintenance journey. |
| Compare a change | A reviewer sees changed, affected, and code layers; replacement pairing; commit reasons attached to affected records; and unexplained lines. | comparison ranges and layers in `internal/changeview` and `internal/server`; compare-aware status/query. | Covered by `observe-or-compare` and `why-things-changed`, but those combine product behavior with Git architecture and duplicate `open-observe-or-compare`. |
| Pull-request review | An author creates a review deck; a reviewer comments or decides per slide; both see currency and uncovered lines; landed reviews remain history. | `review create/list/approve/request-changes/withdraw/comment`, review application and store. | Covered by two review stories, but no review record exists in the current Saga and quality evidence is absent. Reviewer resumption, discussion flow, and frozen-history behavior deserve explicit criteria. |
| Delivery planning | A delivery lead defines waves, work items, dependencies, versioned contracts, workspace assignments, progress, and merge evidence; readiness remains distinct from delivery proof. | `plan` command family; `internal/workplan`, `internal/readiness`, work/readiness queries. | Entirely absent from the feature taxonomy and stories. This is the largest implemented capability missing from the Saga. |
| Validation and safe local operation | A user can validate/fix structural issues and view untrusted authored visuals without exposing a remote server or network-capable frame. | `validate`; loopback-only `serve/open`; sandbox/CSP, path containment, origin checks, bounded server settings in `internal/server` and `SECURITY.md`. | Structural validation is implicit. Safety, trustworthy errors, offline use, accessibility, and performance are absent as product requirements or cross-cutting constraints. |

## Major end-to-end workflows

### 1. Adopt on a first pull request

Initialize the Saga, let the first content command create or select a feature,
author a small implementation deck, attach all changed lines to explanatory
Items, run `status` or `check`, and open the local reviewer. The current Saga
captures the minimality principle but not install/bootstrap failures, safe
defaults, or how a person knows they are done.

### 2. Grow durable product knowledge

Turn discoveries from a change into personas, stories, criteria, terms,
prototypes, design, tests, and implementation explanation; connect adjacent
records with pinned relations; revise or retire records as understanding
changes. The command surface implements this workflow, but the Saga currently
documents its data model more than the person's iterative job.

### 3. Keep evidence current after code or requirements move

Observe or compare, inspect current/remapped/stale references and relation
pins, repair mappings, re-run status, then re-pin to the landed commit or move
a companion Saga's sync cursor. The current stories explain the reference
algorithm but not the maintainer's diagnosis-and-recovery journey.

### 4. Review a pull request

Open against the base, read the review deck and related current documentation,
inspect linked diffs and uncovered lines, comment or decide per slide, revisit
out-of-date decisions, and retain the landed review in history. The Saga states
the model but has no review artifact or quality evidence proving the workflow.

### 5. Automate inspection and policy

Use deterministic JSON queries to inspect the graph and weak mappings, use
`status` for findings, and use `check --covers` for only the policy the team has
chosen. This implemented workflow has no human story for the maintainer who
needs stable, bounded automation and actionable failures.

### 6. Coordinate large delivery

Define waves and independently mergeable work, record only real dependencies,
version provider/consumer contracts, assign workspaces, append progress and
merge evidence, and query readiness without treating progress as proof. No
current feature or story acknowledges this workflow.

## Current Saga coverage and health

The current Saga is valid and contains seven features, three active human
personas, eighteen accepted stories, sixty-nine acceptance criteria, thirty-five
terms, three design chapters, one implementation deck, one onboarding deck,
one test case, and seventeen relations. It contains no prototypes, feature
flags, claims, work items, or reviews.

`status --json app.saga` at the audited commit reports:

| Axis | Covered | Total | Interpretation |
| --- | ---: | ---: | --- |
| Stories | 10 | 10 | All code targets that are already mapped reach a story; this is not coverage of the repository's capabilities or codebase. |
| Personas | 10 | 10 | The same ten mapped targets reach a persona; it does not validate whether stories express real persona value. |
| Design | 4 | 18 | Four stories have an addressing design; fourteen do not. |
| Quality | 2 | 69 | One test case verifies two criteria; sixty-seven criteria have no recorded test case. |
| Health | 80 | 81 | One evidence reference for the documentation sidebar is stale after `internal/server/appnav.go` changed. |

This explains why graph completeness alone is insufficient for a canonical
Saga. The graph can be internally consistent over a very small mapped surface
while public capabilities remain absent.

## Misclassified stories and recommended disposition

The table gives one primary destination for every current story. “Rewritten
story” means retain the durable intent but restate it as a persona problem,
observable outcome, and value. Exact commands and storage choices can still be
documented beneath that story in the indicated layer.

| Current story | What it is now | Recommended canonical disposition |
| --- | --- | --- |
| `changed-lines-accounted` | Coverage invariant stated as system behavior. | **Acceptance criterion** of a reviewer-confidence story: every changed line is either explained by the review/implementation narrative or visibly unexplained. Keep “computed, not stored” as a design invariant. |
| `check-covers` | Command specification, including exact syntax and exit behavior. | **Acceptance criterion** of a maintainer policy-automation story. Put the literal command and flags in an implementation note/CLI contract. |
| `code-references` | Code-evidence architecture decision and edge-case algorithm. | **Design invariant** under evidence currency. Add a separate reader story about trusting a link after code moves and an author story about repairing stale evidence. |
| `companion-repositories` | Real deployment capability, mixed with cursor and Git mechanics. | **Rewritten story** for a maintainer who must document code without modifying its repository. Make checkout verification, portability, compare semantics, and sync behavior criteria; keep cursor implementation in design. |
| `coverage-report` | Product principle mixed with the `status` response contract. | **Rewritten story** for an author who needs prioritized, non-judgmental findings. Counts/lists, stable JSON, exit-zero-on-findings, and next-action ordering are criteria. |
| `documentation-not-approval` | Information/governance architecture rule. | **Design invariant** separating durable documentation from review conversation and decisions. Test the absence of documentation approval controls through review workflow criteria. |
| `durable-features` | Information architecture and identity rules. | **Design invariant** for domain organization and stable identity. User value belongs in stories about finding and evolving product knowledge. |
| `first-change` | Persona-value workflow, though phrased in passive policy language. | **Rewritten story**: a first-time change author can produce a useful review package without completing the whole documentation model. Preserve implementation-only completion as criteria. |
| `growth-not-debt` | Product principle and prioritization policy. | **Design invariant** for progressive adoption: absence is an opportunity, breakage of adopted knowledge is a health finding, and growth never silently becomes a merge verdict. |
| `observe-or-compare` | Useful product behavior with merge-base and layer implementation detail. | **Rewritten story** for a reader who can choose current understanding or change understanding without maintaining two documents. Keep merge-base and non-persisted comparison as design invariants. |
| `one-app-saga` | Top-level architecture decision and storage rule. | **Design invariant**. Do not preserve “one repository” as a user story; explain its benefit in the overview and test uniqueness/identity structurally. |
| `open-observe-or-compare` | Duplicate command and UI specification for `observe-or-compare`. | **Acceptance criterion** merged into the rewritten observe/compare story. Exact `open` invocations are implementation notes. |
| `overview-and-terms` | Product capability mixed with term-record schema. | **Rewritten story** for a newcomer who can learn the app's purpose and language. Record fields, bidirectional indexes, and rename staleness belong in design and criteria. |
| `review-is-pr-deck` | Valuable review workflow mixed with “review equals PR,” deck storage, and state architecture. | **Rewritten story** for a reviewer who needs a coherent explanation, scoped decisions, currency, and visible omissions. One-review-per-PR and deck/event representation are design invariants. |
| `sidebar` | UI component specification. | **Acceptance criterion** of an information-finding/navigation story. The literal section order is design detail unless user research makes it a stable contract. |
| `traceability-chain` | Graph architecture and coverage policy. | **Design invariant** defining allowed adjacent links, inference, and currency. Add value stories for impact analysis and evidence traversal in either direction. |
| `two-sides` | Reviewer information-architecture decision. | **Acceptance criterion** of the review/navigation story: current documentation and change review are distinguishable and cross-linked. Header labels and placement are design details. |
| `why-things-changed` | Reader value mixed with the architectural choice to recover rationale from Git. | **Rewritten story** for a future reader who can discover why current behavior replaced the previous behavior. Git attribution, pairing inference, and squash metadata are design invariants. |

The clearest command specs are `check-covers` and
`open-observe-or-compare`. The clearest architecture decisions are
`one-app-saga`, `durable-features`, `traceability-chain`, `code-references`, and
`documentation-not-approval`. `sidebar` and `two-sides` are interface design,
not independent persona-value stories.

## Product requirements versus design and implementation

### Product requirements belong in stories and criteria

Product requirements should say what a person can accomplish and what they
can observe. Examples for the canonical Saga include:

- a first-time author can create a useful explanation without documenting the
  whole application;
- a reviewer can find every unexplained changed line and retain the final
  judgment;
- a newcomer can learn the app's purpose, vocabulary, domains, and supporting
  code;
- a maintainer can detect and repair stale knowledge after code or requirements
  move;
- a team can record test policy and see whether current evidence satisfies it;
- automation can query bounded, deterministic views without parsing private
  storage;
- a delivery lead can see actual dependency blockers without confusing
  reported progress with delivery evidence.

Exit codes, visible states, scope rules, offline behavior, accessibility, and
failure recovery are appropriate acceptance criteria when users or automation
depend on them.

### Technical design should hold the invariants

The following are important, but they are not user stories:

- one Saga identity per application and durable product-domain features;
- app-unique stable URNs independent of feature membership;
- immutable identities, append-only full revisions and lifecycle events, and
  explicit reconciliation of concurrent heads;
- only adjacent traceability links are authored; longer paths are derived;
- relation and evidence pins make currency and staleness computable;
- code evidence names repository, commit, path/range or file event, and digest;
- compare is a merge-base-derived view and is never stored in `saga.json`;
- documentation has no decisions or comments; review records do;
- status reports independent axes and never reduces them to one score;
- progress and approval are not implementation or quality evidence;
- the local app refuses remote bind, sandboxes active content, and avoids
  runtime network dependencies.

These should live in design chapters, the normative specification, or explicit
cross-cutting constraints, with tests and code evidence.

### Implementation notes should hold replaceable mechanics

Examples include exact command spellings and flags, directory names such as
`___requirements`, JSON schema versions, Go package boundaries, renderer route
names, generated evidence filenames, and the current content-digest/remapping
algorithm. They matter for implementers and compatibility, but should not set
the product taxonomy or masquerade as persona value.

## Proposed minimal feature/domain taxonomy

Five durable product domains cover the public capability surface without
copying the package tree or making every command a feature:

| Proposed feature | Outcome and contents | Current material to move or absorb |
| --- | --- | --- |
| **Application knowledge** | People can capture and understand the app: overview, personas, terms, onboarding, feature flags, durable features, stories, criteria, citations, prototypes, design, quality intent, and lifecycle. | Most of `format` and `requirements`; currently missing prototype, flag, and lifecycle stories. |
| **Evidence and currency** | People can connect knowledge to code and tests, inspect confidence, and repair stale or incomplete evidence: visual Items, landmarks, relations, references, claims, verification, coverage, quality evidence, sync, and re-pin. | `code-evidence` plus traceability and quality portions now scattered or absent. |
| **Change understanding and review** | Authors explain a change; reviewers observe or compare, inspect affected knowledge and code, discuss, decide, resume, and retain history. | `comparison`, `reviewer-app`, and `reviews`, deduplicated around human workflows. |
| **Guided authoring and automation** | Authors, maintainers, CI, and agents can bootstrap, validate, discover next actions, ask explicit policy questions, and use stable bounded interfaces. | `agent-loop`, command/query/skill/spec contracts, and validation/safety requirements. |
| **Delivery coordination** | Teams can decompose large work into waves and mergeable items, manage real dependencies and versioned contracts, assign workspaces, and distinguish progress from proof. | New feature backed by the implemented `plan`, readiness, and work-query surface. |

CLI, browser, JSON query, and agent skill are channels across these domains,
not features. Git-native storage, security, accessibility, performance,
determinism, conflict resistance, and stable identity are cross-cutting design
constraints, not domains.

## Recommended canonicalization sequence

1. Establish the five-feature taxonomy and retain existing record identities
   while moving stories only where necessary.
2. Rewrite the genuine value stories and demote the architecture/UI/command
   stories according to the disposition table.
3. Add stories for the largest implemented omissions: prototyping, quality
   policy/evidence, claims/verification, structured automation, feature flags,
   delivery planning, and evidence repair.
4. Separate the broad `change-author` role into contextual jobs in story prose;
   add new personas only after confirming distinct humans and goals.
5. Add design coverage for the resulting product stories, starting with the
   cross-cutting invariants that currently sit inside stories.
6. Connect existing automated tests to criteria as quality records before
   inventing new tests. The repository already has extensive tests; the Saga's
   two-of-sixty-nine result primarily reflects missing documentation links.
7. Add one real review artifact for a representative change and document the
   review/resumption workflow through observable criteria.
8. Repair the stale sidebar reference, then expand code evidence by capability
   rather than chasing a superficial coverage percentage.

The target is not a larger Saga for its own sake. It is a smaller set of clear
human stories supported by explicit design invariants, implementation notes,
quality evidence, and code links—enough that the Saga describes the product
without merely mirroring its commands or package structure.
