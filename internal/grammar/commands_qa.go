package grammar

func init() {
	repoFlag := optional("repo", "PATH", "code checkout when separate")
	commands = append(commands,
		Command{Name: "e2e-suite clear", Status: StatusImplemented, Mutates: true, Writes: []string{"qa-suite"}, Usage: "change-saga e2e-suite clear [--json] <saga>", Summary: "stop saying where end-to-end tests live; the test cases and their code are untouched", Flags: []Flag{jsonFlag}, Positionals: sagaOnly},
		Command{Name: "e2e-suite set", Status: StatusImplemented, Mutates: true, Writes: []string{"qa-suite"}, Usage: "change-saga e2e-suite set [--path PATH...] [--command TEMPLATE] [--json] <saga>", Summary: "say where end-to-end tests live and how to run a chosen few; the template names them with {locations} and is never run by this tool", Flags: []Flag{repeatable("path", "PATH", "repository path holding end-to-end tests; replaces the current paths", false), optional("command", "TEMPLATE", "command that runs the tests named by {locations}; empty removes it"), jsonFlag}, Positionals: sagaOnly},
		Command{Name: "e2e-suite show", Status: StatusImplemented, Usage: "change-saga e2e-suite show [--repo PATH] [--json] <saga>", Summary: "print where end-to-end tests live and how to run them, and report a path that does not exist; none of this is a problem to fix", Flags: []Flag{repoFlag, jsonFlag}, Positionals: sagaOnly},
	)
	resources = append(resources,
		Resource{Kind: "qa-suite", URN: "none: one per Saga", Storage: "___qa/suite.json", Writers: []string{"e2e-suite set", "e2e-suite clear"}},
	)
}
