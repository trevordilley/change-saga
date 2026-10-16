package grammar

func init() {
	commands = append(commands, Command{Name: "feature set-plan", Status: StatusImplemented, Mutates: true, Writes: []string{"feature-plan"}, Usage: "change-saga feature set-plan --feature ID|URN [--status planned|built] [--milestone TEXT] [--priority N] [--json] <saga>", Summary: "set product planning metadata without changing feature identity, stories, or history", Flags: []Flag{required("feature", "ID|URN", "existing feature"), optional("status", "planned|built", "Roadmap or Features; omitted preserves it"), optional("milestone", "TEXT", "version or milestone; empty clears it"), optional("priority", "N", "lower positive numbers come first; zero clears it"), jsonFlag}, Positionals: sagaOnly})
	resources = append(resources, Resource{Kind: "feature-plan", URN: "urn:change-saga:<saga>:feature:<feature>", Storage: "___features/<feature>.feature/plan.json", Schema: schemaBase + "v5/feature-plan.schema.json", Versions: []int{5}, History: "editable current snapshot; Git preserves planning history", Writers: []string{"feature add", "feature set-plan"}, Notes: "planned features appear on Roadmap; built features appear on Features; absent plan preserves legacy built placement; priority and milestone are explicit product intent, never inferred from coverage"})

	for _, op := range []string{"add", "set-overview", "set-features"} {
		flags := []Flag{jsonFlag}
		usage := "change-saga feature-theme " + op
		if op == "add" {
			flags = append(flags, required("id", "ID", "stable product theme id"), required("title", "TEXT", "product theme title"))
			usage += " --id ID --title TEXT"
		} else {
			flags = append(flags, required("theme", "ID|URN", "product theme selector"))
			usage += " --theme ID|URN"
		}
		if op != "set-features" {
			flags = append(flags, optional("text", "MARKDOWN", "authored Markdown overview"), optional("source", "FILE|-", "Markdown source, or standard input"), optional("subtext", "TEXT", "one-line essay subtext"), optional("abstract", "MARKDOWN", "separate essay abstract"))
			usage += " (--text MARKDOWN | --source FILE|-)"
		}
		if op != "set-overview" {
			flags = append(flags, repeatable("feature", "ID|URN", "member feature; omitted clears membership on set-features", false))
			usage += " [--feature ID|URN...]"
		}
		commands = append(commands, Command{Name: "feature-theme " + op, Status: StatusImplemented, Mutates: true, Writes: []string{"feature-theme"}, Usage: usage + " [--json] <saga>", Summary: "author optional product categorization without moving features; exact retries are idempotent", Flags: flags, Positionals: sagaOnly})
	}
	resources = append(resources, Resource{Kind: "feature-theme", URN: "urn:change-saga:<saga>:feature-theme:<theme>", Storage: "___feature-themes/<theme>.theme/theme.json", Schema: schemaBase + "v5/feature-theme.schema.json", Versions: []int{5}, History: "current snapshot; Git preserves authored Markdown overview and membership edits", Writers: []string{"feature-theme add", "feature-theme set-overview", "feature-theme set-features"}, Notes: "optional many-to-many feature membership; normal rationale deck under ___slides with role feature-theme; query feature-themes discovers overview, membership and decks; visual theme styling is separate"})
}
