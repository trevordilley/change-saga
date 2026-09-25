package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/grammar"
)

func reconciliationOf(t *testing.T, root string, args ...string) reconciliationReport {
	t.Helper()
	output := mustRun(t, Reconcile, append(append([]string{"--json", "--against", "main"}, args...), root)...)
	var report reconciliationReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	for _, task := range report.Queue {
		for _, inv := range append(task.Inspect, task.Repair...) {
			if strings.HasPrefix(inv.Command, "query ") {
				operation := strings.TrimPrefix(inv.Command, "query ")
				if _, _, _, err := parseQuery(operation, inv.Argv[3:]); err != nil {
					t.Fatalf("invalid inspection route: %+v: %v", inv, err)
				}
				continue
			}
			if _, ok := grammar.Lookup(inv.Command); !ok {
				t.Fatalf("unknown route: %+v", inv)
			}
		}
	}
	return report
}

func TestReconciliationHeadCurrencyAndRepairLoop(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(3, func() error { return sqs.Send(job) })\n}\n")
	git(t, repo, "commit", "-am", "Retry enqueue")
	report := reconciliationOf(t, root)
	if report.Currency.Regressions != 1 || report.Currency.PreExisting != 0 || report.Currency.Stale != 1 {
		t.Fatalf("currency: %+v", report.Currency)
	}
	compared, _ := statusLayers(t, root, "--against", "main")
	if compared.Report.Summary.Stale != 0 {
		t.Fatal("base-valid deleted-line evidence must remain valid in comparison coverage")
	}
	var repair *reconciliationTask
	story := false
	for i := range report.Queue {
		task := &report.Queue[i]
		if task.Debt == "regression" {
			repair = task
		}
		if task.Resource == "urn:change-saga:shop:story:pay" && task.Debt == "reassess" {
			story = true
		}
	}
	if repair == nil || !story || repair.Repair[0].Command != "replace-coverage" {
		t.Fatalf("queue: %+v", report.Queue)
	}
	mustRun(t, ReplaceCoverage, "--record", repair.EvidenceFile, "--target", repair.Resource, "--commit", "HEAD", "--path", "src/queue.go", "--lines", "3-6", root)
	mustRun(t, Validate, "--json", root)
	repaired := reconciliationOf(t, root)
	if repaired.Currency.Stale != 0 || repaired.Currency.Current != 1 {
		t.Fatalf("repaired: %+v", repaired.Currency)
	}
	// Reassessment remains a human task even after byte currency is restored.
	if len(repaired.Queue) == 0 {
		t.Fatal("repairing a pin must not suppress declared-link reassessment")
	}
}

func TestReconciliationKeepsBaselineDebtAndShiftedCurrent(t *testing.T) {
	repo, root := shopSaga(t)
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(job)\n}\n")
	git(t, repo, "commit", "-am", "Existing undocumented behavior")
	git(t, repo, "checkout", "-b", "header")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "// New header\npackage shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(job)\n}\n")
	git(t, repo, "commit", "-am", "Header")
	report := reconciliationOf(t, root)
	if report.Currency.PreExisting != 1 || report.Currency.Regressions != 0 || report.Currency.BaseStale != 1 {
		t.Fatalf("baseline debt: %+v", report.Currency)
	}
	// Repair then commit the documented state as the next baseline.
	ref := report.Currency.References[0]
	mustRun(t, ReplaceCoverage, "--record", ref.EvidenceFile, "--target", ref.Owner, "--commit", "HEAD", "--path", "src/queue.go", "--lines", "4-7", root)
	commitAll(t, repo, "Repair baseline documentation")
	git(t, repo, "branch", "-f", "main", "HEAD")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "// Another header\n// New header\npackage shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(job)\n}\n")
	git(t, repo, "commit", "-am", "Shift only")
	shifted := reconciliationOf(t, root)
	if shifted.Currency.Stale != 0 || shifted.Currency.Remapped != 1 {
		t.Fatalf("shifted currency: %+v", shifted.Currency)
	}
}

