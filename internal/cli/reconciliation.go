package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Reconciliation is a read-only omission/impact report, never a verdict or a
// mutation plan. Its currency axis deliberately does not use coverage.Sides.
type reconciliationReport struct {
	Snapshot              string                  `json:"snapshot"`
	Schema                string                  `json:"schema"`
	Opening               opening                 `json:"opening"`
	Saga                  changeview.SagaSides    `json:"saga"`
	DocumentationCoverage areas.Report            `json:"documentation_coverage"`
	ReviewCoverage        []reviewstate.Report    `json:"reviews"`
	HeadHealth            areas.Area              `json:"head_health"`
	Currency              reconciliationCurrency  `json:"currency"`
	Changed               []changeview.Change     `json:"changed"`
	Queue                 []reconciliationTask    `json:"queue"`
	Diagnostics           []changeview.Diagnostic `json:"diagnostics"`
	Limits                []string                `json:"limits"`
	Recheck               []grammar.Invocation    `json:"recheck"`
}

type reconciliationCurrency struct {
	Historical        int                       `json:"historical"`
	HistoricalStale   int                       `json:"historical_stale"`
	Total             int                       `json:"total"`
	Current           int                       `json:"current"`
	Remapped          int                       `json:"remapped"`
	Stale             int                       `json:"stale"`
	PreExisting       int                       `json:"pre_existing"`
	Regressions       int                       `json:"regressions"`
	Introduced        int                       `json:"introduced"`
	Unknown           int                       `json:"baseline_unknown"`
	BaseStale         int                       `json:"base_stale"`
	BaselineAvailable bool                      `json:"baseline_available"`
	References        []reconciliationReference `json:"references"`
}

type reconciliationReference struct {
	HistoryReasons []string `json:"history_reasons,omitempty"`
	ownedReference
	Head coderesolve.Resolution  `json:"head"`
	Base *coderesolve.Resolution `json:"base,omitempty"`
	Debt string                  `json:"debt"`
}

type reconciliationTask struct {
	Resource     string               `json:"resource"`
	Kind         string               `json:"kind"`
	Debt         string               `json:"debt"`
	Because      []changeview.Cause   `json:"because"`
	EvidenceFile string               `json:"evidence_file,omitempty"`
	Reference    int                  `json:"reference,omitempty"`
	Guidance     string               `json:"guidance"`
	Inspect      []grammar.Invocation `json:"inspect"`
	Repair       []grammar.Invocation `json:"repair"`
}

