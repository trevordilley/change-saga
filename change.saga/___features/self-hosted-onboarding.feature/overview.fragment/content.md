## Intended outcome {#intended-outcome}

Provide a UI that makes repository setup less confusing: configure the target repository containing the application and set up the separate repository holding its Saga. These repositories have different purposes, and the flow should help users understand which one they are configuring.

## Prerequisite {#prerequisite}

Depends on [Self-hostable server mode](urn:change-saga:change-saga:feature:self-hostable-server). This relationship is documented here while [Roadmap dependency mapping](urn:change-saga:change-saga:feature:roadmap-dependency-mapping) remains planned.

## Scope {#scope}

The confirmed focus is repository configuration through the UI. Cloud provisioning is not part of the currently specified onboarding flow. The host arranges authentication and access controls as described by the server feature.

## Open decisions {#open-decisions}

- Whether setting up the Saga repository means creating a new remote repository, connecting an existing one, initializing Saga content, or some combination.
- How repository credentials are supplied and checked.
- The completion check and recovery experience for invalid or inaccessible repositories.

## Design and handoff {#design-and-handoff}

This feature is planned. Its proposed story covers the two confirmed repository setup tasks. Develop the UX flow and prototype around those tasks, then resolve the open decisions before finalizing acceptance criteria and implementation design.
