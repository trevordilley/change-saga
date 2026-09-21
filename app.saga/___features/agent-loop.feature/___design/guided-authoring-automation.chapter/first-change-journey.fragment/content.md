# A useful first change, then optional growth {#first-change-journey}

The first experience earns trust by producing review material immediately. It
asks the author to explain the current implementation change and nothing else.
The product model can grow after value has been demonstrated.

## Guided path {#guided-path}

1. Establish the Saga and the comparison being explained.
2. Name the feature that owns the change, accepting a reversible default when
   the author has not chosen one.
3. Build a short visual argument: orient the reviewer, explain consequential
   changes, and connect every changed line to the item that explains it.
4. Show any unexplained lines as actionable omissions.
5. End the required path when the current change is explained.
6. Offer one optional growth step, such as capturing the implied story or design.

Each step states the human outcome before showing a command or form. The author
can resume from the last durable record, inspect what will change before a
mutation, and decline optional growth without creating an error or warning.

## Reversible information architecture {#reversible-structure}

First-run content receives stable identities independent of its initial feature
placement. Later reorganization changes containment without breaking links from
review items, requirements, comments, or history. Defaults accelerate the first
change; they do not harden into taxonomy.

## Omission states {#authoring-omissions}

The workflow distinguishes missing explanation from unavailable evaluation.
Unexplained changed lines identify exact files and ranges plus the nearest safe
next action. A malformed Saga, mismatched checkout, or ambiguous comparison is
a trust failure and stops guidance until resolved. Missing personas, stories,
design, and tests remain visible optional growth, never disguised as first-run
failure.
