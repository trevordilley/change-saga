package grammar

// Shared flag declarations. Every living mutation accepts an idempotency key
// and a machine-readable result.
var (
	requestIDFlag = Flag{Name: "request-id", Value: "ID", Description: "idempotency key; replaying the same request is a no-op"}
	jsonFlag      = Flag{Name: "json", Description: "emit a machine-readable result"}
	fromFileFlag  = Flag{Name: "from", Value: "FILE|-", Description: "read the complete structured request from a JSON file, or - for stdin"}
	sagaOnly      = []string{"<saga>"}
)

func required(name, value, description string) Flag {
	return Flag{Name: name, Value: value, Required: true, Description: description}
}

func optional(name, value, description string) Flag {
	return Flag{Name: name, Value: value, Description: description}
}

func repeatable(name, value, description string, isRequired bool) Flag {
	return Flag{Name: name, Value: value, Required: isRequired, Repeatable: true, Description: description}
}

// commands is the command-shape table. Usage strings of implemented commands
// are byte-identical to the CLI's own usage lines; a CLI test enforces that
// and that every declared flag is accepted by the real flag set.
var commands = []Command{
	{
		Name: "story add", Status: StatusImplemented, Mutates: true, Writes: []string{"story", "story-revision", "story-event"},
		Usage:   "change-saga story add --id ID --revision ID --event ID --title TEXT --statement TEXT --priority TEXT [flags] <saga>",
		Summary: "create a story identity, its initial complete revision, and its proposed lifecycle event",
		Flags: []Flag{
			required("id", "ID", "stable story id"), required("revision", "ID", "initial revision id"), required("event", "ID", "initial proposed-event id"),
			required("title", "TEXT", "story title"), required("statement", "TEXT", "complete user-story statement"), required("priority", "TEXT", "story priority"),
			repeatable("criterion", "ID=STATEMENT", "acceptance criterion", false), repeatable("citation", "URN", "citation URN", false),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story revise", Status: StatusImplemented, Mutates: true, Writes: []string{"story-revision"},
		Usage:   "change-saga story revise --story URN --revision ID --parent URN... --title TEXT --statement TEXT --priority TEXT [flags] <saga>",
		Summary: "append a complete story revision; several --parent values reconcile competing revision heads",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), required("revision", "ID", "new revision id"), repeatable("parent", "URN", "current revision head URN", true),
			required("title", "TEXT", "complete revised title"), required("statement", "TEXT", "complete revised statement"), required("priority", "TEXT", "complete revised priority"),
			repeatable("criterion", "ID=STATEMENT", "acceptance criterion", false), repeatable("citation", "URN", "citation URN", false),
			optional("edit", "", "edit the complete proposed revision with $EDITOR"), requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"story-event"},
		Usage:   "change-saga story set-state --story URN --event ID --parent URN... --state STATE [flags] <saga>",
		Summary: "append a story lifecycle event: proposed, accepted, deferred, rejected, or retired",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), required("event", "ID", "new lifecycle event id"), repeatable("parent", "URN", "current lifecycle head URN", true),
			required("state", "STATE", "proposed, accepted, deferred, rejected, or retired"), optional("reason", "TEXT", "why the lifecycle changed"),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "criterion add", Status: StatusImplemented, Mutates: true, Writes: []string{"story-revision"},
		Usage:   "change-saga criterion add --story URN --parent REVISION --revision ID --id ID --statement TEXT [flags] <saga>",
		Summary: "write a complete story revision that adds one acceptance criterion",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), required("parent", "REVISION", "the single current story revision head URN"), required("revision", "ID", "new story revision id"),
			required("id", "ID", "stable criterion id; removed ids can never be reused"), required("statement", "TEXT", "acceptance criterion statement"),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "criterion revise", Status: StatusImplemented, Mutates: true, Writes: []string{"story-revision"},
		Usage:   "change-saga criterion revise --story URN --criterion URN --parent REVISION --revision ID (--statement TEXT|--edit) [flags] <saga>",
		Summary: "write a complete story revision that rewords one criterion without changing its obligation; every relation pinned to the parent becomes stale",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), required("criterion", "URN", "canonical criterion URN"), required("parent", "REVISION", "the single current story revision head URN"),
			required("revision", "ID", "new story revision id"), required("statement", "TEXT", "new wording"), optional("edit", "", "edit the complete proposed revision with $EDITOR"),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "criterion remove", Status: StatusImplemented, Mutates: true, Writes: []string{"story-revision"},
		Usage:   "change-saga criterion remove --story URN --criterion URN --parent REVISION --revision ID --reason TEXT [flags] <saga>",
		Summary: "write a complete story revision that omits one criterion; its id is retired forever",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), required("criterion", "URN", "canonical criterion URN"), required("parent", "REVISION", "the single current story revision head URN"),
			required("revision", "ID", "new story revision id"), required("reason", "TEXT", "why the obligation is removed"),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "citation add", Status: StatusImplemented, Mutates: true, Writes: []string{"citation"},
		Usage:   "change-saga citation add --id ID --kind KIND --title TEXT --reference LOCATOR [flags] <saga>",
		Summary: "record an immutable citation that stories, exceptions, and evidence can cite",
		Flags: []Flag{
			required("id", "ID", "stable citation id"), required("kind", "KIND", "url, repository_commit, issue, document, or decision"),
			required("title", "TEXT", "citation title"), required("reference", "LOCATOR", "authoritative locator"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "relation add", Status: StatusImplemented, Mutates: true, Writes: []string{"relation"},
		Usage:   "change-saga relation add --id ID --type TYPE --from URN --to URN --rationale TEXT [flags] <saga>",
		Summary: "record one typed, pinned source -> target relation; see relation_matrix for legal endpoints and required pins",
		Flags: []Flag{
			required("id", "ID", "stable relation id"), required("type", "TYPE", "relation type from relation_matrix"),
			required("from", "URN", "source endpoint URN"), required("to", "URN", "target endpoint URN"), required("rationale", "TEXT", "why the endpoints are related"),
			optional("from-revision", "URN", "exact source definition revision pin"), optional("to-revision", "URN", "exact target story revision pin"),
			optional("from-content-digest", "DIGEST", "exact source design content digest pin"), optional("to-content-digest", "DIGEST", "exact target design content digest pin"),
			requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "relation supersede", Status: StatusImplemented, Mutates: true, Writes: []string{"relation"},
		Usage:       "change-saga relation supersede --relation URN [--request-id ID] [--json] <saga>",
		Summary:     "retire one active relation; history keeps the record",
		Flags:       []Flag{required("relation", "URN", "canonical relation URN"), requestIDFlag, jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "prototype add-html", Status: StatusImplemented, Mutates: true, Writes: []string{"prototype", "prototype-revision"},
		Usage:   "change-saga prototype add-html --id ID --revision ID --title TEXT --source PATH [--state STATE] [flags] <saga>",
		Summary: "record an interactive HTML prototype and its initial immutable revision",
		Flags: []Flag{
			required("id", "ID", "stable prototype id"), required("revision", "ID", "initial revision id"), required("title", "TEXT", "prototype title"),
			required("source", "PATH", "single .html file or directory containing index.html"), optional("state", "STATE", "draft, ready, or retired"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "prototype annotate", Status: StatusImplemented, Mutates: true, Writes: []string{"prototype-annotation"},
		Usage:   "change-saga prototype annotate --prototype URN --id ID --target URN --rationale TEXT --story-revision URN (--prototype-revision URN | --prototype-content-digest DIGEST) [selector] [flags] <saga>",
		Summary: "pin one prototype element, text, region, or provider node to a story or criterion revision",
		Flags: []Flag{
			required("prototype", "URN", "canonical prototype URN"), required("id", "ID", "stable annotation id"), required("target", "URN", "story or criterion URN"),
			required("rationale", "TEXT", "why this part of the prototype expresses the requirement"), required("story-revision", "URN", "exact story revision pin"),
			optional("prototype-revision", "URN", "exact prototype revision pin"), optional("prototype-content-digest", "DIGEST", "exact prototype content digest pin"),
			optional("element-id", "ID", "element selector"), optional("text", "TEXT", "exact text selector"), optional("region", "X,Y,W,H", "normalized region selector"),
			optional("provider-id", "ID", "provider-native node selector"), optional("deep-link", "URL", "provider deep link"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "add-deck", Status: StatusImplemented, Mutates: true, Writes: []string{"deck"},
		Usage:   "change-saga add-deck [flags] <saga> <name>",
		Summary: "create an embedded v4 deck; role ux places it on the UX design axis, other decks are implementation decks",
		Flags: []Flag{
			optional("id", "ID", "stable deck id"), optional("title", "TEXT", "deck title"), optional("role", "ROLE", "deck role; ux marks a UX flow deck"),
			optional("rank", "N", "review order"), optional("objective", "TEXT", "one concise reviewer objective"),
		},
		Positionals: []string{"<saga>", "<name>"},
	},
	{
		Name: "cover", Status: StatusImplemented, Mutates: true, Writes: []string{"diff-evidence"},
		Usage:   "change-saga cover [flags] [--batch FILE|-] [--dry-run] <saga>",
		Summary: "attach exact changed lines or file events to the smallest target that explains them",
		Flags: []Flag{
			optional("target", "TARGET", "section, fragment, landmark, or Item receiving the evidence"), optional("repo", "PATH", "source checkout when separate"),
			optional("path", "PATH", "changed repository path"), optional("side", "SIDE", "old or new"), optional("lines", "RANGES", "line ranges such as 4-9,12"),
			optional("changed-lines", "", "select every changed line and event for --path"), optional("event", "EVENT", "file event"),
			optional("old-path", "PATH", "old path of a rename"), optional("new-path", "PATH", "new path of a rename"), optional("note", "TEXT", "author note"),
			optional("name", "NAME", "coverage record filename"), repeatable("uri", "SAGA_DIFF_URI", "exact diff URI", false), optional("batch", "FILE|-", "batch request"),
			optional("dry-run", "", "resolve without writing"), jsonFlag, optional("quiet", "", "suppress successful output"),
			optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "rebase-evidence", Status: StatusImplemented, Mutates: true, Writes: []string{"diff-evidence"},
		Usage:   "change-saga rebase-evidence [--repo PATH] [--carry-verifications] [--dry-run] [--json|--quiet] <saga>",
		Summary: "re-pin exact diff evidence to the current source comparison when the change is provably equivalent",
		Flags: []Flag{
			optional("repo", "PATH", "source checkout when separate"), optional("from-base", "OID", "expected old base OID"),
			optional("carry-verifications", "", "carry equivalent verification results with an audit trail"), optional("dry-run", "", "prove and report without writing"),
			jsonFlag, optional("quiet", "", "suppress successful output"), optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "upgrade", Status: StatusImplemented, Mutates: true, Writes: []string{"saga"},
		Usage:       "change-saga upgrade --to 3 [--json] <saga>",
		Summary:     "atomically adopt a newer report-container version; never invents links, tests, or exceptions",
		Flags:       []Flag{required("to", "VERSION", "target Saga format version"), jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "validate", Status: StatusImplemented, Usage: "change-saga validate [--json] [--fix] <saga>",
		Summary:     "check every structural invariant without writing",
		Flags:       []Flag{jsonFlag, optional("fix", "", "add missing stable Markdown heading anchors")},
		Positionals: sagaOnly,
	},
	{
		Name: "status", Status: StatusImplemented, Usage: "change-saga status [--json] [--repo PATH] <saga>",
		Summary: "report changed-source accounting, gates, axis cells, stale pins, and ordered next actions",
		Flags: []Flag{
			jsonFlag, optional("repo", "PATH", "source checkout when separate"), optional("policy", "POLICY", "feature or compatibility readiness policy"),
			optional("max", "N", "maximum uncovered items in text mode"), optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "spec", Status: StatusImplemented, Usage: "change-saga spec [--json]",
		Summary: "publish resources, relation matrix, command grammar, and invariants",
		Flags:   []Flag{jsonFlag},
	},

	// Planned v5 writers. Their records are frozen by schema/v5; the writers are
	// landing separately, so these shapes are published as placeholders.
	{
		Name: "quality test-case add", Status: StatusPlanned, Mutates: true, Writes: []string{"test-case", "test-case-revision", "test-case-event"},
		Usage:   "change-saga quality test-case add --id ID --revision ID --event ID --from FILE|- [flags] <saga>",
		Summary: "create a test case with its initial complete revision (ordered steps, coverage kinds, automation) and proposed event",
		Flags: []Flag{
			required("id", "ID", "stable test-case id"), required("revision", "ID", "initial revision id"), required("event", "ID", "initial proposed-event id"),
			required("from", "FILE|-", "complete revision: title, coverage_kinds, automation, preconditions, steps, expected_result"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality test-case revise", Status: StatusPlanned, Mutates: true, Writes: []string{"test-case-revision"},
		Usage:   "change-saga quality test-case revise --test-case URN --parent URN... --revision ID --reason TEXT --from FILE|- [flags] <saga>",
		Summary: "append a complete test-case revision; runs and verifies relations pinned to the parent become stale",
		Flags: []Flag{
			required("test-case", "URN", "canonical test-case URN"), repeatable("parent", "URN", "current revision head URN", true), required("revision", "ID", "new revision id"),
			required("reason", "TEXT", "why the definition changed"), required("from", "FILE|-", "complete revision"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality test-case set-state", Status: StatusPlanned, Mutates: true, Writes: []string{"test-case-event"},
		Usage:   "change-saga quality test-case set-state --test-case URN --parent URN... --event ID --state STATE [flags] <saga>",
		Summary: "append a test-case lifecycle event: proposed, active, deprecated, or retired",
		Flags: []Flag{
			required("test-case", "URN", "canonical test-case URN"), repeatable("parent", "URN", "current lifecycle head URN", true), required("event", "ID", "new event id"),
			required("state", "STATE", "proposed, active, deprecated, or retired"), optional("reason", "TEXT", "why the lifecycle changed"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality policy set", Status: StatusPlanned, Mutates: true, Writes: []string{"quality-policy"},
		Usage:   "change-saga quality policy set --id ID --criterion URN --story-revision URN --require KIND... --rationale TEXT [flags] <saga>",
		Summary: "record the test kinds one criterion revision requires; positive is required by default",
		Flags: []Flag{
			required("id", "ID", "stable policy id"), required("criterion", "URN", "canonical criterion URN"), required("story-revision", "URN", "exact story revision pin"),
			repeatable("require", "KIND", "positive, negative, or edge", true), repeatable("allow-automation", "AUTOMATION", "manual, automated, or hybrid", false),
			repeatable("supersedes", "URN", "policy this one replaces", false), required("rationale", "TEXT", "why these kinds are required"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality evidence add", Status: StatusPlanned, Mutates: true, Writes: []string{"quality-evidence"},
		Usage:   "change-saga quality evidence add --test-case URN --test-revision URN --id ID --role ROLE (--diff URI|--verification URN|--citation URN)... [flags] <saga>",
		Summary: "record immutable typed evidence for one test revision: test code diffs, implementation-under-test diffs, or execution artifacts",
		Flags: []Flag{
			required("test-case", "URN", "canonical test-case URN"), required("test-revision", "URN", "exact test revision pin"), required("id", "ID", "stable evidence id"),
			required("role", "ROLE", "test_implementation, implementation_under_test, or execution_artifact"), repeatable("diff", "SAGA_DIFF_URI", "exact diff URI", false),
			repeatable("verification", "URN", "verification URN", false), repeatable("citation", "URN", "citation URN", false),
			repeatable("supersedes", "URN", "evidence this record replaces", false), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality run record", Status: StatusPlanned, Mutates: true, Writes: []string{"test-run"},
		Usage:   "change-saga quality run record --test-case URN --test-revision URN --id ID --result RESULT --summary TEXT --evidence URN... [flags] <saga>",
		Summary: "append an immutable run result pinned to the test revision and the current source comparison",
		Flags: []Flag{
			required("test-case", "URN", "canonical test-case URN"), required("test-revision", "URN", "exact test revision pin"), required("id", "ID", "stable run id"),
			repeatable("parent", "URN", "current run head URN", false), required("result", "RESULT", "passed, failed, blocked, or skipped"),
			required("summary", "TEXT", "what was observed"), repeatable("evidence", "URN", "current evidence URN", true), optional("command", "COMMAND", "command that produced the result"),
			requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "coverage-exception add", Status: StatusPlanned, Mutates: true, Writes: []string{"coverage-exception"},
		Usage:   "change-saga coverage-exception add --id ID --axis AXIS --criterion URN --story-revision URN --rationale TEXT --citation URN... [flags] <saga>",
		Summary: "record an explicit, cited decision that one axis does not apply to one criterion revision; never excuses changed-source accounting",
		Flags: []Flag{
			required("id", "ID", "stable exception id"), required("axis", "AXIS", "prototype, ux, ui, technical, quality, or implementation"),
			required("criterion", "URN", "canonical criterion URN"), required("story-revision", "URN", "exact current story revision pin"),
			required("rationale", "TEXT", "why the axis does not apply"), repeatable("citation", "URN", "citation supporting the decision", true), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "coverage-exception supersede", Status: StatusPlanned, Mutates: true, Writes: []string{"coverage-exception"},
		Usage:       "change-saga coverage-exception supersede --exception URN --with URN [flags] <saga>",
		Summary:     "replace one exception decision with another; competing heads are never resolved by timestamp",
		Flags:       []Flag{required("exception", "URN", "exception being replaced"), required("with", "URN", "replacing exception"), requestIDFlag, jsonFlag},
		Positionals: sagaOnly,
	},
}
