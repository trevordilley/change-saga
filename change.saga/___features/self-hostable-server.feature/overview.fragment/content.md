## Intended outcome {#intended-outcome}

Run a Change Saga server in the user's own cloud. It serves the Saga UI while Saga content is maintained outside the target repository. Remote references connect the documentation to the material it describes in that target repository.

## Responsibility boundary {#responsibility-boundary}

The host arranges authentication and access controls. This feature does not require a built-in authentication provider or access-management system. Identifying who makes edits may still be needed; the attribution requirement and identity handoff remain open.

[Self-hosted onboarding](urn:change-saga:change-saga:feature:self-hosted-onboarding) depends on this server capability.

## Open decisions {#open-decisions}

- Whether editor attribution is required initially and how the host supplies the editor's identity.
- Repository authentication and private-repository support.
- Which remote reference types are needed initially, how revisions are pinned or followed, and how unavailable references appear.
- Deployment packaging and persistence details.

## Design and handoff {#design-and-handoff}

This feature is planned. The initial proposed story records the confirmed outcomes; deployment and identity design remain to be worked out. Supporting technical Essays, diagrams, and implementation design can be developed before handoff.
