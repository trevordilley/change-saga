package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunQueryKeepsMachineOutputOnStdout(t *testing.T) {
	tests := []struct {
		name string
		args []string
		exit int
	}{
		{"help", []string{"query", "--help"}, 0},
		{"invalid", []string{"query", "overview"}, 2},
		{"missing saga", []string{"query", "overview", "--saga", "review.saga"}, 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(test.args, &stdout, &stderr); got != test.exit {
				t.Fatalf("exit = %d, want %d", got, test.exit)
			}
			if stderr.Len() != 0 {
				t.Fatalf("structured query wrote stderr: %q", stderr.String())
			}
			decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
			var envelope map[string]any
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
			}
			if err := decoder.Decode(&map[string]any{}); !errors.Is(err, io.EOF) {
				t.Fatalf("stdout was not exactly one JSON value: %v\n%s", err, stdout.String())
			}
			if envelope["schema"] != "change-saga.ai/v1" {
				t.Fatalf("wrong schema: %#v", envelope)
			}
		})
	}
}

func TestRunMutationJSONKeepsFailureOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"cover", "--ref", "not-a-location", "--json", "missing.saga"}, &stdout, &stderr); got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("structured mutation wrote stderr: %q", stderr.String())
	}
	var result struct {
		OK       bool `json:"ok"`
		Failures []struct {
			Message string `json:"message"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.OK || len(result.Failures) != 1 {
		t.Fatalf("mutation failure = %#v, err=%v\n%s", result, err, stdout.String())
	}
}

func TestRunDesignHelpUsesNestedDispatcher(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"design", "--help"}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", got, stderr.String())
	}
	for _, operation := range []string{"design add-chapter", "design add-section", "design add-fragment", "design set-fragment-content"} {
		if !bytes.Contains(stdout.Bytes(), []byte(operation)) {
			t.Errorf("design help omitted %q:\n%s", operation, stdout.String())
		}
	}
}

func TestRunLivingMutationFamiliesUseTheSupportedJSONContract(t *testing.T) {
	for _, family := range []string{"story", "criterion", "citation", "relation", "plan"} {
		t.Run(family, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run([]string{family, "unknown", "--json"}, &stdout, &stderr); got != 1 {
				t.Fatalf("exit = %d, want 1", got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("structured mutation wrote stderr: %q", stderr.String())
			}
			var result struct {
				OK        bool   `json:"ok"`
				Operation string `json:"operation"`
				Created   []any  `json:"created"`
				EventIDs  []any  `json:"event_ids"`
				Error     *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
			if err := decoder.Decode(&result); err != nil {
				t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
			}
			if err := decoder.Decode(&map[string]any{}); !errors.Is(err, io.EOF) {
				t.Fatalf("stdout was not exactly one JSON value: %v\n%s", err, stdout.String())
			}
			if result.OK || result.Operation != family+" unknown" || result.Error == nil || result.Error.Code == "" || result.Created == nil || result.EventIDs == nil {
				t.Fatalf("unexpected mutation failure: %#v", result)
			}
		})
	}
}

// A second Saga is guidance, not a failure: init still exits 0.
func TestRunInitExitsZeroBesideAnExistingSaga(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"remote", "add", "origin", "https://example.test/acme/shop.git"}} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if err := os.Mkdir(filepath.Join(repo, "app.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if got := run([]string{"init", "--repo", repo, filepath.Join(repo, "change.saga")}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit = %d, want 0\n%s%s", got, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "  - app.saga") || !strings.Contains(stdout.String(), "one Saga per repository") {
		t.Fatalf("init did not note the existing Saga:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(repo, "change.saga", "saga.json")); err != nil {
		t.Fatal(err)
	}
}
