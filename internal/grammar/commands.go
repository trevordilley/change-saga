package grammar

// Shared flag declarations. Every living mutation accepts an idempotency key
// and a machine-readable result.
var (
	requestIDFlag = Flag{Name: "request-id", Value: "ID", Description: "idempotency key; replaying the same request is a no-op"}
	jsonFlag      = Flag{Name: "json", Description: "emit a machine-readable result"}
	fromFileFlag  = Flag{Name: "from", Value: "FILE|-", Description: "read the complete structured request from a JSON file, or - for stdin"}
	sagaOnly      = []string{"<saga>"}
	// epicFlag is required by commands that create epic content and accepted by
	// commands that change an existing record, where it must name the epic that
	// already holds the record.
	epicFlag       = Flag{Name: "epic", Value: "ID", Description: "epic id or URN that holds the record"}
	epicCreateFlag = Flag{Name: "epic", Value: "ID", Required: true, Description: "epic id or URN the new content belongs to"}
	// againstFlag and headFlag choose how a Saga is opened. The comparison is
	// never stored: without --against a command observes --head.
	againstFlag = Flag{Name: "against", Value: "REV", Description: "compare --head with its merge-base with this revision; omit to observe --head"}
	headFlag    = Flag{Name: "head", Value: "REV", Description: "the commit to observe, or the head of the comparison; defaults to HEAD"}
)

// reviewDecisionFlags declare the reviewer seat of a review decision or
// comment and where its commits live.
var reviewDecisionFlags = []Flag{
	required("reviewer-kind", "KIND", "human for your own decision, ai for an AI reviewer's"),
	optional("reviewer-name", "NAME", "a distinct AI reviewer seat (AI only)"), optional("agent", "AGENT", "AI agent kind (AI only)"),
	optional("model", "MODEL", "exact AI model name (AI only)"), optional("repo", "PATH", "code checkout when separate"), jsonFlag,
}

