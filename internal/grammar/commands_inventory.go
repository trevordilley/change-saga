package grammar

func init() {
	for i := range commands {
		if commands[i].Name == "add-item" || commands[i].Name == "revise-item" {
			commands[i].Flags = append(commands[i].Flags, optional("documentation", "URN", "canonical Component, System or data-entity target"), optional("documentation-revision", "URN", "exact definition revision pin"),
				optional("documentation-view", "REV", "saved Saga commit admitting a non-current pin (inventory format 2)"), optional("selections", "FILE|-", "JSON array of explicit code selections (inventory format 2)"), optional("repo", "PATH", "source checkout for saved views and selections"))
		}
	}

	for _, kind := range []string{"component", "system", "data-entity", "erd", "erd-overlay"} {
		for _, op := range []string{"add", "revise", "set-state"} {
			name := kind + " " + op
			flags := []Flag{required("id", "ID", "stable identity"), optional("from", "FILE|-", "complete technical-definition JSON"), optional("revision", "ID", "revision identity; defaults to r1"), optional("event", "ID", "lifecycle event identity"), optional("state", "STATE", "active or retired"), optional("reason", "TEXT", "reason for lifecycle change"), repeatable("parent", "URN", "every observed revision or lifecycle head", false), optional("repo", "PATH", "source checkout"), jsonFlag}
			usage := "change-saga " + name + " --id ID [--from FILE|- --revision ID | --event ID --state STATE --reason TEXT] [--parent URN]"
			if kind == "erd" || kind == "erd-overlay" {
				flags = append(flags, optional("visual", "SVG", "offline SVG visual stored content-addressed with the revision"))
				usage += " [--visual SVG]"
			} else {
				flags = append(flags, optional("delivery", "REV", "delivery commit for an implemented revision; resolved once to a full OID"))
				usage += " [--delivery REV]"
			}
			commands = append(commands, Command{Name: name, Status: StatusImplemented, Mutates: true, Writes: []string{kind},
				Usage:   usage + " [--repo PATH] [--json] <saga>",
				Summary: "author a stable technical definition or append an explicit revision/lifecycle event",
				Flags:   flags, Positionals: sagaOnly})
		}
	}
	commands = append(commands, Command{Name: "inventory adopt-format", Status: StatusImplemented, Mutates: true, Writes: []string{"inventory-format"},
		Usage:   "change-saga inventory adopt-format --format 2 [--json] <saga>",
		Summary: "explicitly adopt inventory format 2 (intent, data entities, ERD, selections) without rewriting existing records",
		Flags:   []Flag{required("format", "N", "inventory format to adopt (2)"), jsonFlag}, Positionals: sagaOnly})
}
