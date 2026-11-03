# Features and Roadmap

Read this reference to plan a feature, move one between Features and Roadmap, or
assign priorities, milestones and labels. Read [stories](stories.md) for how to name
and frame a feature, and [reorganize](reorganize.md) to deliver a Roadmap feature.

Features and Roadmap share the same feature records, Essay pages, stories, and
optional theme memberships. Features shows built capabilities; Roadmap shows
planned capabilities. Use `feature add --status planned` for future work and
`feature set-plan --feature ID --status planned|built` to move an existing
feature without changing its identity or content. `--priority N` ranks work
(lower positive numbers first; zero is unranked), and `--milestone ID|URN` references an
optional shared milestone. Discover `query labels` and `query milestones` first;
reuse existing entities. Create missing ones with `label add` or `milestone add`
using `--id ID --title TEXT`. Repeat `--label ID|URN` to assign a label set;
`--clear-labels` removes it. Preserve omitted fields; an explicit empty milestone
clears it. Rename the shared entity once with `label rename --label ID|URN
--title TEXT` or `milestone rename --milestone ID|URN --title TEXT`. Never copy
label or milestone display names into each feature. Features and Roadmap remember
independent None, Theme, Persona, Label, or Milestone grouping preferences. Do not infer built status from accepted stories or coverage,
or invent priorities or milestones. `query roadmap` discovers planned work with
snapshot-bound pagination; ordinary `query context --feature ID` includes its
plan. Existing features without a plan retain built placement for compatibility.