func Reconcile(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("reconcile", commandUsage["reconcile"], out)
	jsonOutput := flags.Bool("json", false, "emit the complete reconciliation queue as JSON")
	repo := flags.String("repo", "", "source repository checkout when separate")
	opening := registerOpenFlags(flags)
	allowMismatch := flags.Bool("allow-repository-mismatch", false, "accept a checkout whose origin differs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || opening.rng().Against == "" {
		return fmt.Errorf("usage: %s", commandUsage["reconcile"])
	}
	report, err := buildReconciliation(ctx, flags.Arg(0), *repo, opening.rng(), *allowMismatch)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "Documentation reconciliation %s..%s\n", shortOID(report.Opening.BaseOID), shortOID(report.Opening.HeadOID))
	fmt.Fprintf(out, "HEAD currency: %d current (%d remapped), %d stale: %d pre-existing, %d regressions, %d introduced, %d baseline unknown, %d historical\n", report.Currency.Current, report.Currency.Remapped, report.Currency.Stale, report.Currency.PreExisting, report.Currency.Regressions, report.Currency.Introduced, report.Currency.Unknown, report.Currency.HistoricalStale)
	fmt.Fprintf(out, "Documentation diff coverage: %d/%d; open review decks: %d (independent ranges and coverage in --json)\n", report.DocumentationCoverage.Areas.Implementation.Covered, report.DocumentationCoverage.Areas.Implementation.Total, len(report.ReviewCoverage))
	for _, review := range report.ReviewCoverage {
		if review.Coverage != nil {
			fmt.Fprintf(out, "  Review %s: %d/%d changed lines in %s..%s\n", review.ID, review.Coverage.Summary.Covered, review.Coverage.Summary.Total, shortOID(review.Coverage.BaseOID), shortOID(review.Coverage.HeadOID))
		}
	}
	for _, task := range report.Queue {
		fmt.Fprintf(out, "\n%s [%s; %s]\n", task.Resource, task.Kind, task.Debt)
		for _, cause := range task.Because {
			fmt.Fprintf(out, "  %s: %s", cause.Kind, cause.Detail)
			if cause.Via != "" {
				fmt.Fprintf(out, " (via %s)", cause.Via)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "  "+task.Guidance)
		for _, command := range task.Inspect {
			fmt.Fprintln(out, "  inspect: "+renderReconciliationCommand(command))
		}
		for _, command := range task.Repair {
			fmt.Fprintln(out, "  repair shape (supply author inputs): "+renderReconciliationCommand(command))
		}
	}
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(out, "Note %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
	for _, limit := range report.Limits {
		fmt.Fprintln(out, limit)
	}
	fmt.Fprintln(out, "\nCheck again:")
	for _, command := range report.Recheck {
		fmt.Fprintln(out, "  "+renderReconciliationCommand(command))
	}
	return nil
}

func renderReconciliationCommand(command grammar.Invocation) string {
	parts := make([]string, len(command.Argv))
	for i, value := range command.Argv {
		parts[i] = "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return strings.Join(parts, " ")
}

func buildReconciliation(ctx context.Context, root, repo string, rng gitdiff.Range, allowMismatch bool) (reconciliationReport, error) {
	opened, err := readComparison(ctx, root, repo, rng, allowMismatch)
	if err != nil {
		return reconciliationReport{}, err
	}
	snapshot, err := reviewapp.Snapshot(ctx, root, opened.changes)
	if err != nil {
		return reconciliationReport{}, err
	}
	fixed := gitdiff.Range{Against: opened.changes.BaseOID, Head: opened.changes.HeadOID}
	compared, err := buildStatus(ctx, root, repo, fixed, allowMismatch, "")
	if err != nil {
		return reconciliationReport{}, err
	}
	if err := compared.trustworthy(root); err != nil {
		return reconciliationReport{}, err
	}
	layers := compared.Comparison
	if layers == nil {
		return reconciliationReport{}, fmt.Errorf("reconciliation requires a comparison")
	}
	// Resolve the source once, and use the exact Saga snapshots selected by the
	// layers (including companion cursor semantics) for both health and debt.
	resolver, err := coderesolve.New(ctx, opened.checkout)
	if err != nil {
		return reconciliationReport{}, err
	}
	defer resolver.Close()
	result := reconciliationReport{Snapshot: snapshot, Schema: "change-saga.reconciliation/v1", Opening: compared.Opening, Saga: layers.Saga, Changed: layers.Changed, Queue: []reconciliationTask{}, Diagnostics: append([]changeview.Diagnostic{}, layers.Diagnostics...),
		Limits: []string{"Affected records require reassessment, not necessarily edits. Only declared links and exact references establish impact; undeclared dependencies are not discovered.", "Current pins prove byte currency, not semantic correctness. Review diff coverage never substitutes for living documentation. Validate checks structure only.", "The queue does not approve slides, verify behavior, repin evidence, or record a review. Repairs require author judgment; rerun relevant code and test checks."},
	}
	result.Opening.Against, result.Opening.Head = opened.changes.Base, opened.changes.Head
	var document *saga.Saga
	var head statusDocument
	var owned []ownedReference
	tests := map[string]quality.TestCase{}
	historical := map[string][]string{}
	err = changeview.ReadSnapshot(ctx, root, layers.Saga.Head, func(snapshot string) error {
		var e error
		document, _, e = saga.Load(snapshot)
		if e != nil {
			return e
		}
		qualityDoc, e := quality.Load(snapshot)
		if e != nil {
			return e
		}
		for _, test := range qualityDoc.TestCases {
			urn, _ := qualityid.TestCase(document.Manifest.ID, test.Identity.ID)
			tests[urn] = test
			heads := map[string]bool{}
			for _, h := range test.EvidenceHeads {
				heads[h] = true
			}
			for _, evidence := range test.Evidence {
				owner, _ := qualityid.Evidence(document.Manifest.ID, test.Identity.ID, evidence.ID)
				if !heads[owner] {
					historical[owner] = []string{"evidence was superseded"}
				}
			}
		}
		owned, e = sagaReferences(document)
		if e != nil {
			return e
		}
		head, e = buildStatus(ctx, snapshot, opened.checkout, gitdiff.Range{Head: compared.Opening.HeadOID}, allowMismatch, "")
		if e != nil {
			return e
		}
		if e = head.trustworthy(root); e != nil {
			return e
		}
		diff, e := readComparison(ctx, snapshot, opened.checkout, gitdiff.Range{Against: compared.Opening.BaseOID, Head: compared.Opening.HeadOID}, allowMismatch)
		if e != nil {
			return e
		}
		diffLiving, e := livingapp.LoadStatus(ctx, livingapp.StatusOptions{SagaRoot: snapshot, Document: document, Report: diff.report, Changes: diff.changes, Resolver: resolver})
		if e != nil {
			return e
		}
		result.DocumentationCoverage = areas.Evaluate(coverageInputs(document, diff.changes, diff.report, diffLiving, layers, ""))
		return nil
	})
	if err != nil {
		return result, err
	}
	result.HeadHealth, result.ReviewCoverage = head.Coverage.Areas.Health, head.Reviews
	baseRefs := map[string]coderesolve.Resolution{}
	baseProblems := map[string]bool{}
	baseline := layers.Saga.Base.Source == changeview.SideAbsent && len(layers.Diagnostics) == 0
	if layers.Saga.Base.Source == changeview.SideGit {
		err = changeview.ReadSnapshot(ctx, root, layers.Saga.Base, func(snapshot string) error {
			baseDoc, validation, e := saga.Load(snapshot)
			if e != nil {
				return e
			}
			if !validation.Valid {
				return fmt.Errorf("base Saga is malformed")
			}
			refs, e := sagaReferences(baseDoc)
			if e != nil {
				return e
			}
			for _, ref := range refs {
				resolution := resolver.Resolve(ctx, ref.Code, compared.Opening.BaseOID)
				baseRefs[reconciliationReferenceKey(ref)] = resolution
				if !resolution.Current() {
					result.Currency.BaseStale++
				}
			}
			status, e := buildStatus(ctx, snapshot, opened.checkout, gitdiff.Range{Head: compared.Opening.BaseOID}, allowMismatch, "")
			if e != nil {
				return e
			}
			if e = status.trustworthy(root); e != nil {
				return e
			}
			for _, entry := range status.Coverage.Areas.Health.UncoveredEntries {
				baseProblems[entry.Resource+"\x00"+entry.Reason] = true
			}
			return nil
		})
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, changeview.Diagnostic{Code: "baseline_unavailable", Message: err.Error()})
		} else {
			baseline = true
		}
	}
	result.Currency.BaselineAvailable = baseline
	result.Currency.References = []reconciliationReference{}
	for _, ref := range owned {
		resolution := resolver.Resolve(ctx, ref.Code, compared.Opening.HeadOID)
		row := reconciliationReference{ownedReference: ref, Head: resolution, Debt: "none", HistoryReasons: historical[ref.Owner]}
		prior, existed := baseRefs[reconciliationReferenceKey(ref)]
		if baseline && existed {
			row.Base = &prior
		}
		result.Currency.Total++
		if len(row.HistoryReasons) > 0 {
			result.Currency.Historical++
			row.Debt = "historical"
		}
		if resolution.Current() {
			result.Currency.Current++
			if resolution.Moved {
				result.Currency.Remapped++
			}
		} else {
			result.Currency.Stale++
			if len(row.HistoryReasons) > 0 {
				result.Currency.HistoricalStale++
				result.Currency.References = append(result.Currency.References, row)
				continue
			}
			switch {
			case !baseline:
				row.Debt = "baseline_unknown"
				result.Currency.Unknown++
			case !existed:
				row.Debt = "introduced"
				result.Currency.Introduced++
			case prior.Current():
				row.Debt = "regression"
				result.Currency.Regressions++
			default:
				row.Debt = "pre_existing"
				result.Currency.PreExisting++
			}
			task := reconciliationRoute(root, repo, compared.Opening.HeadOID, allowMismatch, document, tests, ref.Owner, ref.Kind)
			task.Debt, task.EvidenceFile, task.Reference = row.Debt, ref.EvidenceFile, ref.Index
			task.Because = []changeview.Cause{{Kind: "head_currency", Detail: resolution.Reason, Via: ref.Code.Location().String()}}
			if ref.Kind == "evidence" && len(task.Repair) == 0 {
				task.Repair = []grammar.Invocation{reconciliationInvoke("replace-coverage", root, repo, allowMismatch, grammar.V("record", ref.EvidenceFile), grammar.V("target", ref.Owner), grammar.V("commit", compared.Opening.HeadOID), grammar.V("path", ref.Code.Path), grammar.V("lines", ""), grammar.V("dry-run", "true")), grammar.MustInvoke("remove-coverage", root, grammar.V("record", ref.EvidenceFile), grammar.V("dry-run", "true"))}
				task.Guidance += " Replacement owns the whole evidence file; preserve other references in a reviewed --batch request. Remove only obsolete evidence."
			}
			result.Queue = append(result.Queue, task)
		}
		result.Currency.References = append(result.Currency.References, row)
	}

	for _, entry := range result.DocumentationCoverage.Areas.Implementation.UncoveredEntries {
		commit := compared.Opening.HeadOID
		if entry.Side == "old" {
			commit = compared.Opening.BaseOID
		}
		task := reconciliationTask{Resource: entry.Resource, Kind: "documentation_gap", Debt: "uncovered_change",
			Because:  []changeview.Cause{{Kind: "diff_coverage", Detail: entry.Reason + "; " + entry.Side + " lines " + entry.Lines + " " + entry.Event}},
			Guidance: "Read the changed code and current explanation, then choose narrow semantic owners. Supply an exact reference per owner; review-deck evidence does not fill this documentation gap. For a transaction-managed Item, use query slide and apply-slide instead of cover.",
			Inspect:  []grammar.Invocation{reconciliationQuery(root, repo, compared.Opening.HeadOID, "query gaps", grammar.V("against", compared.Opening.BaseOID), grammar.V("kind", "uncovered"))},
			Repair:   []grammar.Invocation{reconciliationInvoke("cover", root, repo, allowMismatch, grammar.V("target", ""), grammar.V("ref", ""), grammar.V("dry-run", "true"))},
		}
		task.Because[0].Via = commit + ":" + entry.Resource
		result.Queue = append(result.Queue, task)
	}
	// Keep non-reference debt visible even when there is no code diff.
	for _, entry := range result.HeadHealth.UncoveredEntries {
		if entry.Kind == "code_reference" || entry.Kind == "term" {
			continue
		} // exact references above own these rows
		task := reconciliationRoute(root, repo, compared.Opening.HeadOID, allowMismatch, document, tests, entry.Resource, entry.Kind)
		task.Debt = "new_or_changed"
		if !baseline {
			task.Debt = "baseline_unknown"
		} else if baseProblems[entry.Resource+"\x00"+entry.Reason] {
			task.Debt = "pre_existing"
		}
		task.Because = []changeview.Cause{{Kind: "head_health", Detail: entry.Reason}}
		result.Queue = append(result.Queue, task)
	}
	for _, affected := range layers.Affected {
		task := reconciliationRoute(root, repo, compared.Opening.HeadOID, allowMismatch, document, tests, affected.URN, affected.Kind)
		task.Debt, task.Because = "reassess", affected.Because
		result.Queue = append(result.Queue, task)
	}
	for _, changed := range layers.Changed {
		if changed.Kind != changeview.KindStory {
			continue
		}
		task := reconciliationRoute(root, repo, compared.Opening.HeadOID, allowMismatch, document, tests, changed.URN, changed.Kind)
		task.Debt = "reassess"
		task.Because = []changeview.Cause{{Kind: "requirement_change", Detail: "requirement " + changed.Change + "; reassess linked implementation and tests even when no code changed"}}
		result.Queue = append(result.Queue, task)
	}
	if layers.Saga.Head.Source == changeview.SideGit {
		result.Limits = append(result.Limits, "This report uses a historical Saga snapshot. Inspect the selected comparison; check out the intended Saga revision and rerun before authoring repairs. Query content and mutation commands otherwise address the working Saga.")
		for i := range result.Queue {
			result.Queue[i].Inspect = []grammar.Invocation{reconciliationQuery(root, repo, compared.Opening.HeadOID, "query layers", grammar.V("against", compared.Opening.BaseOID))}
			result.Queue[i].Repair = []grammar.Invocation{}
			result.Queue[i].Guidance = "Inspect this historical comparison, then check out the intended Saga revision and rerun reconciliation before repairing its content."
		}
	}
	sort.SliceStable(result.Queue, func(i, j int) bool {
		a, b := result.Queue[i], result.Queue[j]
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		if a.Debt != b.Debt {
			return a.Debt < b.Debt
		}
		if a.EvidenceFile != b.EvidenceFile {
			return a.EvidenceFile < b.EvidenceFile
		}
		return a.Reference < b.Reference
	})
	result.Recheck = []grammar.Invocation{grammar.MustInvoke("validate", root, grammar.V("json", "true")), reconciliationInvoke("reconcile", root, repo, allowMismatch, grammar.V("against", compared.Opening.BaseOID), grammar.V("head", compared.Opening.HeadOID), grammar.V("json", "true"))}
	after, err := reviewapp.Snapshot(ctx, root, opened.changes)
	if err != nil {
		return result, err
	}
	if after != snapshot {
		return result, fmt.Errorf("Saga changed during reconciliation; rerun against a stable snapshot")
	}
	return result, nil
}

