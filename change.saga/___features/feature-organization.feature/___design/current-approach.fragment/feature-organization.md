# One feature identity across planning and delivery {#one-feature-identity}

A feature's delivery category is editable planning metadata rather than another Product record. Features projects built capabilities; Roadmap projects planned capabilities. Changing that category keeps the same feature URL, stories, essay, design and prototypes. Roadmap sorts positive priorities from lower to higher and leaves unranked work after ranked work, preserving stable order for ties. Requirement acceptance and delivery category answer different questions.[^projection]

## Share categories instead of copying names {#shared-categories}

A label or milestone owns one stable ID and editable display title. Feature plans retain canonical references: labels are a set, while a feature has at most one milestone. Two features can share the same definition, so a rename updates presentation without rewriting assignments. Assignment validates same-Saga references before replacing the plan; duplicate classification names are checked without regard to letter case. Omitted assignments are preserved, while explicit replacement or clearing changes only the requested fields.[^assignments]

For example, two planned features can share the milestone “Hosted reading” and one label while retaining different priorities. Renaming that milestone changes both displayed names; moving one feature to built does not delete its label or milestone reference. These are organizational memberships, not inferred dependencies or review approvals.[^assignments][^projection]

## Presentation groups keep canonical destinations {#independent-groups}

Features and Roadmap have separate grouping choices. None, Persona, Label and Milestone organize the same feature records; Theme is available when authored themes exist. A feature with several labels can appear in several groups, but each row opens the same canonical page. Unassigned features stay visible. Search and directory presentation resolve the current shared names instead of persisting display-name copies in every feature.[^projection]

This design keeps status, priority, shared categories and grouping preferences distinct. Promotion changes which collection projects the feature; a grouping preference changes how a collection is read. Neither operation rewrites product intent to fit the presentation.[^projection]

[^assignments]: Feature plan mutation validates canonical shared classification assignments before publication.
[^projection]: Feature collections and grouping project existing records without changing their identity.
