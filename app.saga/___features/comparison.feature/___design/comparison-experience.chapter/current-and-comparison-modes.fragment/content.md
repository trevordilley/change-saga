# Current-state and comparison modes {#current-and-comparison-modes}

The reviewer has one documentation information architecture and two explicit
ways to open it. Current-state mode answers “what is true now?” Comparison mode
answers “what does this proposed change alter or put at risk?” Routes, status,
and structured queries share the same opening context.

## The active mode is always visible {#mode-indicator}

A persistent header label reads **Current state** or **Comparing with _ref_**.
Comparison mode also shows resolved base and head commits and offers **Return to
current state**. Color is supplementary; label, icon, page title, and accessible
announcement all carry the distinction.

A copied deep link includes the mode and selected range. Opening a current-state
link cannot silently inherit an earlier comparison session, and opening a
comparison link cannot fall back to current state if its range is invalid.

## Current state is the default product truth {#current-state-mode}

Without a comparison selection, pages show current personas, features, stories,
criteria, designs, tests, claims, and implementation evidence at the selected
head. Retired or replaced material appears through history, not mixed into the
current body. Stale evidence remains visible as a health warning because it is
part of the current record’s condition.

Current state contains no diff-only empty panels and makes no per-change
coverage claim. It retains entry points to related reviews and to the comparison
that introduced a record.

## One comparison selection scopes every surface {#comparison-selection}

A comparison resolves the merge base of `against` and `head`; that merge base,
not the named branch tip, is the before state. The resolved range is a single
opening-context object consumed by reviewer pages, status, and structured
queries. Each response echoes the same against, head, and resolved commit IDs.

Changing the comparison invalidates cursors and derived layer results as one
transaction. Unsupported, missing, unrelated, or mismatched repository
revisions produce a diagnostic and no partial comparison. In a companion Saga,
the same context also carries the aligned documentation and source ranges.

## Product context stays reachable during review {#comparison-context}

Comparison mode emphasizes changed and affected records, but the unchanged
current documentation remains navigable through the same sidebar and links.
Opening context does not create a second copy of the Saga. A compared record
can be viewed as before/after or opened in current context, and returning
restores the selected layer, filters, expansion, and scroll target.

The page never attaches approval or comments to living documentation. If a
review exists, discussion and decisions remain on its review slide while links
back to current product context stay one step away.

## Related reviews are derived context {#related-review-context}

A current feature, story, or criterion lists reviews whose changed code
intersects code reached through that record’s evidence paths. The list is
labeled **Derived from code links**, including when empty. Each entry opens the
review’s frozen comparison; authors cannot add or remove entries by editing the
record.
