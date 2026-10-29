package cli

import "github.com/twentyideas/changesaga/internal/grammar"

// Keep the public authoring help and machine-readable contract on the same
// command definitions. Family entry points still own their flag parsing.
func init() {
	for _, name := range []string{
		"move-deck", "move-slide", "move-chapter", "move-section", "move-fragment",
		"feature rename", "feature remove", "feature archive", "feature restore", "feature-theme rename", "feature-theme remove",
		"label remove", "milestone remove",
		"whiteboard list", "whiteboard rename", "whiteboard assign-role", "whiteboard unassign-role",
		"whiteboard archive", "whiteboard restore",
	} {
		if command, ok := grammar.Lookup(name); ok {
			commandUsage[name] = command.Usage
			commandDescription[name] = command.Summary
		}
	}
}
