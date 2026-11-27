# Proposed deployment of a separately maintained Saga {#proposed-hosting}

This is a proposed design for the existing cloud-hosting story. It records no deployed service, granted cloud access or completed verification. The desired boundary is a hosted reader whose documentation repository can differ from the source repository it explains.

## Separate the three identities {#hosting-identities}

The deployment should distinguish the host endpoint, the Saga repository and the target source repository. A host configuration selects the Saga explicitly and retains the target's canonical repository identity and exact source context. A similarly named checkout must not substitute for either repository. Reads of a missing or inaccessible Saga should remain unavailable instead of presenting an empty or different documentation set.

The proposed flow is: obtain the configured Saga, resolve its source association, serve the reader, and follow an exact repository/commit/path reference to source. A remote reference should preserve historical commit context when current source moves. An inaccessible pin must remain an explicit failure rather than silently opening current content.

## Operational boundary and open decisions {#hosting-boundary}

The host must retain enough durable configuration to select the same Saga after restart. Credentials and checkout mechanics belong to the deployment's access boundary, not to authored content or transport URLs stored as canonical repository identity. Authentication, tenant isolation, credential ownership, approval and grant/revoke authority, update scheduling and infrastructure provisioning still need explicit decisions before implementation or deployment can be claimed.

The associated quality definitions describe an authorized staging deployment, separate-repository reading and exact remote navigation. They are plans for verification, not executions or evidence that cloud hosting already exists.
