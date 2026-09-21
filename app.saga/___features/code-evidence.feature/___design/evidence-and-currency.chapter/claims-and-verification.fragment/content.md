# Claims and independent verification {#claims-and-verification}

A claim is an author’s falsifiable assertion about one explanation. A
verification is an independent, append-only result that tested that claim.
They are displayed together but never merged into one editable status.

## Author claim contract {#author-claim}

Creating a claim requires exactly one testable statement, one explanation
target, and one or more exact source references. The statement must be capable
of being contradicted; intentions such as “this is safe” are rejected unless
they state what observation would make them false.

The claim card leads with **Author claim**, shows its target and captured source
locations, and labels each reference’s currency. A stale reference makes the
supporting evidence stale; it does not rewrite or automatically fail the claim.
Claims cannot target a vague feature or free-floating page when an addressable
explanation landmark or Item exists.

## Verification entry {#verification-entry}

A verifier records one result—verified, failed, or inconclusive—plus the method
used and a concise summary of what was observed. Optional evidence references
identify the exact independent material examined. The UI names the verifier and
time and never substitutes the author’s claim evidence for the verification
method.

A result is interpreted only against the claim revision and evidence it names.
“Inconclusive” is a first-class result, not an empty verification. Recording a
new result does not select a winner or change the claim.

## Immutable verification history {#verification-history}

Verification entries form a chronological history beneath the unchanged claim.
Every result remains visible, including failures superseded by later success.
The newest entry is visually prominent, while a summary counts verified,
failed, and inconclusive results separately.

Editing is modeled as a new event or, when the assertion itself changes, a new
claim. Deleting or rewriting an earlier independent result is not offered. The
history therefore lets a reviewer distinguish “the assertion changed,” “the
evidence changed,” and “another verifier reached a different result.”
