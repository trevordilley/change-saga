package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestDoctorReportsCapabilitiesWithoutASaga(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := Doctor(context.Background(), []string{"--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != "change-saga.doctor/v1" || report.SagaCheck != nil || report.Build.Executable == "" || report.Build.ResolvedExecutable == "" || !strings.HasPrefix(report.Build.SHA256, "sha256:") {
		t.Fatalf("missing build diagnostics: %+v", report)
	}
	caps := report.Capabilities
	if caps.Plans.Layout != "phases" || !caps.Plans.Authoring || !caps.Plans.Checkpoints || caps.Plans.LegacyWaves || !reflect.DeepEqual(caps.Plans.Queries, []string{"plans", "plan", "task"}) || !caps.Saga.CurrentRecords || !caps.Saga.StrictJSON || caps.Saga.ManifestVersion != 5 {
		t.Fatalf("incorrect capabilities: %+v", caps)
	}
	// Reported query names must actually be available independently of data.
	for _, operation := range caps.Plans.Queries {
		var schema bytes.Buffer
		if err := Query(context.Background(), []string{"schema", operation}, &schema); err != nil || !strings.Contains(schema.String(), `"ok":true`) {
			t.Fatalf("advertised query %s is unavailable: %v %s", operation, err, &schema)
		}
	}
}

func TestDoctorValidatesCurrentRecordsAndLeavesBytesUnchanged(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Plan, "add-phase", "--plan", "first", "--id", "build", "--title", "Build", root)
	run(t, Plan, "add-phase", "--plan", "first", "--id", "security", "--title", "Security review", "--checkpoint", root)
	run(t, Plan, "expect", "--plan", "first", "--checkpoint", "security", "--id", "approved", "--statement", "The designated person approved the design", root)
	run(t, Task, "add", "--plan", "first", "--id", "work", "--title", "Work", "--phase", "build", "--story", planStoryURN, root)
	if err := Normalize(context.Background(), []string{root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root)
	var out bytes.Buffer
	if err := Doctor(context.Background(), []string{"--json", root}, &out); err != nil {
		t.Fatalf("doctor: %v %s", err, &out)
	}
	var report doctorReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.SagaCheck == nil || !report.SagaCheck.Valid {
		t.Fatalf("report: %s; %v", &out, err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("doctor changed authored files")
	}
	// Plan queries remain usable on normalized whole-Saga data.
	if err := Query(context.Background(), []string{"plan", "--saga", root, "--plan", "first"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	plan := loadedPlan(t, root, "first")
	if gate := plan.Phases[1].Checkpoint; gate == nil || gate.Passed != nil || len(gate.Expectations) != 1 {
		t.Fatalf("checkpoint changed during normalized read: %+v", gate)
	}
}

func TestDoctorChecksRecordsOutsidePlans(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	// A broken current persona must fail even though the Plan itself loads.
	path := filepath.Join(root, "___personas", testPersona+".persona", "current.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"content":{"unexpected":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root)
	var out bytes.Buffer
	err := Doctor(context.Background(), []string{"--json", root}, &out)
	var status *StatusError
	var report doctorReport
	if decodeErr := json.Unmarshal(out.Bytes(), &report); decodeErr != nil {
		t.Fatalf("doctor returned invalid JSON: %v", decodeErr)
	}
	found := false
	if report.SagaCheck != nil {
		for _, issue := range report.SagaCheck.Issues {
			if issue.Severity == "error" && strings.Contains(issue.Message, path) {
				found = true
			}
		}
	}
	if !errors.As(err, &status) || status.Code != 1 || report.SagaCheck == nil || report.SagaCheck.Valid || !found {
		t.Fatalf("doctor ignored non-Plan content: %v %s", err, &out)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("failed diagnosis changed authored files")
	}
}

func TestUnsupportedPlanFieldsArePreservedAndDiagnosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		task  bool
		field string
		value any
		hint  string
	}{
		{"unknown checkpoint field", false, "phases", []any{map[string]any{"id": "gate", "title": "Gate", "checkpoint": map[string]any{"future_gate": true}}}, "unknown field"},
		{"null checkpoint", false, "phases", []any{map[string]any{"id": "gate", "title": "Gate", "checkpoint": nil}}, "checkpoint must be an object"},
		{"wave plan", false, "waves", []any{}, "legacy wave/work-item"},
		{"wave task", true, "wave", "old-stage", "legacy wave/work-item"},
		{"unknown plan field", false, "future_field", true, `unknown field`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newPlanFixture(t).root
			path := filepath.Join(root, saga.PlansDir, "first.plan", saga.PlanManifestName)
			if test.task {
				run(t, Task, "add", "--plan", "first", "--id", "work", "--title", "Work", "--story", planStoryURN, root)
				path = filepath.Join(root, saga.PlansDir, "first.plan", saga.PlanTasksDir, "work.task", saga.TaskManifestName)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			record[test.field] = test.value
			data, _ = json.Marshal(record)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotFiles(t, root)
			var out bytes.Buffer
			if err := Doctor(context.Background(), []string{"--json", root}, &out); err == nil || !strings.Contains(out.String(), test.hint) {
				t.Fatalf("doctor: %v %s", err, &out)
			}
			out.Reset()
			if err := Query(context.Background(), []string{"plans", "--saga", root}, &out); err == nil || !strings.Contains(out.String(), "invalid_saga") || !strings.Contains(out.String(), test.hint) {
				t.Fatalf("query: %v %s", err, &out)
			}
			if message := refused(t, planCommand, "add-phase", "--plan", "first", "--id", "more", "--title", "More", root); !strings.Contains(message, "run change-saga validate") {
				t.Fatalf("writer: %s", message)
			}
			if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
				t.Fatal("unsupported fields were rewritten or discarded")
			}
		})
	}
}
