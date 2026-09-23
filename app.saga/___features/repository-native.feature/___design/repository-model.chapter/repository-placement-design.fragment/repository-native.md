# Source and companion repository model {#source-and-companion-repository-model}

Every Saga declares one canonical source-repository identity. When the Saga lives inside that source checkout, it describes the commit at which it is read. When it lives in a companion repository, a sync cursor records the exact source commit documented by the current Saga revision.

Code references remain portable because they combine canonical repository identity, commit, path, range, and digest. Repository verification rejects accidental use against a different checkout unless the caller explicitly overrides it.
