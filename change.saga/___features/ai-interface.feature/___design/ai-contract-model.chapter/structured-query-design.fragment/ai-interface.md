# AI-facing interface model {#ai-facing-interface-model}

AI clients use domain operations rather than discovering storage paths. `change-saga query` publishes a versioned JSON envelope, operation-specific purpose and usage, stable error codes, snapshot identity, and bounded pagination.

The transport adapter opens a validated application session and returns domain resources addressed by stable URNs. Authoring commands use the same published grammar and validated mutation layer, keeping filesystem layout out of the client contract.
