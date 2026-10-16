A product owner should be able to describe future work in enough detail for a teammate to take it forward, then keep that knowledge when the feature ships. Change Saga gives that work one identity throughout its life. [Features](/features) shows built capabilities and [Roadmap](/roadmap) shows planned capabilities; both open the same feature reading pages.

## One feature from plan to delivery {#plan-to-delivery}

Built and planned are mutually exclusive delivery categories. They behave like built-in status labels, while ordinary labels and themes can have multiple memberships. Changing delivery status moves a feature between the two views without recreating its Essay, stories, acceptance criteria, prototype associations, design, quality records or implementation deck. Its canonical URL and identity stay the same. Story acceptance and evidence coverage remain separate facts; they do not automatically mark a feature built.

A planned feature can carry the same product and implementation context as a built feature. The owner can articulate outcomes and criteria, explore prototypes and UX, and develop design before handing work to a teammate. Optional numeric priority orders Roadmap work: lower positive numbers come first, unranked work follows, and ties retain creation order. [Dependency-aware work planning](/features/work-planning) describes the separate execution and integration workflow.

## Define organization once {#shared-organization}

A label has one stable identity and one shared display name. Features reference that definition instead of copying a name such as “DevOps” into every record. A feature may reference several labels. Rename the shared definition once and its name changes everywhere it is displayed, with memberships and feature identities preserved. Discover existing labels before creating them; duplicate names within the label catalog are rejected.

A milestone follows the same shared-record pattern, with at most one assigned to a feature. It can name a version, a launch or another target chosen by the owner; a name does not imply a date or a scheduling commitment. Milestones are optional. Omitting labels or a milestone from a planning update preserves them; explicitly clearing them removes the assignment. New assignments require existing shared definitions. Older free-text milestones remain readable for compatibility.

Themes provide another shared way to group features and can carry their own Essay and rationale deck. A simple label needs no rationale document. A theme or label can span built and planned work, while each section shows only the features matching its delivery status.

## Read each collection your way {#independent-grouping}

The menu beside Features and Roadmap offers None, Persona, Label and Milestone, plus Theme when themes exist. Each section remembers its own choice during navigation: grouping Roadmap by milestone does not change Features grouped by label. The choice controls both the directory and the sidebar, even for a collection containing only one feature.

A feature belonging to several labels or themes appears under each applicable group with links to the same record. Unlabeled features and features without milestones remain visible. Shared label and milestone names can be searched in the directories. In this Saga, the shared Product planning label connects this built capability with the planned [Roadmap dependency mapping](/features/roadmap-dependency-mapping) feature.

## Current delivery and boundaries {#delivery-boundaries}

This capability is implemented in commit `59098190c1cf`. The delivered checks cover shared-name renames, stable references, missing and cross-Saga assignments, independent grouping, navigation and promotion between planned and built. Go package and focused CLI/server tests passed, along with three Chromium scenarios for feature organization, themes and Roadmap. Those checks are evidence of the tested behavior, not a product acceptance decision for the stories below.

A dependency graph and Gantt presentation remain future work in [Roadmap dependency mapping](/features/roadmap-dependency-mapping). Self-hosting and its onboarding remain planned. No milestone or schedule is inferred for those features.

Read [Storage and evolution](/technical/pages/storage#shared-feature-organization) for the shared definitions, reference validation and view projection, with exact code evidence attached to that section.