func TestReconciliationRequirementsOnlyImpact(t *testing.T) {
	repo, root := shopSaga(t)

	mustRun(t, Quality, "test-case", "add", "--feature", "checkout", "--id", "charge", "--title", "Charge once", "--kind", "positive", "--automation", "automated", "--step", `{"id":"pay","action":"Pay","expected_result":"Charged once"}`, "--expected-result", "Charged once", root)
	mustRun(t, Relation, "add", "--feature", "checkout", "--id", "test-pay", "--type", "verifies", "--from", "urn:change-saga:shop:test-case:charge", "--to", "urn:change-saga:shop:story:pay:criterion:charged", "--rationale", "Checks the charge count", root)
	commitAll(t, repo, "Declare a test for the requirement")
	git(t, repo, "checkout", "-b", "intent")
	mustRun(t, Story, "revise", "--story", "urn:change-saga:shop:story:pay", "--revision", "r2", "--parent", "urn:change-saga:shop:story:pay:revision:r1", "--title", "Pay once", "--statement", "As a shopper I pay exactly once", "--priority", "must", "--criterion", "charged=The card is charged exactly once", root)
	report := reconciliationOf(t, root)
	if report.DocumentationCoverage.Areas.Implementation.Total != 0 || report.Currency.Stale != 0 {
		t.Fatal("requirement-only edit must not fabricate a code change")
	}
	var story, design, relation, test bool
	for _, task := range report.Queue {
		switch task.Resource {
		case "urn:change-saga:shop:story:pay":
			story = len(task.Inspect) > 0 && task.Inspect[0].Command == "query traceability"
		case "urn:change-saga:shop:fragment:queue-design":
			design = true
		case "urn:change-saga:shop:test-case:charge":
			test = true
		case "urn:change-saga:shop:relation:design-pay":
			relation = len(task.Repair) > 0 && task.Repair[0].Command == "relation repin"
		}
	}
	if !story || !design || !relation || !test {
		t.Fatalf("requirement impact omitted: %+v", report.Queue)
	}
}

func TestReconciliationSelectedHeadAndSourcePaths(t *testing.T) {
	repo, root := shopSaga(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "changed")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\nfunc Enqueue() {}\n")
	git(t, repo, "commit", "-am", "Change code")
	report := reconciliationOf(t, root, "--head", base, "--repo", repo)
	if report.Currency.Stale != 0 || report.Saga.Head.Source != "git" {
		t.Fatalf("selected head: %+v", report)
	}
	for _, task := range report.Queue {
		for _, inv := range task.Inspect {
			if strings.Contains(strings.Join(inv.Argv, " "), "change-saga-snapshot-") {
				t.Fatal("disposable paths leaked into action")
			}
		}
	}
}

func TestReconciliationTransactionRepairUsesSnapshot(t *testing.T) {
	root, repo, assetDir, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, assetDir, commit, sagaID, "initial", "create", "absent", "worker")
	publish := func(request SlideTransactionRequest) SlideTransactionResult {
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(assetDir, request.RequestID+".json")
		writeFile(t, path, string(data))
		output := mustRun(t, ApplySlide, "--from", path, "--repo", repo, "--json", root)
		var result SlideTransactionResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := publish(create)
	git(t, repo, "checkout", "-b", "behavior")
	writeFile(t, filepath.Join(repo, "service.go"), "package service\n\nfunc Run() error { return retry() }\n")
	changed := commitAll(t, repo, "Change implementation")
	report := reconciliationOf(t, root, "--repo", repo)
	var task *reconciliationTask
	for i := range report.Queue {
		if report.Queue[i].Kind == "evidence" {
			task = &report.Queue[i]
			break
		}
	}
	if task == nil || len(task.Repair) != 1 || task.Repair[0].Command != "apply-slide" || task.Inspect[0].Command != "query slide" {
		t.Fatalf("transaction route: %+v", task)
	}
	if report.Currency.Unknown != 1 || report.Currency.BaselineAvailable {
		t.Fatalf("unversioned companion must keep unknown baseline: %+v", report.Currency)
	}
	// The route is runnable and exposes the exact publication guard.
	snapshot := mustRun(t, Query, task.Inspect[0].Argv[2:]...)
	if !strings.Contains(snapshot, created.Snapshot) {
		t.Fatal("query omitted expected snapshot")
	}
	update := slideTransactionRequest(t, repo, assetDir, changed, sagaID, "repair", "update", created.Snapshot, "worker")
	publish(update)
	repaired := reconciliationOf(t, root, "--repo", repo)
	if repaired.Currency.Stale != 0 {
		t.Fatalf("transaction repair did not restore currency: %+v", repaired.Currency)
	}
}