// Match immutable reference identity and bytes, not merely its array position.
// New or revised evidence cannot inherit an old record's debt classification.
func reconciliationReferenceKey(ref ownedReference) string {
	data, _ := json.Marshal(ref)
	return string(data)
}

func reconciliationInvoke(command, root, repo string, mismatch bool, values ...grammar.Value) grammar.Invocation {
	shape, _ := grammar.Lookup(command)
	if _, ok := shape.Flag("repo"); ok && repo != "" {
		values = append(values, grammar.V("repo", repo))
	}
	if _, ok := shape.Flag("allow-repository-mismatch"); ok && mismatch {
		values = append(values, grammar.V("allow-repository-mismatch", "true"))
	}
	return grammar.MustInvoke(command, root, values...)
}

func reconciliationRoute(root, repo, head string, mismatch bool, document *saga.Saga, tests map[string]quality.TestCase, resource, kind string) reconciliationTask {
	task := reconciliationTask{Resource: resource, Kind: kind, Inspect: []grammar.Invocation{}, Repair: []grammar.Invocation{}, Guidance: "Read the affected explanation and linked intent; change it only if its meaning or evidence no longer holds."}
	query := func(command string, values ...grammar.Value) grammar.Invocation {
		return reconciliationQuery(root, repo, head, command, values...)
	}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			if resource != slide.Target && !strings.HasPrefix(resource, slide.Target+":item:") {
				continue
			}
			task.Inspect = append(task.Inspect, query("query slide", grammar.V("target", slide.Target)))
			if transactionManagedSlide(slide) {
				task.Guidance = "Read authoring_snapshot, all authoring_heads, Items, evidence and criterion_links. Reassess content; if needed submit complete desired state using apply-slide, preserving exact references and links. Conflicting heads require explicit reconciliation; do not use partial evidence writers."
				task.Repair = append(task.Repair, reconciliationInvoke("apply-slide", root, repo, mismatch, grammar.V("from", ""), grammar.V("dry-run", "true")))
			} else if kind != "evidence" {
				task.Repair = append(task.Repair, grammar.MustInvoke("revise-slide", root, grammar.V("slide", slide.Target), grammar.V("takeaway", "")))
			}
			return task
		}
	}
	switch kind {
	case "persona":
		task.Inspect = append(task.Inspect, query("query personas", grammar.V("persona", resource)))
	case "deck", "feature":
		task.Inspect = append(task.Inspect, query("query children", grammar.V("parent", resource)))
	case "story", "criterion":
		flag := "requirement"
		if kind == "criterion" {
			flag = "criterion"
		}
		task.Inspect = append(task.Inspect, query("query traceability", grammar.V(flag, resource)))
		task.Guidance = "Follow declared design, implementation and test paths. Read revised intent, reassess code, rerun relevant tests, and revise documentation or evidence only where needed. Empty paths are a visibility gap, not proof of no impact."
	case "relation":
		task.Inspect = append(task.Inspect, query("query relations", grammar.V("relation", resource)))
		task.Repair = append(task.Repair, grammar.MustInvoke("relation repin", root, grammar.V("relation", resource)))
		task.Guidance = "Read both endpoints and current pins. Repin with a new rationale only if the relationship remains true; otherwise supersede it through the public command."
	case "term":
		task.Inspect = append(task.Inspect, query("query terms", grammar.V("term", resource)))
		task.Repair = append(task.Repair, reconciliationInvoke("term revise", root, repo, mismatch, grammar.V("term", resource)))
	case "quality_evidence", "test_case", "test_run":
		test := strings.Split(strings.Split(resource, ":evidence:")[0], ":run:")[0]
		context := tests[test]
		inspectValues := []grammar.Value{grammar.V("json", "true"), grammar.V("head", head)}
		if context.Feature != "" {
			inspectValues = append(inspectValues, grammar.V("feature", context.Feature))
		}
		task.Inspect = append(task.Inspect, reconciliationInvoke("status", root, repo, mismatch, inspectValues...))
		runValues := []grammar.Value{grammar.V("test", test), grammar.V("commit", head)}
		for _, parent := range context.RunHeads {
			runValues = append(runValues, grammar.V("parent", parent))
		}
		task.Repair = append(task.Repair, reconciliationInvoke("quality evidence add", root, repo, mismatch, grammar.V("test", test)), reconciliationInvoke("quality run record", root, repo, mismatch, runValues...))
		task.Guidance = "Inspect the test and linked criteria, rerun verification, then append evidence and a run with the actual result. Do not rewrite historical evidence or infer success from fresh pins."
	case "claim":
		task.Inspect = append(task.Inspect, query("query claims"))
		claim := strings.Split(resource, ":claim:")
		if len(claim) == 2 {
			task.Inspect = append(task.Inspect, query("query verifications", grammar.V("claim", claim[1])))
			task.Repair = append(task.Repair, grammar.MustInvoke("verify-claim", root, grammar.V("claim", claim[1])))
		}
		task.Guidance = "Read the claim and its append-only verification history. Re-test its assertion; append a verification through verify-claim, preserving the original claim evidence."
	default:
		if strings.Contains(resource, ":fragment:") {
			fragment := strings.Split(resource, ":landmark:")[0]
			task.Inspect = append(task.Inspect, query("query fragment", grammar.V("target", fragment)))
		}
		task.Inspect = append(task.Inspect, query("query mappings", grammar.V("target", resource)))
	}
	return task
}

func reconciliationQuery(root, repo, head, command string, values ...grammar.Value) grammar.Invocation {
	operation := strings.TrimPrefix(command, "query ")
	values = append(values, grammar.V("saga", root), grammar.V("head", head))
	if repo != "" {
		values = append(values, grammar.V("repo", repo))
	}
	invocation := grammar.Invocation{Command: command, Status: grammar.StatusImplemented, Usage: queryUsage[operation], Arguments: []grammar.Argument{}, Inputs: []grammar.Argument{}, Argv: []string{"change-saga", "query", operation}}
	for _, value := range values {
		invocation.Arguments = append(invocation.Arguments, grammar.Argument{Flag: value.Flag, Value: value.Value})
		invocation.Argv = append(invocation.Argv, "--"+value.Flag, value.Value)
	}
	return invocation
}
