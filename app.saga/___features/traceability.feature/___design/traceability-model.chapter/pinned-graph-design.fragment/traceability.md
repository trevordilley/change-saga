# Traceability model {#traceability-model}

Stable resource identities are connected by typed relations. Mutable endpoints are pinned to the exact story revision, design digest, or other definition used when the relation was authored. Currency is derived by comparing those pins with current heads; stale and conflicted paths remain visible instead of being silently repaired.

Implementation Items and quality evidence carry commit-pinned code references. The impact projection follows only persisted relation edges, allowing forward traversal from intent to code and reverse lookup from code to the product knowledge it may affect.
