package grammar

func init() {
	common := []Flag{optional("request-id", "ID", "idempotency key"), optional("json", "", "machine-readable result")}
	definition := []Flag{required("target", "URN", "existing content identity; automatically pins current content"), required("scope", "content|code", "explicit assertion scope; code requires exact references"), required("reason", "TEXT", "authored explanation"), optional("element", "PATH", "revision-local whiteboard appearance"), optional("replacement", "URN", "optional successor"), optional("replacement-element", "PATH", "successor whiteboard appearance"), repeatable("ref", "LOCATION", "exact deprecated code for code scope; related evidence for content scope", false), optional("repo", "PATH", "code checkout")}
	commands = append(commands,
		Command{Name: "deprecation add", Status: StatusImplemented, Mutates: true, Writes: []string{"deprecation", "deprecation-revision", "deprecation-event"}, Usage: "change-saga deprecation add --id ID --target URN --scope content|code --reason TEXT [--ref LOCATION...] [--replacement URN] [flags] <saga>", Summary: "record an authored deprecation without changing its target", Flags: append(append([]Flag{required("id", "ID", "stable annotation identity"), optional("revision", "ID", "initial revision; default r1"), optional("event", "ID", "initial event; default active")}, definition...), common...)},
		Command{Name: "deprecation revise", Status: StatusImplemented, Mutates: true, Writes: []string{"deprecation-revision"}, Usage: "change-saga deprecation revise --deprecation URN --revision ID --parent URN... --target URN --scope content|code --reason TEXT [--ref LOCATION...] [--replacement URN] [flags] <saga>", Summary: "append a complete corrected assertion, reconciling all observed heads", Flags: append(append([]Flag{required("deprecation", "URN", "annotation identity"), required("revision", "ID", "new revision"), repeatable("parent", "URN", "every current revision head", true)}, definition...), common...)},
		Command{Name: "deprecation set-state", Status: StatusImplemented, Mutates: true, Writes: []string{"deprecation-event"}, Usage: "change-saga deprecation set-state --deprecation URN --event ID --parent URN... --state active|withdrawn --reason TEXT [flags] <saga>", Summary: "withdraw or restore an assertion without removing history", Flags: append([]Flag{required("deprecation", "URN", "annotation identity"), required("event", "ID", "new event"), repeatable("parent", "URN", "every current lifecycle head", true), required("state", "active|withdrawn", "assertion lifecycle"), required("reason", "TEXT", "why the assertion was withdrawn or restored")}, common...)},
	)
	for _, kind := range []string{"deprecation", "deprecation-revision", "deprecation-event"} {
		member, path, history := "", "___deprecations/<id>.deprecation/deprecation.json", "immutable identity"
		writers := []string{"deprecation add"}
		if kind == "deprecation-revision" {
			member = ":revision:<revision>"
			path = "___deprecations/<id>.deprecation/revisions/<revision>.json"
			history = "complete immutable assertions; multiple heads remain a conflict"
			writers = append(writers, "deprecation revise")
		}
		if kind == "deprecation-event" {
			member = ":event:<event>"
			path = "___deprecations/<id>.deprecation/events/<event>.json"
			history = "append-only active/withdrawn lifecycle with reasons"
			writers = append(writers, "deprecation set-state")
		}
		resources = append(resources, Resource{Kind: kind, URN: "urn:change-saga:<saga>:deprecation:<id>" + member, Storage: path, Schema: schemaBase + "v5/" + kind + ".schema.json", Versions: []int{5}, History: history, Writers: writers, Notes: "content scope never deprecates related code; code scope asserts only explicit exact references; optional stories use pinned explains relations"})
	}
	for i := range relations {
		if relations[i].Type == "explains" {
			relations[i].Sources = append(relations[i].Sources, "deprecation")
			relations[i].RequiredPins = append(relations[i].RequiredPins, "from_revision for deprecation")
			relations[i].Meaning += "; deprecation supplies optional story context with self scope, never code coverage"
		}
	}
}
