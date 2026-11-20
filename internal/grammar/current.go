package grammar

import "strings"

// currentCommand removes authoring controls for Saga's former content graph.
// Git supplies versions, branches and merges; the author supplies current data.
func currentCommand(command Command) Command {
	editable := false
	for _, family := range []string{"story", "criterion", "persona", "flag", "term", "component", "system", "data-entity", "erd", "erd-overlay", "prototype", "deprecation", "whiteboard", "quality test-case"} {
		if strings.HasPrefix(command.Name, family+" ") {
			editable = true
			break
		}
	}
	if !editable {
		return command
	}
	flags := make([]Flag, 0, len(command.Flags))
	for _, flag := range command.Flags {
		if flag.Name == "revision" || flag.Name == "parent" || flag.Name == "event" {
			continue
		}
		flags = append(flags, flag)
	}
	command.Flags = flags
	seen := map[string]bool{}
	writes := []string{}
	for _, resource := range command.Writes {
		resource = strings.TrimSuffix(strings.TrimSuffix(resource, "-revision"), "-event")
		if !seen[resource] {
			writes = append(writes, resource)
			seen[resource] = true
		}
	}
	command.Writes = writes
	parts := []string{"change-saga", command.Name}
	for _, flag := range flags {
		value := "--" + flag.Name
		if flag.Value != "" {
			value += " " + flag.Value
		}
		if flag.Repeatable {
			value += " ..."
		}
		if !flag.Required {
			value = "[" + value + "]"
		}
		parts = append(parts, value)
	}
	parts = append(parts, command.Positionals...)
	command.Usage = strings.Join(parts, " ")
	if command.Mutates {
		command.Summary = "Edit current " + strings.Split(command.Name, " ")[0] + " documentation; Git records its history."
	}
	switch command.Name {
	case "story add":
		command.Summary = "Add a sourced story with its current acceptance-criteria. It may precede a prototype or evolve alongside one; leave unknown criteria unauthored. Git records earlier content."
	case "story revise":
		command.Summary = "Edit the current story. Omitted fields keep their current values; structured --from supplies complete content. Preserve stable criterion identities and exact evidence pins. Git records earlier content."
	case "story set-state":
		command.Summary = "Set the current state and reason. Acceptance records intent, not completion or approval. Before setting accepted, include at least one pass/fail criterion; use the narrowest obligation supported by confirmed intent. Git records earlier state."
	case "criterion add":
		command.Summary = "Add an independent pass/fail criterion using a stable identity. Choose the narrowest obligation supported by confirmed intent. Edit current story content; Git records earlier values."
	}
	return command
}

func obsoleteContentFlag(command, name string) bool {
	if name != "revision" && name != "parent" && name != "event" {
		return false
	}
	for _, original := range commands {
		if original.Name != command {
			continue
		}
		_, before := original.Flag(name)
		_, after := currentCommand(original).Flag(name)
		return before && !after
	}
	return false
}
