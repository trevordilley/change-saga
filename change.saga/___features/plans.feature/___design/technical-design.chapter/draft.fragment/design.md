**Draft. Nothing here is built.** The decision to rebuild is made; the rest is a proposal. This sits in a feature design chapter only because a Plan record does not exist yet. Once it does, this is what a Plan's deck and essay would hold.

The Plan is where the technical design for a change is drafted, and its Tasks are how the work gets done.

## The work plan {#the-work-plan}

**Decision: keep the idea and rebuild the implementation, as what we want it to be.** No Saga outside this repository uses the work plan, so no migration is needed. The concepts match Tasks and Phases. The code is connected but unused, and most of what it records is not what a Task needs.

What an audit of `internal/workplan` and its callers found:

| Finding | Evidence |
| --- | --- |
| Nobody uses it here | `query work-items` and `query waves` return nothing for this Saga. It has no `implements` relations, the one relation type only a work item can source |
| It has no reader | No route or template shows a wave or work item. `internal/server/apppages.go:283` would only list a work item that implements a story |
| Agents are barely taught it | No skill workflow routes to its commands. CLI help (`internal/cli/cli.go:357`, `:490-498`), `README.md:628`, and reference lists do describe it |
| It has no end-to-end test | Nothing in `e2e/tests` uses it. It has Go tests only |
| It has only been ported since it landed | Later commits to the package are format and rename work. The largest, `f4ad3085`, added per-feature loading and ID uniqueness |
| Part of it cannot be reached | `RecordContractState` and `ReviseContract` have no callers outside tests, so a `contract_fulfilled` dependency cannot be satisfied through any public command |
| Its readiness axis is empty | `query readiness` reports `plan_coverage` at 0 of 29 accepted criteria, because no work items exist. Delivery is also 0 of 29, for a separate reason below |
| Its model is heavier than a Task needs | Immutable revisions, append-only event graphs, and competing heads. Stories, prototypes, and terms still use revisions and events, so this is a direction for Tasks, not something the whole Saga has already done |

What carries over in spirit is small: a dependency edge, cycle detection (`internal/workplan/validate.go:278-330`), and readiness of a Task from its prerequisites (`internal/workplan/readiness.go`).

Dropped, with what replaces each:

- **Revisions, events, heads, request IDs:** Git. A Task is one file, and changing it is a commit.
- **Merge units and merge events:** a Plan's Reviews, which already record when their change landed.
- **Workspace assignment:** nothing yet. Which agent holds a Task is the orchestrator's concern until someone needs it recorded.
- **Contracts:** nothing yet. They were unreachable, so removing them loses no working behaviour. Worth reconsidering if parallel Tasks start colliding over an interface.
- **Touch areas, deliverables, completion checks:** code references and the essay.
- **Dependency conditions:** a Task's `depends_on` is satisfied when the prerequisite Task is done. `merge_integrated` and `contract_fulfilled` go away.

Kept:

- **`--request-id` on writes.** Git does not make a retried command safe, and other commands declare it.

### What this gives up {#what-this-gives-up}

- **`plan record-merge` is the only path from a criterion to a merge commit** (`internal/livingapp/compose.go:651`). A Review records when it merged, but links to criteria only through `explains`. Nothing here uses that path today.
- **Tasks will work differently from stories and terms,** which keep revisions and event graphs. A Task's history is in Git.
- **Conflicting state stops being kept visible.** If parallel agents turn out to need it, it comes back as a deliberate design.

## Records {#records}

```text
change.saga/___plans/<plan-id>.plan/
  plan.json              id, title, phases, prototypes, reviews
  deck/                  a flat deck bundle, role plan, as a review's deck is
  tasks/<task-id>.task/
    task.json            title, phase, story, depends_on, code, sizing, state, summary
    essay.md             optional
```

```json
{
  "id": "merge-grammar",
  "title": "Replace the old plan verbs",
  "phase": "first-phase",
  "depends_on": ["remove-work-plan"],
  "code": [{"path": "internal/cli/cli.go", "start": 1, "end": 40, "commit": "<full commit>", "digest": "sha256:<64 hex>", "note": "Example only"}],
  "sizing": {"within_two_days": true, "reason": "One command family"},
  "state": "planned"
}
```

Each design choice serves one rule:

- **One file per Task,** so agents on parallel branches marking different Tasks done touch different files. The same Task marked done on two branches conflicts only if the two records differ.
- **`depends_on` is stored; `unblocks` is derived.** Adding a Task then edits only that Task. `task add --unblocks OTHER` writes to the other Task, which is the rare case.
- **A Task leads to a story.** It has `story`, or it unblocks a Task that leads to one. Validation refuses a Task that reaches none, and a dependency cycle. `story` holds a URN, which `feature deliver` preserves.
- **The deck is a review's deck in a new role.** `internal/saga/review.go` already loads a flat bundle under a caller's targets, so Plans can reuse the loader with their own URNs (`urn:change-saga:SAGA:plan:ID`). A slide Item names a Task through `record`, which needs work: `validateDeckRole` refuses `record` for a new role (`internal/saga/slides.go:624`), and `task` must join the allowed kinds.
- **Done state is read at render time,** never written into a slide.

