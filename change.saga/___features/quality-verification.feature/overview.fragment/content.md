# Define how a requirement is verified {#define-verification}

Quality describes a verification approach and where it is implemented. A test case connects a story's acceptance criteria to concrete preconditions, ordered actions and expected outcomes. Its definition is useful before test code exists. The same case appears under a planned Roadmap feature and under that feature after delivery.

Assertions can name the exact criteria they exercise. Implementation evidence connects stable steps to the automated test code, and may also identify the product code under test. These are references to canonical records, not copies of requirements or code. Manual and setup steps do not need code links. A linked test file establishes only what its cited assertions exercise; it does not establish every obligation in a broader story.

## Preserve definitions as the product changes {#preserve-definitions-as-the-product-changes}

Requirement relations and evidence retain their revision pins. Revising a criterion or case can make a link stale; changing source code can require reassessment of its evidence. Authors append revisions and superseding evidence after inspecting those changes. Competing heads remain visible until deliberately reconciled. A planned case without implementation evidence is valid, and its absence is a useful statement of work still to do.

## Keep execution history separate {#keep-execution-history-separate}

Quality coverage and readiness describe defined verification, missing mappings, policy mismatches and stale references. They do not report execution success or failure. CI results do not mark Saga documents passed or failed. Historical runs remain available through the explicit quality-history query and the deprecated compatibility writer; recorded command text is inert and must never be executed by a reader.

## This repository's verification definitions {#this-repository-s-verification-definitions}

The repository's cases are proposed drafts grounded in the current user stories and inspected source at `cc75d453`. They retain exact criterion and step mappings and link only evidence that was actually inspected. Some steps have automation, some have partial evidence, and others describe planned manual verification. No execution result is implied by any of these definitions.

The accompanying [Verification definitions and gaps](/features/quality-verification#target-urn-change-saga-change-saga-fragment-verification-definition-f938bc30ae8f) document records the criterion-by-criterion assessment, evidence limits and missing tests. The retired confidence story and deferred hosting work are left unchanged. Older technical diagrams that describe run-gated Quality are historical explanations requiring reassessment; the current Quality stories and case definitions express the corrected model.
