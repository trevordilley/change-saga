## Intended outcome {#intended-outcome}

Let the product owner map dependencies between Roadmap features and communicate those prerequisites to the team alongside the planned capabilities.

## First use case {#first-use-case}

[Self-hosted onboarding](urn:change-saga:change-saga:feature:self-hosted-onboarding) depends on [Self-hostable server mode](urn:change-saga:change-saga:feature:self-hostable-server). A reader should be able to understand the direction of that prerequisite relationship.

## Questions for discovery {#questions-for-discovery}

- Should dependencies appear as links, a visual graph, or both?
- Are dependencies advisory, or should unmet prerequisites prevent marking a dependent feature built?
- What should happen to a dependency when its prerequisite moves from Roadmap to Features?
- Should the first release reject circular dependencies?

## Design and handoff {#design-and-handoff}

This feature is planned; structured dependency mapping is not implemented by adding this entry. Define the dependency behavior and observable acceptance criteria before designing its authoring and reading experience. Reuse the existing feature identity and planning model where appropriate.
