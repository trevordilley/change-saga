**Planned: none of this is built.** Every story here is proposed, and the commands they name are working names. A story marked `must` is what Trevor asked for; `should` is an addition proposed for him to accept or strike. When built, these stories move into [Quality verification](/features/quality-verification).

## The problem {#the-problem}

Unit and integration tests are useful, but the ones that let a team sleep at night are end-to-end tests: they run the real app the way a user does. They are also the flakiest and the most expensive to run, so a team needs to know exactly what it has, what each one proves, and which ones a given change puts in play.

Today, nobody can answer that from a Saga. Quality lives inside each feature, with no place to see every end-to-end test, and nothing distinguishes one from a unit test. A criterion can look covered while only its happy path is ever tested.

## The question at the centre {#the-question-at-the-centre}

**If I change this line of code, which end-to-end tests do I need to run to prove the stories it touches still work?**

[One command answers that](/requirements/find-e2e-tests-for-code) for a line or for a range. Reviews and releases both work on code ranges, so they ask the same question and get the same answer. It follows the Saga's links from the code to stories, stories to their criteria, and criteria to tests, then [prints the tests in a form a runner accepts](/requirements/get-tests-in-a-runnable-form).

That flow is transitive, which is the point of Change Saga. A line an Item cites reaches that Item's criteria directly. Failing that, it reaches the feature holding that Item, and so that feature's stories. A line nothing cites, in a file a feature's records do cite, reaches that feature through its file. Each step casts a wider net, so the answer says which path found each test, and `--direct` keeps only the specific ones.

The tool answers and people decide. [It says what it could not reach](/requirements/see-what-selection-cannot-reach), so a short list never reads as proof that the rest is safe, and it never says whether the answer is good enough.

## The catalog {#the-catalog}

The QA section of the Overview holds [a catalog of every end-to-end test used to prove the app works](/requirements/see-the-e2e-catalog). The [recommended unit is one runnable test](/requirements/one-catalog-entry-per-test): a `test()` is what runs, what costs money, what flakes, and what is positive or negative.

Each entry has [code references to the test that runs it](/requirements/open-e2e-test-code), and [each acceptance criterion on a built feature links to the entries that verify it](/requirements/see-criteria-e2e-covers). Not every criterion needs an end-to-end test, so a missing one is simply shown.

## Past the happy path {#past-the-happy-path}

Covering every criterion with happy paths only proves the app works in ideal circumstances. So [each entry shows whether it is positive, negative or edge](/requirements/see-positive-and-negative-coverage), and each criterion shows which of those its tests cover. A criterion with only positive coverage is visibly weaker than one with negative coverage too.

These are facts to read and not a grade. They are also claims: a kind is what the author stated, and the tool does not check it.

## A measure, not a rule {#a-measure-not-a-rule}

All of this is informative and not prescriptive. Nothing here fails a check, adds a health gap or asks a team to write a test. Some apps have no end-to-end tests at all and are fine.

It covers end-to-end tests only. Integration and unit tests are just part of the code, so they have no catalog entries and no counts here, though the code they exercise is still part of how a change reaches a story.

## QA tools {#qa-tools}

With those links in place, the rest is asking questions. People can [slice the end-to-end tests by feature, story, persona, kind or code path](/requirements/slice-test-coverage), and run that slice. They can also [see where those tests are thin](/requirements/find-weak-spots-in-testing): criteria with only positive coverage, criteria with no end-to-end test, tests that verify nothing, and tests gone stale. More tools can follow, because each is a view over the same records.

## It uses what Quality already has {#it-uses-what-quality-already-has}

Quality already has most of these parts, so this adds no new record and no second catalog.

| Idea | What already exists |
| --- | --- |
| A catalog entry | A test case |
| A criterion's reference to an entry | A pinned `verifies` relation |
| An entry's code references | `test_implementation` evidence, which pins exact lines and goes stale |
| Positive and negative | `coverage_kinds`, which also has `edge` for limits and concurrency |
| Walking a range to tests | `review test-plan`, which today can only start from a review |

What is missing is a way to tell which test cases are end to end, a way to ask about any line or range, and a place to see it all together. The recommendation is to [declare where the end-to-end tests live](/requirements/say-where-e2e-tests-live) and derive the rest from where each test case's code is.

## What is true today {#what-is-true-today}

Measured on this Saga:

- This repository has 139 `test()` calls in 43 spec files under `e2e/tests`, and test cases point at 11 of those files.
- 27 of its 182 test cases have code in `e2e/`, reaching 16 distinct `test()` blocks by a rough parse.
- 51 of the 243 current criteria on built features have an end-to-end test. 46 of the 51 are positive only, and none has a negative end-to-end test.
- Of the 58 references into `e2e/tests`, 4 start on a `test()` line and 54 sit inside a test body or helper. They read well, but Playwright finds no test at a line inside a body, so most would need a [reference to the declaring line](/requirements/open-e2e-test-code) before a command could run them.
- On v0.6.0 to v0.7.0, 1,378 of 17,802 changed lines reach a story by exact line, and following files, features and Tasks raises that to at most 4,617. Most of the rest is product code in features with no implementation deck, such as `internal/server/plans.go`, plus 3,248 lines of the deleted work plan. The walk cannot invent links that were never recorded, so writing the Plans feature's deck would do more here than any change to the walk.

These are counts from the records, and kinds are as their authors stated them. A team that wants to fill any of this in can have an agent do it, and nothing asks them to.

## Limits {#limits}

The Saga records how something is verified, never whether it passed. The catalog holds no results and no flakiness history; those stay in the team's test runner and CI. It also cannot tell whether a test proves what it claims, and a person still has to judge that.
