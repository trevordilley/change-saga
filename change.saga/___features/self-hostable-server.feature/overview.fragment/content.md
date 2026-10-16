## Intended outcome {#intended-outcome}

Maintain the Saga outside the target repository and make it available through a self-hostable server. The Saga must retain remote references to the relevant material in the target repository so readers can connect product and technical documentation to the application it describes.

## Confirmed scope {#confirmed-scope}

- A self-hostable server mode.
- Saga content can be maintained outside the target repository.
- References to material in the target repository work remotely from that separately maintained Saga.

[Self-hosted onboarding](urn:change-saga:change-saga:feature:self-hosted-onboarding) depends on this capability.

## Questions for discovery {#questions-for-discovery}

- Is the first audience one person with one repository, or a team sharing access?
- Must private repositories work in the first release?
- Which reference types must work remotely, and should references follow a branch, pin a commit, or support both?
- What should a reader see when a reference is unavailable or its source has changed?

## Design and handoff {#design-and-handoff}

This feature is planned. The hosting environment, access model, storage design, and deployment approach remain undecided. Record the agreed stories and acceptance criteria, then develop the technical Essay and supporting design material before implementation.
