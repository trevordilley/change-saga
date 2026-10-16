# A stable contract for coding tools {#a-stable-contract-for-coding-tools}

An author or coding assistant should be able to discover the application through a bounded, machine-readable contract. Queries return structured data with snapshot identity and explicit pagination. Authoring commands validate typed requests and return stable targets that the next operation can use.

Use feature context to start small, expand the relevant story, and follow cursors to completion. A changed snapshot invalidates continuation rather than silently mixing answers. CLI help, query schema and the compiled spec are the current contract; saved examples are guidance.

Querying is read-only. A suggested next action is not permission to accept a requirement or a review. See the [authoring workflow](/technical/pages/workflow).
