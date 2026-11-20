# Keep knowledge reviewable in Git {#keep-knowledge-reviewable-in-git}

The Saga stores small, independently authored records and assets. Stable identity survives new definitions, moves and ordinary branch integration. Git commits keep older intent inspectable; current records contain only editable current content and state.

Supported writers publish current records under the Saga lock. Independent additions can merge naturally; concurrent edits to the same file use normal Git merge conflicts. Current state records the authored reason, and Git preserves earlier content and decisions.

File publication is checked and cooperative, but multi-file writes are not a distributed transaction. The [storage essay](/technical/pages/storage) explains locks, staging, rollback and recovery boundaries.
