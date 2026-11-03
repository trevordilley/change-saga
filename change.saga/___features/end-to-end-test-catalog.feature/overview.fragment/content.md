**Planned: none of this is built.** Every story here is proposed, and the commands they name are working names. A story marked `must` is what Trevor asked for; `should` is an addition proposed for him to accept or strike. When built, these stories move into [Quality verification](/features/quality-verification).

## The problem {#the-problem}

Unit and integration tests are useful, but the ones that let a team sleep at night are end-to-end tests: they run the real app the way a user does. They are also the flakiest and the most expensive to run, so a team needs to know exactly what it has and what each one proves.

Today, nobody can answer that from a Saga. Quality lives inside each feature, with no place to see every end-to-end test, and nothing distinguishes one from a unit test.

## What this adds {#what-this-adds}

The QA section of the Overview holds **a catalog of every end-to-end test used to prove the app works**. Each entry has code references to the test that runs it, and each acceptance criterion on a built feature links to the entries that verify it. So anyone can read from a promise to the test that proves it to the code that runs it, in either direction.

This is also what lets [a release choose which tests to run](/requirements/select-e2e-tests-for-a-release) rather than running them all.

## It uses what Quality already has {#it-uses-what-quality-already-has}

Quality already has most of these parts, so this adds no new record and no second catalog.

| Trevor's idea | What already exists |
| --- | --- |
| A catalog entry | A test case |
| A criterion's reference to an entry | A pinned `verifies` relation |
| An entry's code references | `test_implementation` evidence, which pins exact lines and goes stale |

What is missing is a way to tell which test cases are end to end, and a place to see them together. The recommendation is to [declare where the end-to-end tests live](/requirements/say-where-e2e-tests-live) and derive the rest from where each test case's code is.

## What is true today {#what-is-true-today}

Measured on this Saga:

- This repository has 139 `test()` calls in 43 spec files under `e2e/tests`, and test cases point at 11 of those files.
- 27 of its 182 test cases have code in `e2e/`.
- 51 of the 243 current criteria on built features have one of those test cases, and 12 of the 62 accepted ones do.

These are counts from the records, and a criterion with no end-to-end test may be well tested another way. Backfilling is real work, which an agent can do but which this feature does not do by itself.

## Limits {#limits}

The Saga records how something is verified, never whether it passed. The catalog holds no results and no flakiness history; those stay in the team's test runner and CI. It also cannot tell whether a test proves what it claims, and a person still has to judge that.

## Open questions {#open-questions}

1. **Is every criterion expected to have an end-to-end test?** Some are poor fits: a criterion about a refused flag is cheaper to prove lower down, and at 51 of 243 most criteria here have none. Recommendation: report the gap and let a team opt in with `check --covers e2e`. Do not make it a health failure.
2. **What is one catalog entry: a test case, or one `test()`?** A test case can point at several `test()` blocks, and one `test()` can prove several criteria. Recommendation: one entry per test case, as this keeps today's records, with the code references naming the `test()` blocks. The alternative is one per `test()`, which would be nearer a true catalog but means splitting many existing cases.
