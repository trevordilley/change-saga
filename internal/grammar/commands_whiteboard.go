package grammar

// Whiteboards use the shared diagram source but own complete document revisions.
var whiteboardCommands = []Command{
	{
		Name: "whiteboard publish", Status: StatusImplemented, Mutates: true,
		Writes:  []string{"whiteboard-revision"},
		Usage:   "change-saga whiteboard publish --id ID --revision ID [--parent ID] [--name TITLE] --from FILE|- <saga>",
		Summary: "publish a complete immutable technical-design whiteboard with exact source pins; updates require the observed current parent",
		Flags: []Flag{
			required("id", "ID", "whiteboard document identity"), required("revision", "ID", "new immutable board revision"),
			optional("parent", "ID", "current board revision, required for updates"), optional("name", "TITLE", "authored board title"),
			required("from", "FILE|-", "complete shared diagram.Document JSON"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "whiteboard get", Status: StatusImplemented,
		Usage:   "change-saga whiteboard get --id ID [--revision ID] [--focus PATH] <saga>",
		Summary: "return complete whiteboard diagram source or describe one focused appearance with source pins and connection context",
		Flags: []Flag{
			required("id", "ID", "whiteboard document identity"), optional("revision", "ID", "exact historical board revision; defaults to current"),
			optional("focus", "PATH", "element instance path"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "whiteboard describe", Status: StatusImplemented,
		Usage:   "change-saga whiteboard describe --id ID [--revision ID] [--focus PATH] [--format text|json] <saga>",
		Summary: "describe a whiteboard or focused appearance with transformed geometry, source pins and connection context",
		Flags: []Flag{
			required("id", "ID", "whiteboard document identity"), optional("revision", "ID", "exact historical board revision; defaults to current"),
			optional("focus", "PATH", "element instance path"), optional("format", "text|json", "output format; defaults to text"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "whiteboard check", Status: StatusImplemented,
		Usage:   "change-saga whiteboard check --id ID [--revision ID] [--focus PATH] [--json] <saga>",
		Summary: "validate complete whiteboard history and resolve the current composition, or check one exact revision",
		Flags: []Flag{
			required("id", "ID", "whiteboard document identity"), optional("revision", "ID", "exact board revision; otherwise validate all history"),
			optional("focus", "PATH", "also verify this instance path resolves"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
}

func init() { commands = append(commands, whiteboardCommands...) }