func TestReconciliationReviewCoverageCannotSubstituteForDocs(t *testing.T) {
	fixture := newReviewFixture(t)
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	base := strings.TrimSpace(git(t, fixture.repo, "merge-base", "main", "HEAD"))
	for _, file := range []string{"queue.go", "store.go"} {
		slide := map[string]string{"queue.go": "queue", "store.go": "table"}[file]
		mustRun(t, Cover, "--target", "urn:change-saga:app:review:pr-7:slide:"+slide+":item:node", "--ref", base+":"+file+"#L3", "--repo", fixture.repo, fixture.root)
	}
	report := reconciliationOf(t, fixture.root)
	if len(report.ReviewCoverage) != 1 || report.ReviewCoverage[0].Coverage.Summary.Uncovered != 0 {
		t.Fatalf("review is not fully covered: %+v", report.ReviewCoverage)
	}
	if report.DocumentationCoverage.Areas.Implementation.Covered != 0 || report.DocumentationCoverage.Areas.Implementation.Uncovered != 4 {
		t.Fatalf("review evidence leaked into docs: %+v", report.DocumentationCoverage)
	}
	if report.Currency.Total != 0 {
		t.Fatal("PR evidence leaked into living HEAD currency")
	}
}

func TestReconciliationCompanionBaselineAndSourceIdentity(t *testing.T) {
	root, repo, assetDir, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, assetDir, commit, sagaID, "initial", "create", "absent", "worker")
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(assetDir, "request.json")
	writeFile(t, requestPath, string(data))
	mustRun(t, ApplySlide, "--from", requestPath, "--repo", repo, root)
	docs := filepath.Dir(root)
	git(t, docs, "init", "-b", "docs")
	git(t, docs, "config", "user.name", "Docs Author")
	git(t, docs, "config", "user.email", "docs@example.test")
	mustRun(t, Sync, "--repo", repo, root)
	baseDocs := commitAll(t, docs, "Document base source")
	git(t, repo, "checkout", "-b", "behavior")
	writeFile(t, filepath.Join(repo, "service.go"), "package service\n\nfunc Run() error { return retry() }\n")
	commitAll(t, repo, "Change source")
	report := reconciliationOf(t, root, "--repo", repo)
	if !report.Opening.Companion || report.Opening.Cursor != commit || report.Saga.Base.Commit != baseDocs || report.Currency.Regressions != 1 {
		t.Fatalf("companion semantics lost: %+v; %+v", report.Opening, report.Currency)
	}
	for _, task := range report.Queue {
		for _, inv := range append(task.Inspect, task.Repair...) {
			if inv.Command == "apply-slide" || strings.HasPrefix(inv.Command, "query ") {
				found := false
				for _, arg := range inv.Arguments {
					found = found || arg.Flag == "repo" && arg.Value == repo
				}
				if !found {
					t.Fatalf("route lost source checkout: %+v", inv)
				}
			}
		}
	}
	if dirty := git(t, docs, "status", "--porcelain"); strings.TrimSpace(dirty) != "" {
		t.Fatalf("read-only reconciliation wrote to companion: %s", dirty)
	}
}

func TestReconciliationPreservesTestEvidenceDiffCoverage(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "tests")
	writeFile(t, filepath.Join(repo, "src", "queue_test.go"), "package shop\nfunc TestQueue() {}\n")
	head := commitAll(t, repo, "Add a test")
	mustRun(t, Quality, "test-case", "add", "--feature", "checkout", "--id", "charge", "--title", "Charge once", "--kind", "positive", "--automation", "automated", "--step", `{"id":"pay","action":"Pay","expected_result":"Charged once"}`, "--expected-result", "Charged once", root)
	mustRun(t, Quality, "test-case", "set-state", "--test-case", "urn:change-saga:shop:test-case:charge", "--parent", "urn:change-saga:shop:test-case:charge:event:proposed", "--state", "active", "--reason", "Ready", root)
	mustRun(t, Quality, "evidence", "add", "--test", "urn:change-saga:shop:test-case:charge", "--role", "test_implementation", "--repo", repo, "--code", head+":src/queue_test.go#L1-L2", root)
	report := reconciliationOf(t, root)
	if report.DocumentationCoverage.Areas.Implementation.Total != 3 || report.DocumentationCoverage.Areas.Implementation.Covered != 2 || report.DocumentationCoverage.Areas.Implementation.Uncovered != 1 {
		t.Fatalf("lost current test evidence coverage: %+v", report.DocumentationCoverage.Areas.Implementation)
	}
	if gap := report.DocumentationCoverage.Areas.Implementation.UncoveredEntries[0]; gap.Event != "add" {
		t.Fatalf("a focused line reference must not swallow the file event: %+v", gap)
	}
}

