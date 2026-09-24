package grammar

func init() {
	for i := range commands {
		if commands[i].Name == "add-item" || commands[i].Name == "revise-item" {
			commands[i].Flags = append(commands[i].Flags, optional("documentation", "URN", "canonical Component/System target"), optional("documentation-revision", "URN", "exact definition revision pin"))
		}
	}

	for _, kind := range []string{"component", "system"} {
		for _, op := range []string{"add", "revise", "set-state"} {
			name := kind + " " + op
			commands = append(commands, Command{Name: name, Status: StatusImplemented, Mutates: true, Writes: []string{kind},
				Usage:   "change-saga " + name + " --id ID [--from FILE|- --revision ID | --event ID --state STATE --reason TEXT] [--parent URN] [--repo PATH] [--json] <saga>",
				Summary: "author a stable technical definition or append an explicit revision/lifecycle event",
				Flags:   []Flag{required("id", "ID", "stable identity"), optional("from", "FILE|-", "complete technical-definition JSON"), optional("revision", "ID", "revision identity; defaults to r1"), optional("event", "ID", "lifecycle event identity"), optional("state", "STATE", "active or retired"), optional("reason", "TEXT", "reason for lifecycle change"), repeatable("parent", "URN", "every observed revision or lifecycle head", false), optional("repo", "PATH", "source checkout"), jsonFlag}, Positionals: sagaOnly})
		}
	}
}
