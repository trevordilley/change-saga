package grammar

func init() {
	for _, kind := range []string{"label", "milestone"} {
		for _, op := range []string{"add", "rename"} {
			selector := "id"
			idUsage := "ID"
			if op == "rename" {
				selector = kind
				idUsage = "ID|URN"
			}
			commands = append(commands, Command{Name: kind + " " + op, Status: StatusImplemented, Mutates: true, Writes: []string{kind}, Usage: "change-saga " + kind + " " + op + " --" + selector + " " + idUsage + " --title TEXT [--json] <saga>", Summary: "author a shared feature " + kind + " without rewriting feature references", Flags: []Flag{required(selector, idUsage, "stable record identity"), required("title", "TEXT", "shared display name"), jsonFlag}, Positionals: sagaOnly})
		}
		resources = append(resources, Resource{Kind: kind, URN: "urn:change-saga:<saga>:" + kind + ":<id>", Storage: "___" + kind + "s/<id>.json", Schema: schemaBase + "v5/" + kind + ".schema.json", Versions: []int{5}, History: "stable identity; Git preserves title edits", Writers: []string{kind + " add", kind + " rename", kind + " remove"}, Notes: "features reference a shared record; duplicate titles are rejected within this kind"})
	}
}
