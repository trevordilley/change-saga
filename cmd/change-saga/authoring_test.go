package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Exercise the executable dispatcher as well as the family handlers: grammar
// and direct model tests alone cannot catch an unreachable public command.
func TestRunAuthoringOperationsExposeHelpAndStructuredFailures(t *testing.T) {
	commands := []string{
		"move-deck", "move-slide", "move-chapter", "move-section", "move-fragment",
		"feature rename", "feature remove", "feature-theme rename", "feature-theme remove",
		"label remove", "milestone remove", "whiteboard rename", "whiteboard assign-role",
		"whiteboard unassign-role", "whiteboard archive", "whiteboard restore",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(append(strings.Fields(command), "--help"), &stdout, &stderr); code != 0 {
				t.Fatalf("help exit=%d: %s%s", code, &stdout, &stderr)
			}
			if !strings.Contains(stdout.String(), "change-saga "+command) || !strings.Contains(stdout.String(), "-dry-run") {
				t.Fatalf("missing public usage or preview option: %s", &stdout)
			}
			stdout.Reset()
			stderr.Reset()
			if code := run(append(strings.Fields(command), "--json"), &stdout, &stderr); code != 1 {
				t.Fatalf("invalid invocation exit=%d: %s%s", code, &stdout, &stderr)
			}
			var result map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || stderr.Len() != 0 || result["error"] == nil {
				t.Fatalf("expected one structured error on stdout: %v, stdout=%s stderr=%s", err, &stdout, &stderr)
			}
		})
	}
}