## Reader {#reader}

A third section beside Documentation and Reviews (`internal/server/appnav.go`), with routes `/plans`, `/plans/{id}`, and `/plans/{id}/tasks/{task}`. Plan pages reuse the deck viewer and essay. The [prototype](/prototypes/plans) shows the intended surfaces.

## Reviews {#reviews}

A Plan's `reviews` and a Review's `plan` reference each other, and share one Git history. A Plan is done when its Reviews have merged and its Tasks are done, subject to the open question in the essay. The comparison reads both at the Review's head.

## Readiness {#readiness}

`query readiness` has two axes that read 0 of 29 here, for different reasons:

- **Plan coverage** counts work items, so it is empty because none exist. It should count Tasks, or go.
- **Delivery coverage has nothing to do with work items.** It counts evidence from a `verifies` relation whose source is a claim or verification (`internal/livingapp/compose.go:447-462`). All 283 `verifies` records here come from test cases, which it ignores. That is an existing defect, or at least a surprising rule, to look at on its own. Removing the work plan neither causes nor fixes it.

So readiness is a separate change. It should not be bundled with removal, beyond keeping plan coverage from breaking.

Other code also depends on the work plan loading: `internal/livingapp/status_load.go:198-203`, `internal/cli/relation_currency.go:49-54`, `internal/livingapp/audit.go:201`, `query traceability`'s `work_items`, and `feature_context.go:206`.

## What is deleted {#what-is-deleted}

| Removed | Size or location |
| --- | --- |
| `internal/workplan` | 2,698 lines, plus 543 of tests |
| `internal/cli/living_plan.go`, and the `plan` family's old verbs | 447 lines; `grammar` and `cli.go` registration |
| Queries `waves`, `work-items`, `work-events`, `work-conflicts` | `internal/cli/query.go`, `query_reviewapp.go`, `internal/livingapp/compose.go`, `types.go`, `testdata/query/help.golden`, `references/query.md` |
| Work-plan reads in audit, staleness, and authoring | `internal/livingapp/audit.go`, `session.go`, `status_load.go`, `internal/cli/relation_currency.go`, `app_authoring.go` |
| Relation type `implements`, and work-plan endpoint kinds | `internal/requirements/urn.go`, `relation_v5.go`, `validate.go`, `internal/grammar`, `changeview`, `schema/v3` and `schema/v5/relation*.json` |
| Identity and target kinds | `internal/livingid/urn.go`, `internal/sagaref/target.go`, `internal/semanticcheck/check.go:207`, `internal/server/apppages.go:283`, `internal/cli/living_requirements.go:518` |
| Delivery and removal paths | `internal/cli/feature_deliver.go:327`, `record_remove.go:428`, `references/reorganize.md:84` |
| Tests | `living_mutation_test`, `session_test`, `ergonomics_test`, `app_authoring_test`, `skill_test`, `saga/load_test`, `schema_v5_contract_test`, `relation_verify_loading_test`, and query goldens |
| `___workplan` as a reserved directory | `internal/applayout/applayout.go`, `internal/cli/cli.go:1415`, `SPEC.md`, `references/format.md`, `setup_ci_prompt.md` |
| Schemas and docs | `schema/v3` wave, work-item, dependency, contract, revision, and event schemas; `docs/living-saga-work-plan-api.md`, `living-saga-api.md`, `living-saga-implementation-map.md`, and about 14 others; `README.md` |
| The Saga's own account | The Dependency-aware work planning feature, deck, and stories, folded into this one; terms `work-item`, `wave`, `contract`, and `merge-unit`, which pin this code; references in `quality-verification` and `product-knowledge` records. CI's health gate depends on these being handled |

Once `___workplan` is no longer reserved, the existing rule applies: other names beginning with `___` are invalid.

## Order of work {#order-of-work}

1. **Remove the work plan.** This frees the `plan` noun and is useful alone. Look at readiness's delivery rule separately.
2. **Plan record and Plans section:** record, deck, `plan create`, read-only pages.
3. **Tasks:** records, validation, Task page, graph, `task` commands.
4. **Slides and done state:** Items naming Tasks, and `task done`.
5. **Reviews:** cross-references, comparison, hiding done Plans.
6. **Guide:** the wizard, and the skill's route. `SKILL.md` tells agents to author Saga content near completion, so a Plan needs its own exception.

## Risks {#risks}

- **Readiness behaviour changes.** Its output shape and meaning are public through `query`, so plan coverage needs a deliberate answer.
- **Dropping contracts and assignment early.** If parallel agents need them, they return as a deliberate feature with a CLI and a reader.
- **Not verified:** the line counts and locations above come from reading and searching the code, and an independent review of it. No removal has been tried, so tests may reveal further coupling.
