package grammar

func init() {
	commands = append(commands,
		Command{
			Name: "move-deck", Status: StatusImplemented, Mutates: true, Writes: []string{"deck"},
			Usage:   "change-saga move-deck --deck TARGET --feature ID [--dry-run] [--json] <saga>",
			Summary: "move an implementation deck and its complete history to another feature, preserving stable identities",
			Flags: []Flag{
				required("deck", "TARGET", "implementation deck path, id, or URN"),
				required("feature", "ID", "destination feature id or URN"),
				{Name: "dry-run", Description: "report the move without writing"}, jsonFlag,
			},
			Positionals: sagaOnly,
		},
		Command{
			Name: "move-slide", Status: StatusImplemented, Mutates: true, Writes: []string{"slide", "item", "code-evidence"},
			Usage:   "change-saga move-slide --slide TARGET --deck TARGET [--dry-run] [--json] <saga>",
			Summary: "move an ordinary implementation slide with its Items and evidence; immutable transaction slides and review ownership changes are refused",
			Flags: []Flag{
				required("slide", "TARGET", "slide path, id, or URN"),
				required("deck", "TARGET", "destination implementation deck"),
				{Name: "dry-run", Description: "report the move without writing"}, jsonFlag,
			},
			Positionals: sagaOnly,
		},
	)
	for _, kind := range []string{"chapter", "section", "fragment"} {
		commands = append(commands, Command{
			Name: "move-" + kind, Status: StatusImplemented, Mutates: true, Writes: []string{kind},
			Usage:   "change-saga move-" + kind + " --target TARGET --parent TARGET [--name NAME] [--dry-run] [--json] <saga>",
			Summary: "move or rename a " + kind + " directory with its descendants and evidence, preserving stable identities",
			Flags: []Flag{
				required("target", "TARGET", kind+" path, id, or URN"),
				required("parent", "TARGET", "destination feature id/URN, designsystem, feature ___design path, chapter, or section; chapters require a report root and sections require a chapter or section"),
				optional("name", "NAME", "new directory basename without a type suffix"),
				{Name: "dry-run", Description: "report the move without writing"}, jsonFlag,
			},
			Positionals: sagaOnly,
		})
	}
}