// termDefinitionFlags declare the complete content of a term revision.
func termDefinitionFlags() []Flag {
	return []Flag{
		required("name", "TEXT", "the term as the team says it"), required("definition", "TEXT", "what the term means in this project"),
		repeatable("alias", "TEXT", "another spelling the team uses", false), repeatable("story", "URN", "story URN or ID the term belongs to", false),
		repeatable("record", "URN", "persona, epic, flag, or term URN the term names", false),
		repeatable("ref", "LOCATION", "code that defines the term, <commit>:<path>#L<start>[-L<end>]; the commit may be any revision", false),
		optional("repo", "PATH", "code checkout when separate"),
	}
}

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
		Name: "epic add", Status: StatusImplemented, Mutates: true, Writes: []string{"epic"},
		Usage:   "change-saga epic add --id ID --title TEXT [--description TEXT] [flags] <saga>",
		Summary: "add a durable product domain; it holds its own stories, design, quality, work plan, and implementation deck",
		Flags: []Flag{
			required("id", "ID", "stable epic id"), required("title", "TEXT", "the product domain the epic covers"),
			optional("description", "TEXT", "what the domain covers"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "overview set-pitch", Status: StatusImplemented, Mutates: true, Writes: []string{"overview-pitch"},
		Usage:   "change-saga overview set-pitch (--text TEXT | --source FILE|-) [--json|--quiet] <saga>",
		Summary: "write the overview's elevator pitch as Markdown; until then it is shown as a gap",
		Flags: []Flag{
			optional("text", "TEXT", "the Markdown content; or use --source"), optional("source", "FILE|-", "a Markdown file, or - for standard input"),
			jsonFlag, {Name: "quiet", Description: "suppress successful output"},
		},
		Positionals: sagaOnly,
	},
	{
		Name: "overview set-description", Status: StatusImplemented, Mutates: true, Writes: []string{"overview-description"},
		Usage:   "change-saga overview set-description (--text TEXT | --source FILE|-) [--json|--quiet] <saga>",
		Summary: "write the overview's description, a short essay, as Markdown; until then it is shown as a gap",
		Flags: []Flag{
			optional("text", "TEXT", "the Markdown content; or use --source"), optional("source", "FILE|-", "a Markdown file, or - for standard input"),
			jsonFlag, {Name: "quiet", Description: "suppress successful output"},
		},
		Positionals: sagaOnly,
	},
	{
		Name: "term add", Status: StatusImplemented, Mutates: true, Writes: []string{"term", "term-revision", "term-event"},
		Usage:   "change-saga term add --id ID --name TEXT --definition TEXT [--alias TEXT...] [--story URN...] [--record URN...] [--ref LOCATION...] [flags] <saga>",
		Summary: "define a term in the project's vocabulary and reference the stories and code it names; a rename of that code makes the reference stale",
		Flags: append([]Flag{required("id", "ID", "stable term id")}, append(termDefinitionFlags(),
			optional("revision", "ID", "initial revision id; defaults to r1"), optional("event", "ID", "initial active-event id; defaults to active"), requestIDFlag, jsonFlag)...),
		Positionals: sagaOnly,
	},
	{
		Name: "term revise", Status: StatusImplemented, Mutates: true, Writes: []string{"term-revision"},
		Usage:   "change-saga term revise --term URN --revision ID --parent URN... --name TEXT --definition TEXT [--alias TEXT...] [--story URN...] [--record URN...] [--ref LOCATION...] [flags] <saga>",
		Summary: "append a complete term revision, such as pointing a stale code reference at renamed code; several --parent values reconcile competing heads",
		Flags: append([]Flag{required("term", "URN", "canonical term URN"), required("revision", "ID", "new revision id"), repeatable("parent", "URN", "current revision head URN", true)},
			append(termDefinitionFlags(), requestIDFlag, jsonFlag)...),
		Positionals: sagaOnly,
	},
	{
		Name: "term set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"term-event"},
		Usage:   "change-saga term set-state --term URN --event ID --parent URN... --state active|retired [--reason TEXT] [flags] <saga>",
		Summary: "retire a term the project no longer uses, or restore it",
		Flags: []Flag{
			required("term", "URN", "canonical term URN"), required("event", "ID", "new lifecycle event id"), repeatable("parent", "URN", "current lifecycle head URN", true),
			required("state", "STATE", "active or retired"), optional("reason", "TEXT", "why the lifecycle changed"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "persona add", Status: StatusImplemented, Mutates: true, Writes: []string{"persona", "persona-revision", "persona-event"},
		Usage:   "change-saga persona add --id ID --name TEXT --description TEXT [--revision r1] [--event active] [flags] <saga>",
		Summary: "add an active persona: someone who gets value from the app, not a tool or agent that operates it; until an accepted story serves it, it is a visible gap",
		Flags: []Flag{
			required("id", "ID", "stable persona id"), required("name", "TEXT", "persona name"), required("description", "TEXT", "who this person is and what value they get from the app"),
			optional("revision", "ID", "initial revision id; defaults to r1"), optional("event", "ID", "initial active-event id; defaults to active"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "persona revise", Status: StatusImplemented, Mutates: true, Writes: []string{"persona-revision"},
		Usage:   "change-saga persona revise --persona URN --revision ID --parent URN... --name TEXT --description TEXT [flags] <saga>",
		Summary: "append a complete persona revision; several --parent values reconcile competing heads",
		Flags: []Flag{
			required("persona", "URN", "canonical persona URN"), required("revision", "ID", "new revision id"), repeatable("parent", "URN", "current revision head URN", true),
			required("name", "TEXT", "complete revised name"), required("description", "TEXT", "complete revised description; keep it a person who gets value from the app"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "persona set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"persona-event"},
		Usage:   "change-saga persona set-state --persona URN --event ID --parent URN... --state active|retired [--reason TEXT] [flags] <saga>",
		Summary: "retire or restore a persona; stories that served only a retired persona become one question: retire them or reassign them",
		Flags: []Flag{
			required("persona", "URN", "canonical persona URN"), required("event", "ID", "new lifecycle event id"), repeatable("parent", "URN", "current lifecycle head URN", true),
			required("state", "STATE", "active or retired"), optional("reason", "TEXT", "why the lifecycle changed"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "flag add", Status: StatusImplemented, Mutates: true, Writes: []string{"flag", "flag-revision", "flag-event"},
		Usage:   "change-saga flag add --id ID --description TEXT --target URN... [--state off|on] [flags] <saga>",
		Summary: "add a feature flag gating stories or whole epics; a gated story can be implemented but not enabled",
		Flags: []Flag{
			required("id", "ID", "stable flag id"), required("description", "TEXT", "what the flag gates and why"),
			repeatable("target", "URN", "story or epic URN the flag gates", true), optional("state", "STATE", "off (default) or on"),
			optional("revision", "ID", "initial revision id; defaults to r1"), optional("event", "ID", "initial event id; defaults to the state"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "flag revise", Status: StatusImplemented, Mutates: true, Writes: []string{"flag-revision"},
		Usage:   "change-saga flag revise --flag URN --revision ID --parent URN... --description TEXT --target URN... [flags] <saga>",
		Summary: "append a complete flag revision: its description and every story or epic it gates",
		Flags: []Flag{
			required("flag", "URN", "canonical flag URN"), required("revision", "ID", "new revision id"), repeatable("parent", "URN", "current revision head URN", true),
			required("description", "TEXT", "complete revised description"), repeatable("target", "URN", "story or epic URN the flag gates", true), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "flag set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"flag-event"},
		Usage:   "change-saga flag set-state --flag URN --event ID --parent URN... --state off|on|retired [--reason TEXT] [flags] <saga>",
		Summary: "turn a flag on or off, or retire it; a retired flag gates nothing",
		Flags: []Flag{
			required("flag", "URN", "canonical flag URN"), required("event", "ID", "new lifecycle event id"), repeatable("parent", "URN", "current lifecycle head URN", true),
			required("state", "STATE", "off, on, or retired"), optional("reason", "TEXT", "why the flag changed"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story move", Status: StatusImplemented, Mutates: true, Writes: []string{"story"},
		Usage:   "change-saga story move --story URN --epic ID [--json] <saga>",
		Summary: "move a story to another epic; its URN, revisions, and every relation and pin to it are unchanged",
		Flags: []Flag{
			required("story", "URN", "canonical story URN"), epicCreateFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story add", Status: StatusImplemented, Mutates: true, Writes: []string{"story", "story-revision", "story-event"},
		Usage:   "change-saga story add --epic ID --id ID --revision ID --event ID --title TEXT --statement TEXT [--priority TEXT] [flags] <saga>",
		Summary: "create a story identity, its initial complete revision, and its proposed lifecycle event",
		Flags: []Flag{epicCreateFlag, repeatable("persona", "URN", "persona URN the story serves: the \"As a ...\" of its statement; optional", false),
			required("id", "ID", "stable story id"), required("revision", "ID", "initial revision id"), required("event", "ID", "initial proposed-event id"),
			required("title", "TEXT", "story title"), required("statement", "TEXT", "complete user-story statement"), optional("priority", "TEXT", "optional free-text priority the reviewer shows, such as must, should, or could; the tool reads no meaning into it"),
			repeatable("criterion", "ID=STATEMENT", "acceptance criterion", false), repeatable("citation", "URN", "citation URN", false),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story revise", Status: StatusImplemented, Mutates: true, Writes: []string{"story-revision"},
		Usage:   "change-saga story revise --story URN --revision ID --parent URN... [--title TEXT] [--statement TEXT] [--priority TEXT] [flags] <saga>",
		Summary: "append a complete story revision; one --parent inherits every field you leave out, several reconcile competing revision heads",
		Flags: []Flag{repeatable("persona", "URN", "persona URN the revised story serves; the parent's personas are kept when omitted", false), epicFlag,
			required("story", "URN", "canonical story URN"), required("revision", "ID", "new revision id"), repeatable("parent", "URN", "current revision head URN", true),
			optional("title", "TEXT", "complete revised title; inherited from a single parent when omitted"), optional("statement", "TEXT", "complete revised statement; inherited from a single parent when omitted"), optional("priority", "TEXT", "optional free-text priority; inherited from a single parent when omitted"),
			repeatable("criterion", "ID=STATEMENT", "acceptance criterion; the parent's criteria are kept when omitted", false), repeatable("citation", "URN", "citation URN; the parent's citations are kept when omitted", false),
			optional("edit", "", "edit the complete proposed revision with $EDITOR"), requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "story set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"story-event"},
		Usage:   "change-saga story set-state --story URN --event ID --parent URN... --state STATE [flags] <saga>",
		Summary: "append a story lifecycle event: proposed, accepted, deferred, rejected, or retired",
		Flags: []Flag{epicFlag,
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
		Flags: []Flag{epicFlag,
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
		Flags: []Flag{epicFlag,
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
		Flags: []Flag{epicFlag,
			required("story", "URN", "canonical story URN"), required("criterion", "URN", "canonical criterion URN"), required("parent", "REVISION", "the single current story revision head URN"),
			required("revision", "ID", "new story revision id"), required("reason", "TEXT", "why the obligation is removed"),
			requestIDFlag, fromFileFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "citation add", Status: StatusImplemented, Mutates: true, Writes: []string{"citation"},
		Usage:   "change-saga citation add --epic ID --id ID --kind KIND --title TEXT --reference LOCATOR [flags] <saga>",
		Summary: "record an immutable citation that stories, exceptions, and evidence can cite; a story in any epic may cite it",
		Flags: []Flag{{Name: "epic", Value: "ID", Required: true, Description: "epic that stores the citation; its URN names no epic, so a story in any epic may cite it"},
			required("id", "ID", "stable citation id"), required("kind", "KIND", "url, repository_commit, issue, document, or decision"),
			required("title", "TEXT", "citation title"), required("reference", "LOCATOR", "authoritative locator"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "relation add", Status: StatusImplemented, Mutates: true, Writes: []string{"relation"},
		Usage:   "change-saga relation add --epic ID --id ID --type TYPE --from URN --to URN --rationale TEXT [--scope self|descendants] [flags] <saga>",
		Summary: "record one typed, pinned source -> target relation; see relation_matrix for legal endpoints and required pins; omitted revision pins default to the unique current head",
		Flags: []Flag{epicCreateFlag,
			required("id", "ID", "stable relation id"), required("type", "TYPE", "relation type from relation_matrix"),
			required("from", "URN", "source endpoint URN"), required("to", "URN", "target endpoint URN"), required("rationale", "TEXT", "why the endpoints are related"),
			optional("scope", "SCOPE", "v5 only: self (default) or descendants for a deck or slide source"),
			optional("from-revision", "URN", "exact source definition revision pin"), optional("to-revision", "URN", "exact target story revision pin"),
			optional("from-content-digest", "DIGEST", "exact source design content digest pin"), optional("to-content-digest", "DIGEST", "exact target design content digest pin"),
			requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "relation supersede", Status: StatusImplemented, Mutates: true, Writes: []string{"relation"},
		Usage:       "change-saga relation supersede --relation URN [--epic ID] [--request-id ID] [--json] <saga>",
		Summary:     "retire one active relation; history keeps the record",
		Flags:       []Flag{epicFlag, required("relation", "URN", "canonical relation URN"), requestIDFlag, jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "relation status", Status: StatusImplemented, Usage: "change-saga relation status [--relation URN] [--json] <saga>",
		Summary:     "report each relation as current, stale, conflicted, invalid, or superseded from its pins against current heads",
		Flags:       []Flag{optional("relation", "URN", "report one relation"), jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "prototype add-html", Status: StatusImplemented, Mutates: true, Writes: []string{"prototype", "prototype-revision"},
		Usage:   "change-saga prototype add-html --epic ID --id ID --revision ID --title TEXT --source PATH [--state STATE] [flags] <saga>",
		Summary: "record an interactive HTML prototype and its initial immutable revision",
		Flags: []Flag{epicCreateFlag,
			required("id", "ID", "stable prototype id"), required("revision", "ID", "initial revision id"), required("title", "TEXT", "prototype title"),
			required("source", "PATH", "single .html file or directory containing index.html"), optional("state", "STATE", "draft, ready, or retired"), requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "prototype annotate", Status: StatusImplemented, Mutates: true, Writes: []string{"prototype-annotation"},
		Usage:   "change-saga prototype annotate --prototype URN --id ID --target URN --rationale TEXT --story-revision URN (--prototype-revision URN | --prototype-content-digest DIGEST) [selector] [flags] <saga>",
		Summary: "pin one prototype element, text, region, or provider node to a story or criterion revision",
		Flags: []Flag{epicFlag,
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
		Usage:   "change-saga add-deck (--epic ID | --role onboarding) [flags] <saga> <name>",
		Summary: "create an epic's implementation deck, or with --role onboarding the app's onboarding deck whose Items reference records",
		Flags: []Flag{optional("epic", "ID", "epic whose implementation deck this is; required unless --role onboarding"),
			optional("id", "ID", "stable deck id"), optional("title", "TEXT", "deck title"), optional("role", "ROLE", "change (an epic's implementation deck, the default) or onboarding (the app's onboarding deck)"),
			optional("rank", "N", "review order"), optional("objective", "TEXT", "one concise reviewer objective"),
		},
		Positionals: []string{"<saga>", "<name>"},
	},
	{
		Name: "revise-deck", Status: StatusImplemented, Mutates: true, Writes: []string{"deck"},
		Usage:   "change-saga revise-deck --deck TARGET [--title TEXT] [--objective TEXT] [--rank N] [--dry-run] [--json] <saga>",
		Summary: "correct a deck's title, objective, or rank in place. Its identity and slides are unchanged. Repeating a revise is a no-op reported as replayed",
		Flags: []Flag{required("deck", "TARGET", "deck path, id, or URN"), optional("title", "TEXT", "deck title"), optional("objective", "TEXT", "one concise reviewer objective"), optional("rank", "N", "non-negative deck order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-deck", Status: StatusImplemented, Mutates: true, Writes: []string{"deck", "slide", "item", "code-evidence"},
		Usage:   "change-saga remove-deck --deck TARGET [--dry-run] [--json] <saga>",
		Summary: "delete a deck with its slides, Items, and their code evidence. Relations that pointed at them are listed; status reports them stale until superseded",
		Flags: []Flag{required("deck", "TARGET", "deck path, id, or URN"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "revise-slide", Status: StatusImplemented, Mutates: true, Writes: []string{"slide"},
		Usage:   "change-saga revise-slide [--review ID] --slide TARGET [--title TEXT] [--section TEXT] [--intent INTENT] [--layout LAYOUT] [--takeaway TEXT] [--exception-rationale TEXT] [--rank N] [--dry-run] [--json] <saga>",
		Summary: "correct a slide's title, section, intent, layout, takeaway, or rank in place; set-slide-content replaces its visual. Its identity and Items are unchanged",
		Flags: []Flag{optional("review", "ID", "the pull request review whose slide this is"), required("slide", "TARGET", "slide path, id, or URN"), optional("title", "TEXT", "slide title"), optional("section", "TEXT", "section label; empty clears it"), optional("intent", "INTENT", "orient, explain, compare, trace, prove, risk, or conclude"), optional("layout", "LAYOUT", "hero, diagram, before-after, sequence, evidence, risk, or custom"), optional("takeaway", "TEXT", "single reviewer takeaway"), optional("exception-rationale", "TEXT", "reason for a custom layout"), optional("rank", "N", "non-negative review order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-slide", Status: StatusImplemented, Mutates: true, Writes: []string{"slide", "item", "code-evidence"},
		Usage:   "change-saga remove-slide --slide TARGET [--dry-run] [--json] <saga>",
		Summary: "delete a slide with its Items and their code evidence. A review's slides carry its approvals and comments, so they are not removed",
		Flags: []Flag{required("slide", "TARGET", "slide path, id, or URN"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "revise-item", Status: StatusImplemented, Mutates: true, Writes: []string{"item"},
		Usage:   "change-saga revise-item [--review ID] --item URN|ID [--slide TARGET] [--kind KIND] [--label TEXT] [--description TEXT] [selector] [callout flags] [--record URN] [--rank N] [--dry-run] [--json] <saga>",
		Summary: "correct an Item's label, description, kind, selector, callout fields, record, or rank in place. Its identity and code evidence are unchanged",
		Flags: []Flag{optional("review", "ID", "the pull request review whose slide holds the Item"), required("item", "URN|ID", "Item URN, or its id with --slide"), optional("slide", "TARGET", "slide holding the Item when --item is an id"), optional("kind", "KIND", "node, edge, region, transition, statement, risk, metric, example, or callout"), optional("label", "TEXT", "concise reviewer-facing label"), optional("description", "TEXT", "non-visual semantic description"), optional("element-id", "ID", "element selector"), optional("region", "X,Y,W,H", "normalized image region selector"), optional("hotspot", "X,Y,W,H", "on-canvas hit area; empty clears it"), optional("about", "ID", "for callouts, another item on this slide"), optional("body", "TEXT", "callout body"), optional("placement", "PLACEMENT", "top, right, bottom, left, or overlay"), optional("leader", "LEADER", "none, line, or arrow"), optional("record", "URN", "the record the Item points at"), optional("rank", "N", "non-negative item order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-item", Status: StatusImplemented, Mutates: true, Writes: []string{"item", "code-evidence", "slide"},
		Usage:   "change-saga remove-item --item URN|ID [--slide TARGET] [--dry-run] [--json] <saga>",
		Summary: "delete an Item and its code evidence, and drop it from its slide's reading order. A callout about the Item must be revised or removed first",
		Flags: []Flag{required("item", "URN|ID", "Item URN, or its id with --slide"), optional("slide", "TARGET", "slide holding the Item when --item is an id"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "revise-chapter", Status: StatusImplemented, Mutates: true, Writes: []string{"chapter"},
		Usage:   "change-saga revise-chapter --target TARGET [--title TEXT] [--order N] [--dry-run] [--json] <saga>",
		Summary: "correct a chapter's title or display order in place. Its identity and content are unchanged",
		Flags: []Flag{required("target", "TARGET", "chapter path, id, or URN"), optional("title", "TEXT", "chapter title"), optional("order", "N", "display order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-chapter", Status: StatusImplemented, Mutates: true, Writes: []string{"chapter", "section", "fragment", "code-evidence"},
		Usage:   "change-saga remove-chapter --target TARGET [--dry-run] [--json] <saga>",
		Summary: "delete a chapter with its sections and fragments. Relations that pointed at them are listed; status reports them stale until superseded",
		Flags: []Flag{required("target", "TARGET", "chapter path, id, or URN"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "revise-section", Status: StatusImplemented, Mutates: true, Writes: []string{"section"},
		Usage:   "change-saga revise-section --target TARGET [--title TEXT] [--order N] [--dry-run] [--json] <saga>",
		Summary: "correct a section's title or display order in place. Its identity and content are unchanged",
		Flags: []Flag{required("target", "TARGET", "section path, id, or URN"), optional("title", "TEXT", "section title"), optional("order", "N", "display order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-section", Status: StatusImplemented, Mutates: true, Writes: []string{"section", "fragment", "code-evidence"},
		Usage:   "change-saga remove-section --target TARGET [--dry-run] [--json] <saga>",
		Summary: "delete a section with its sections and fragments. Relations that pointed at them are listed; status reports them stale until superseded",
		Flags: []Flag{required("target", "TARGET", "section path, id, or URN"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "revise-fragment", Status: StatusImplemented, Mutates: true, Writes: []string{"fragment"},
		Usage:   "change-saga revise-fragment --target TARGET [--title TEXT] [--order N] [--dry-run] [--json] <saga>",
		Summary: "correct a fragment's title or display order in place. Its identity and content are unchanged",
		Flags: []Flag{required("target", "TARGET", "fragment path, id, or URN"), optional("title", "TEXT", "fragment title"), optional("order", "N", "display order"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "remove-fragment", Status: StatusImplemented, Mutates: true, Writes: []string{"fragment", "code-evidence"},
		Usage:   "change-saga remove-fragment --target TARGET [--dry-run] [--json] <saga>",
		Summary: "delete a fragment with its landmarks and code evidence. Relations that pointed at them are listed; status reports them stale until superseded",
		Flags: []Flag{required("target", "TARGET", "fragment path, id, or URN"),
			optional("dry-run", "", "report what would change without writing"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "cover", Status: StatusImplemented, Mutates: true, Writes: []string{"code-evidence"},
		Usage:   "change-saga cover [flags] [--batch FILE|-] [--dry-run] [--against REV [--head REV]] <saga>",
		Summary: "reference the code a target explains, pinned at a commit, from the smallest target that explains it; a review Item compares its review's own range unless --against is given",
		Flags: []Flag{
			optional("target", "TARGET", "section, fragment, landmark, or Item receiving the evidence"), optional("repo", "PATH", "source checkout when separate"),
			optional("path", "PATH", "repository path"), optional("side", "SIDE", "new (head) or old (merge-base)"), optional("lines", "RANGES", "line ranges such as 4-9,12"),
			optional("changed-lines", "", "reference every changed line of --path and whole files for file events"), optional("file", "", "reference the whole file at --path"),
			optional("commit", "REV", "pin at this revision instead of a comparison side"), optional("note", "TEXT", "author note"),
			optional("name", "NAME", "coverage record filename"), repeatable("ref", "LOCATION", "code location <commit>:<path>[#L<start>[-L<end>]]; the commit may be any revision", false), optional("batch", "FILE|-", "batch request"),
			optional("dry-run", "", "resolve without writing"), jsonFlag, optional("quiet", "", "suppress successful output"),
			optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"), againstFlag, headFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "references", Status: StatusImplemented, Usage: "change-saga references [--stale] [--diff] [--json] [--repo PATH] [--against REV [--head REV]] <saga>",
		Summary: "list every code reference viewed in the comparison: current, remapped, or stale with its reason and diff since the pin",
		Flags: []Flag{
			optional("stale", "", "list only stale references"), optional("diff", "", "include each stale reference's diff since its pin"),
			jsonFlag, optional("repo", "PATH", "source checkout when separate"), optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
			againstFlag, headFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "repin", Status: StatusImplemented, Mutates: true, Writes: []string{"code-evidence", "merge", "sync-cursor", "review"},
		Usage:   "change-saga repin --onto REV [--branch REV] [--review ID] [--dry-run] [--json] [--repo PATH] <saga>",
		Summary: "after a change lands, re-pin evidence references to the landed commit, record the branch's commit messages, and freeze the change's review at its exact base and head",
		Flags: []Flag{
			required("onto", "REV", "the commit the change landed as"), optional("branch", "REV", "the branch's last commit when still available"),
			optional("review", "ID", "the landed change's review; defaults to the Saga's only open review"),
			optional("dry-run", "", "report without writing"), jsonFlag, optional("repo", "PATH", "source checkout when separate"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "sync", Status: StatusImplemented, Mutates: true, Writes: []string{"sync-cursor"},
		Usage:   "change-saga sync --repo PATH [--commit REV] [--json] <saga>",
		Summary: "move a companion Saga's sync cursor to the code commit it now documents",
		Flags: []Flag{
			required("repo", "PATH", "code repository checkout the Saga documents"), optional("commit", "REV", "code commit the Saga now documents; defaults to HEAD"),
			jsonFlag, optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
		},
		Positionals: sagaOnly,
	},
	{
		Name: "review create", Status: StatusImplemented, Mutates: true, Writes: []string{"review"},
		Usage:   "change-saga review create --id ID --base REV [--head REF] [--pr N] [--url URL] [--title TEXT] [flags] <saga>",
		Summary: "create the one review of a pull request: a slide deck viewed from the merge-base of --base and the head it follows",
		Flags: []Flag{
			required("id", "ID", "stable review id, for example pr-42"), required("base", "REV", "the revision the pull request merges into"),
			optional("head", "REF", "the ref the review follows as commits are pushed; defaults to the checkout's HEAD"),
			optional("pr", "N", "pull request number"), optional("url", "URL", "pull request URL"), optional("title", "TEXT", "review title"),
			optional("objective", "TEXT", "what the review deck explains"), optional("deck", "ID", "review deck id; defaults to the review id"), jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "review list", Status: StatusImplemented, Usage: "change-saga review list [--review ID] [--uncovered] [--repo PATH] [--json] <saga>",
		Summary: "report every review slide's decisions, the head commit each was given at, and whether each is out of date, and how completely each deck covers its review's range; never a verdict",
		Flags: []Flag{optional("review", "ID", "report one review"),
			optional("uncovered", "", "list only reviews whose deck leaves changes of their range uncovered, and only those gaps"),
			optional("repo", "PATH", "code checkout when separate"), jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "review approve", Status: StatusImplemented, Mutates: true, Writes: []string{"review-approval"},
		Usage:       "change-saga review approve --review ID --slide ID --reviewer-kind human|ai [--body TEXT] [flags] <saga>",
		Summary:     "approve one review slide at the pull request's current head; it goes out of date when the slide or its code changes",
		Flags:       append([]Flag{required("review", "ID", "review id"), required("slide", "ID", "review slide id or URN"), optional("body", "TEXT", "note")}, reviewDecisionFlags...),
		Positionals: sagaOnly,
	},
	{
		Name: "review request-changes", Status: StatusImplemented, Mutates: true, Writes: []string{"review-approval"},
		Usage:       "change-saga review request-changes --review ID --slide ID --reviewer-kind human|ai --body TEXT [flags] <saga>",
		Summary:     "request changes on one review slide at the pull request's current head",
		Flags:       append([]Flag{required("review", "ID", "review id"), required("slide", "ID", "review slide id or URN"), required("body", "TEXT", "what should change")}, reviewDecisionFlags...),
		Positionals: sagaOnly,
	},
	{
		Name: "review withdraw", Status: StatusImplemented, Mutates: true, Writes: []string{"review-approval"},
		Usage:       "change-saga review withdraw --review ID --slide ID --reviewer-kind human|ai [flags] <saga>",
		Summary:     "withdraw your current decision on one review slide",
		Flags:       append([]Flag{required("review", "ID", "review id"), required("slide", "ID", "review slide id or URN"), optional("body", "TEXT", "note")}, reviewDecisionFlags...),
		Positionals: sagaOnly,
	},
	{
		Name: "review comment", Status: StatusImplemented, Mutates: true, Writes: []string{"review-comment"},
		Usage:   "change-saga review comment --review ID (--target SLIDE[/ITEM] | --reply-to ID) --body TEXT --reviewer-kind human|ai [--resolve|--reopen] [flags] <saga>",
		Summary: "comment on a review slide or Item, or reply to a comment; documentation has no comments",
		Flags: append([]Flag{
			required("review", "ID", "review id"), optional("target", "SLIDE[/ITEM]", "review slide or Item: id, <slide>/<item>, or URN"),
			optional("reply-to", "ID", "comment this replies to"), required("body", "TEXT", "Markdown comment"),
			optional("resolve", "", "resolve the thread"), optional("reopen", "", "reopen the thread"),
		}, reviewDecisionFlags...),
		Positionals: sagaOnly,
	},
	{
		Name: "validate", Status: StatusImplemented, Usage: "change-saga validate [--json] [--fix] <saga>",
		Summary:     "check every structural invariant without writing",
		Flags:       []Flag{jsonFlag, optional("fix", "", "add missing stable Markdown heading anchors")},
		Positionals: sagaOnly,
	},
	{
		Name: "status", Status: StatusImplemented, Usage: "change-saga status [--json] [--repo PATH] [--epic ID] [--against REV [--head REV]] <saga>",
		Summary: "report coverage by area (implementation, stories, personas, design, quality, health) with counts and lists, stale pins, and ordered next actions; has no verdict: exits 0 whenever the report can be trusted, 1 when the Saga is malformed or the checkout does not match",
		Flags: []Flag{
			jsonFlag, optional("repo", "PATH", "source checkout when separate"),
			optional("max", "N", "maximum uncovered items in text mode"), optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
			optional("epic", "ID", "narrow the report to one epic"), againstFlag, headFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "check", Status: StatusImplemented, Usage: "change-saga check --covers AREA[,AREA...] [--json] [--repo PATH] [--epic ID] [--against REV [--head REV]] <saga>",
		Summary: "ask whether the named coverage areas are fully covered in scope; exits 0 when they are, 3 with only those areas' gaps when not, and 1 when the report cannot be trusted",
		Flags: []Flag{
			required("covers", "AREA,...", "coverage areas to ask about: implementation, stories, personas, design, quality, health"),
			jsonFlag, optional("repo", "PATH", "source checkout when separate"),
			optional("max", "N", "maximum gaps per area in text mode"), optional("allow-repository-mismatch", "", "accept a checkout whose origin differs"),
			optional("epic", "ID", "narrow the question to one epic"), againstFlag, headFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "spec", Status: StatusImplemented, Usage: "change-saga spec [--json]",
		Summary: "publish resources, relation matrix, command grammar, and invariants",
		Flags:   []Flag{jsonFlag},
	},

	// v5 quality writers.
	{
		Name: "quality test-case add", Status: StatusImplemented, Mutates: true, Writes: []string{"test-case", "test-case-revision", "test-case-event"},
		Usage:   "change-saga quality test-case add --epic ID --id ID [--revision r1] [--event proposed] --title TEXT --kind KIND... --automation MODE --step JSON... --expected-result TEXT [--precondition TEXT...] [--from FILE|-] [flags] <saga>",
		Summary: "create a test case with its initial complete revision (ordered steps, coverage kinds, automation) and proposed event; link it with a verifies relation",
		Flags: []Flag{epicCreateFlag,
			required("id", "ID", "stable test-case id"), optional("revision", "ID", "initial revision id; defaults to r1"), optional("event", "ID", "initial proposed-event id"),
			required("title", "TEXT", "test-case title"), repeatable("kind", "KIND", "positive, negative, or edge", true), required("automation", "MODE", "manual, automated, or hybrid"),
			repeatable("step", "JSON", `ordered step {"id","action","expected_result"}`, true), required("expected-result", "TEXT", "overall expected result"),
			repeatable("precondition", "TEXT", "ordered precondition", false), fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality test-case revise", Status: StatusImplemented, Mutates: true, Writes: []string{"test-case-revision"},
		Usage:   "change-saga quality test-case revise --test URN --parent REVISION... --revision ID [definition flags] [--from FILE|-] [flags] <saga>",
		Summary: "append a complete test-case revision; runs, evidence, and verifies relations pinned to the parent become stale",
		Flags: []Flag{epicFlag,
			required("test", "URN", "canonical test-case URN"), repeatable("parent", "URN", "current revision head URN", true), required("revision", "ID", "new revision id"),
			optional("title", "TEXT", "revised title"), repeatable("kind", "KIND", "positive, negative, or edge", false), optional("automation", "MODE", "manual, automated, or hybrid"),
			repeatable("step", "JSON", "ordered step", false), optional("expected-result", "TEXT", "overall expected result"), repeatable("precondition", "TEXT", "ordered precondition", false),
			fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality test-case set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"test-case-event"},
		Usage:   "change-saga quality test-case set-state --test URN --parent EVENT... --state STATE [--event ID] [--reason TEXT] [flags] <saga>",
		Summary: "append a test-case lifecycle event: proposed, active, deprecated, or retired",
		Flags: []Flag{epicFlag,
			required("test", "URN", "canonical test-case URN"), repeatable("parent", "URN", "current lifecycle head URN", true),
			required("state", "STATE", "proposed, active, deprecated, or retired"), optional("event", "ID", "new event id"), optional("reason", "TEXT", "why the lifecycle changed"),
			fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality policy set", Status: StatusImplemented, Mutates: true, Writes: []string{"quality-policy"},
		Usage:   "change-saga quality policy set --epic ID --criterion URN --story-revision URN --require KIND... --rationale TEXT [--allow MODE...] [--supersedes POLICY...] [--id ID] [flags] <saga>",
		Summary: "record the test kinds one criterion revision requires; positive is required by default",
		Flags: []Flag{epicCreateFlag,
			required("criterion", "URN", "canonical criterion URN"), required("story-revision", "URN", "exact story revision pin"),
			repeatable("require", "KIND", "positive, negative, or edge", true), required("rationale", "TEXT", "why these kinds are required"),
			repeatable("allow", "MODE", "allowed automation: manual, automated, or hybrid", false), repeatable("supersedes", "URN", "current policy head this replaces", false),
			optional("id", "ID", "stable policy id"), fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality evidence add", Status: StatusImplemented, Mutates: true, Writes: []string{"quality-evidence"},
		Usage:   "change-saga quality evidence add --test URN --role ROLE (--code LOCATION... | --verification URN... | --citation URN...) [--test-revision URN] [--supersedes EVIDENCE...] [--id ID] [--batch FILE|-] [flags] <saga>",
		Summary: "record immutable typed evidence for one test revision: test code, the code under test, or execution artifacts",
		Flags: []Flag{epicFlag,
			required("test", "URN", "canonical test-case URN"), required("role", "ROLE", "test_implementation, implementation_under_test, or execution_artifact"),
			repeatable("code", "LOCATION", "code location <commit>:<path>[#L<start>[-L<end>]]; the commit may be any revision", false), optional("repo", "PATH", "source checkout when separate"), repeatable("verification", "URN", "verification URN", false), repeatable("citation", "URN", "citation URN", false),
			optional("test-revision", "URN", "test revision pin; defaults to the unique current head"), repeatable("supersedes", "URN", "evidence this record replaces", false),
			optional("id", "ID", "stable evidence id"), optional("batch", "FILE|-", "batch request; all or none are written"), fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	{
		Name: "quality run record", Status: StatusImplemented, Mutates: true, Writes: []string{"test-run"},
		Usage:   "change-saga quality run record --test URN --result RESULT --summary TEXT --evidence URN... [--parent RUN...] [--test-revision URN] [--command TEXT] [--commit REV] [--id ID] [flags] <saga>",
		Summary: "append an immutable run result pinned to the test revision and the source identity; the command is recorded, never executed",
		Flags: []Flag{epicFlag,
			required("test", "URN", "canonical test-case URN"), required("result", "RESULT", "passed, failed, blocked, or skipped"),
			required("summary", "TEXT", "what ran and what was observed"), repeatable("evidence", "URN", "current evidence URN", true),
			repeatable("parent", "URN", "current run head URN", false), optional("test-revision", "URN", "test revision that ran; defaults to the unique current head"),
			optional("command", "TEXT", "command that ran"), optional("commit", "REV", "code commit the run executed against; defaults to HEAD"),
			optional("repository", "URI", "repository that ran; defaults to the Saga's"), optional("repo", "PATH", "source checkout when separate"),
			optional("id", "ID", "stable run id"), fromFileFlag, requestIDFlag, jsonFlag,
		},
		Positionals: sagaOnly,
	},
	// Coverage exceptions are frozen by schema/v5 but have no writer yet; these
	// shapes are published as planned so the loop is visible end to end.
	{
		Name: "coverage-exception add", Status: StatusPlanned, Mutates: true, Writes: []string{"coverage-exception"},
		Usage:   "change-saga coverage-exception add --epic ID --id ID --axis AXIS --criterion URN --story-revision URN --rationale TEXT --citation URN... [flags] <saga>",
		Summary: "record an explicit, cited decision that one axis does not apply to one criterion revision; never excuses changed-source accounting",
		Flags: []Flag{epicCreateFlag,
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
		Flags:       []Flag{epicFlag, required("exception", "URN", "exception being replaced"), required("with", "URN", "replacing exception"), requestIDFlag, jsonFlag},
		Positionals: sagaOnly,
	},
}
