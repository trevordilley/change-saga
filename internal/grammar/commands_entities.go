package grammar

func init() {
	for _, kind := range []string{"feature", "feature-theme", "label", "milestone"} {
		selector := kind
		if kind == "feature-theme" {
			selector = "theme"
		}
		ops := []string{"remove"}
		if kind == "feature" || kind == "feature-theme" {
			ops = append(ops, "rename")
		}
		for _, op := range ops {
			flags := []Flag{required(selector, "ID|URN", "existing record in this Saga")}
			usage := "change-saga " + kind + " " + op + " --" + selector + " ID|URN"
			summary := "remove an unreferenced record; refuses authored content and immutable history"
			if op == "rename" {
				flags = append(flags, required("title", "TEXT", "display title; stable ID and URN are preserved"))
				usage += " --title TEXT"
				summary = "rename a display title while preserving identity, membership, and history"
			}
			if kind == "feature" && op == "remove" {
				flags = append(flags, optional("with-content", "", "delete the feature with everything in it, remove links to it elsewhere, and list what other records need adjusting"))
				usage += " [--with-content]"
				summary = "remove an empty feature, or with --with-content the feature and everything in it; preview with --dry-run"
			}
			flags = append(flags, optional("dry-run", "", "validate and report without writing"), jsonFlag)
			commands = append(commands, Command{Name: kind + " " + op, Status: StatusImplemented, Mutates: true, Writes: []string{kind}, Usage: usage + " [--dry-run] [--json] <saga>", Summary: summary, Flags: flags, Positionals: sagaOnly})
		}
	}
}