func TestReconciliationNewStaleEvidenceDoesNotInheritBaselineDebt(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "drift")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\nfunc Enqueue() {}\n")
	commitAll(t, repo, "Replace queue")
	// Newly authored historical evidence is valid at its pin, but stale now.
	mustRun(t, Cover, "--target", "urn:change-saga:shop:fragment:queue-design", "--commit", "main", "--path", "src/queue.go", "--lines", "3-6", "--name", "new-historical", "--note", "New explanation of historical behavior", root)
	report := reconciliationOf(t, root)
	if report.Currency.Regressions != 1 || report.Currency.Introduced != 1 || report.Currency.PreExisting != 0 {
		t.Fatalf("new debt misclassified: %+v", report.Currency)
	}
}

func TestReconciliationSupersededEvidenceAndRunRepair(t *testing.T) {
	repo, root := shopSaga(t)
	mustRun(t, Story, "set-state", "--story", "urn:change-saga:shop:story:pay", "--parent", "urn:change-saga:shop:story:pay:event:proposed", "--state", "accepted", "--reason", "Fixture requirement", "--event", "accepted", root)
	test := "urn:change-saga:shop:test-case:charge"
	mustRun(t, Quality, "test-case", "add", "--feature", "checkout", "--id", "charge", "--title", "Charge once", "--kind", "positive", "--automation", "automated", "--step", `{"id":"pay","action":"Pay","expected_result":"Charged once"}`, "--expected-result", "Charged once", root)
	mustRun(t, Quality, "test-case", "set-state", "--test", test, "--parent", test+":event:proposed", "--state", "active", "--reason", "Ready", root)
	mustRun(t, Relation, "add", "--feature", "checkout", "--id", "test-pay", "--type", "verifies", "--from", test, "--to", "urn:change-saga:shop:story:pay:criterion:charged", "--rationale", "Checks the charge count", root)
	mustRun(t, Quality, "evidence", "add", "--test", test, "--id", "old", "--role", "implementation_under_test", "--code", "HEAD:src/queue.go#L3-L6", root)
	commitAll(t, repo, "Document original test")
	git(t, repo, "checkout", "-b", "repair")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\nfunc Enqueue() {}\n")
	commitAll(t, repo, "Revise queue")
	mustRun(t, Quality, "evidence", "add", "--test", test, "--id", "replacement", "--role", "implementation_under_test", "--supersedes", test+":evidence:old", "--code", "HEAD:src/queue.go#L2", root)
	report := reconciliationOf(t, root)
	if report.Currency.HistoricalStale != 1 || report.Currency.Historical != 1 || report.Currency.Regressions != 1 {
		t.Fatalf("superseded evidence must remain visible without adding active debt: %+v", report.Currency)
	}
	for _, task := range report.Queue {
		if task.Resource == test+":evidence:old" && task.Debt != "reassess" {
			t.Fatalf("history became a repair task: %+v", task)
		}
	}
	mustRun(t, Quality, "run", "record", "--test", test, "--id", "before", "--result", "passed", "--summary", "Fixture result", "--evidence", test+":evidence:replacement", root)
	mustRun(t, Quality, "test-case", "revise", "--test", test, "--parent", test+":revision:r1", "--revision", "r2", "--expected-result", "Charged exactly once", root)
	report = reconciliationOf(t, root)
	found := false
	for _, task := range report.Queue {
		if task.Kind != "test_run" {
			continue
		}
		found = true
		if task.Inspect[0].Command != "status" {
			t.Fatalf("wrong run inspection: %+v", task)
		}
		output := mustRun(t, Status, task.Inspect[0].Argv[2:]...)
		if !strings.Contains(output, test) {
			t.Fatal("run inspection lost test context")
		}
		for _, repair := range task.Repair {
			if repair.Command != "quality run record" {
				continue
			}
			args := strings.Join(repair.Argv, " ")
			if !strings.Contains(args, test+":run:before") || !strings.Contains(args, "--commit") {
				t.Fatalf("rerun lost current heads/commit: %+v", repair)
			}
		}
	}
	if !found {
		t.Fatalf("missing stale-run task: %+v", report.Queue)
	}
}
