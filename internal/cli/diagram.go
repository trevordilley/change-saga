package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Diagram dispatches the diagram command family: compact reading views,
// targeted edits to a diagram-sourced slide, bundled icon discovery, and
// render drift checks.
func Diagram(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("diagram", []string{"describe", "get", "edit", "icons", "check"}, out)
	}
	switch args[0] {
	case "describe":
		return diagramDescribe(args[1:], out)
	case "get":
		return diagramGet(args[1:], out)
	case "edit":
		return diagramEdit(ctx, args[1:], out)
	case "icons":
		return diagramIcons(args[1:], out)
	case "check":
		return diagramCheck(args[1:], out)
	default:
		return fmt.Errorf("usage: %s", commandUsage["diagram"])
	}
}

// DiagramEditResult is the published slide transaction plus the diagram
// elements whose rendering the operations may have changed.
type DiagramEditResult struct {
	SlideTransactionResult
	ChangedElements []string `json:"changed_elements"`
}

// diagramSlideState is one diagram-sourced revision of a slide and the
// complete Items, evidence, and criterion links published with it.
type diagramSlideState struct {
	deck     *saga.Deck
	slide    *saga.Slide
	revision saga.SlideTransactionRevision
	source   diagram.Document
}

// loadDiagramRevision reads the revision named by snapshot, or the current
// revision when snapshot is empty. Reading an older revision keeps a retried
// edit idempotent: the same operations on the same base reproduce the same
// complete payload, which apply-slide replays instead of refusing.
func loadDiagramRevision(root, target, snapshot string) (diagramSlideState, error) {
	document, _, err := saga.Load(root)
	if err != nil {
		return diagramSlideState{}, err
	}
	slide := findSlide(document, target)
	if slide == nil {
		return diagramSlideState{}, fmt.Errorf("slide %q does not exist", target)
	}
	var deck *saga.Deck
	for _, candidate := range allDecks(document) {
		for _, owned := range candidate.Slides {
			if owned == slide {
				deck = candidate
			}
		}
	}
	if !transactionManagedSlide(slide) || slide.Diagram == nil || deck == nil {
		return diagramSlideState{}, fmt.Errorf("slide %s has no diagram source; publish one with `change-saga apply-slide` using a \"diagram\" instead of an \"asset\"", slide.Target)
	}
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(document.Root, filepath.FromSlash(slide.Path)), &record); err != nil {
		return diagramSlideState{}, err
	}
	if snapshot == "" {
		snapshot = slide.AuthoringSnapshot
	}
	for _, revision := range record.Revisions {
		if revision.Snapshot != snapshot {
			continue
		}
		if revision.Diagram == nil {
			return diagramSlideState{}, fmt.Errorf("revision %s of %s has no diagram source", snapshot, slide.Target)
		}
		data, err := os.ReadFile(filepath.Join(deck.Directory, revision.Diagram.Source))
		if err != nil {
			return diagramSlideState{}, err
		}
		source, err := diagram.Decode(data)
		if err != nil {
			return diagramSlideState{}, fmt.Errorf("diagram source %s: %w", revision.Diagram.Source, err)
		}
		return diagramSlideState{deck: deck, slide: slide, revision: revision, source: source}, nil
	}
	return diagramSlideState{}, fmt.Errorf("expected snapshot %s is not a revision of %s; read the current authoring_snapshot with `change-saga query diagram`", snapshot, slide.Target)
}

// requestFromRevision rebuilds the complete apply-slide request that
// republishes revision with a replacement diagram.
func requestFromRevision(state diagramSlideState, requestID, expected string, source diagram.Document) SlideTransactionRequest {
	value := state.revision.Slide
	request := SlideTransactionRequest{
		Version: saga.SlideTransactionVersion, Operation: "update", RequestID: requestID, Deck: state.deck.ID, ExpectedSnapshot: expected,
		Slide: SlideTransactionSlide{
			ID: value.ID, Title: value.Title, Rank: value.Rank, Section: value.Section, Intent: value.Intent, Layout: value.Layout,
			MediaType: value.MediaType, Takeaway: value.Takeaway, ReadingOrder: append([]string{}, value.ReadingOrder...), ExceptionRationale: value.ExceptionRationale,
		},
		Diagram: &source,
		Items:   []SlideTransactionItemRequest{},
	}
	for _, item := range state.revision.Items {
		manifest := item.Item
		request.Items = append(request.Items, SlideTransactionItemRequest{
			ID: manifest.ID, Rank: manifest.Rank, Kind: manifest.Kind, Label: manifest.Label, Description: manifest.Description,
			Selector: manifest.Selector, Hotspot: manifest.Hotspot, About: manifest.About, Body: manifest.Body, Placement: manifest.Placement, Leader: manifest.Leader,
			Evidence: append([]saga.CodeFile{}, item.Evidence...), CriterionLinks: append([]saga.CriterionLink{}, item.CriterionLinks...),
		})
	}
	return request
}

func diagramEdit(ctx context.Context, args []string, out io.Writer) error {
	name := "diagram edit"
	flags := commandFlags(name, commandUsage[name], out)
	target := flags.String("slide", "", "slide target whose diagram to edit")
	expected := flags.String("expected", "", "exact authoring snapshot the operations apply to")
	requestID := flags.String("request-id", "", "idempotency key; retrying the same edit is a no-op")
	from := flags.String("from", "", "JSON array of diagram operations, or - for standard input; [] re-renders unchanged source")
	repo := flags.String("repo", "", "code checkout used to re-verify the slide's exact evidence digests")
	dryRun := flags.Bool("dry-run", false, "validate and return the semantic diff without publishing")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *target == "" || *expected == "" || *requestID == "" || *from == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	data, err := readInputFile(*from)
	if err != nil {
		return err
	}
	operations, err := diagram.DecodeOperations(data)
	if err != nil {
		return fmt.Errorf("read diagram operations: %w", err)
	}
	root := flags.Arg(0)
	state, err := loadDiagramRevision(root, *target, *expected)
	if err != nil {
		return err
	}
	edited, changed, err := diagram.Edit(state.source, operations)
	if err != nil {
		return err
	}
	base, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := ApplySlideTransaction(ctx, root, base, *repo, requestFromRevision(state, *requestID, *expected, edited), *dryRun)
	if err != nil {
		return err
	}
	edit := DiagramEditResult{SlideTransactionResult: result, ChangedElements: changed}
	if *jsonOutput {
		return writeJSON(out, edit)
	}
	verb := "Applied"
	if result.DryRun {
		verb = "Would apply"
	} else if result.Replayed {
		verb = "Replayed"
	}
	fmt.Fprintf(out, "%s diagram edit to %s\nSnapshot: %s\nChanged elements: %s\n", verb, result.Target, result.Snapshot, strings.Join(changed, ", "))
	return nil
}

func readInputFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(io.LimitReader(os.Stdin, 17<<20))
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--from must name a regular JSON file")
	}
	return os.ReadFile(path)
}

func diagramIcons(args []string, out io.Writer) error {
	name := "diagram icons"
	flags := commandFlags(name, commandUsage[name], out)
	query := flags.String("query", "", "substring the icon name must contain")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	names := diagram.Icons(*query)
	if *jsonOutput {
		return writeJSON(out, map[string]any{"pack": "lucide@" + diagram.LucideRevision, "icons": names})
	}
	for _, icon := range names {
		fmt.Fprintln(out, icon)
	}
	return nil
}

// DiagramCheckSlide reports whether a slide's current diagram source still
// renders to its published SVG with this binary's renderer.
type DiagramCheckSlide struct {
	Target        string `json:"target"`
	Renderer      string `json:"renderer"`
	Current       bool   `json:"current"`
	Problem       string `json:"problem,omitempty"`
	RepairCommand string `json:"repair_command,omitempty"`
}

func diagramCheck(args []string, out io.Writer) error {
	name := "diagram check"
	flags := commandFlags(name, commandUsage[name], out)
	target := flags.String("slide", "", "check only this slide")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	document, _, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	report := []DiagramCheckSlide{}
	for _, deck := range allDecks(document) {
		for _, slide := range deck.Slides {
			if slide.Diagram == nil || (*target != "" && findSlide(document, *target) != slide) {
				continue
			}
			report = append(report, checkDiagramSlide(deck, slide))
		}
	}
	if *target != "" && len(report) == 0 {
		return fmt.Errorf("slide %q does not exist or has no diagram source", *target)
	}
	stale := 0
	for _, entry := range report {
		if !entry.Current {
			stale++
		}
	}
	if *jsonOutput {
		if err := writeJSON(out, map[string]any{"ok": stale == 0, "renderer": diagram.Renderer, "slides": report}); err != nil {
			return err
		}
	} else {
		for _, entry := range report {
			if entry.Current {
				fmt.Fprintf(out, "current  %s\n", entry.Target)
			} else {
				fmt.Fprintf(out, "stale    %s: %s\n", entry.Target, entry.Problem)
			}
		}
		fmt.Fprintf(out, "%d diagram slide(s) checked, %d stale\n", len(report), stale)
	}
	if stale > 0 {
		return &StatusError{Code: 3}
	}
	return nil
}

func checkDiagramSlide(deck *saga.Deck, slide *saga.Slide) DiagramCheckSlide {
	entry := DiagramCheckSlide{Target: slide.Target, Renderer: slide.Diagram.Renderer}
	fail := func(problem string) DiagramCheckSlide {
		entry.Problem = problem
		entry.RepairCommand = fmt.Sprintf("echo '[]' | change-saga diagram edit --slide %s --expected %s --request-id RERENDER-ID --from - <saga>", slide.Target, slide.AuthoringSnapshot)
		return entry
	}
	data, err := os.ReadFile(filepath.Join(deck.Directory, slide.Diagram.Source))
	if err != nil {
		return fail(err.Error())
	}
	source, err := diagram.Decode(data)
	if err != nil {
		return fail(err.Error())
	}
	rendered, err := diagram.Render(source, diagram.Options{Title: slide.Title, Description: slide.Takeaway})
	if err != nil {
		return fail("source no longer renders: " + err.Error())
	}
	published, err := os.ReadFile(filepath.Join(deck.Directory, slide.Entrypoint))
	if err != nil {
		return fail(err.Error())
	}
	if !bytes.Equal(rendered, published) {
		return fail(fmt.Sprintf("published SVG differs from %s output of its source", diagram.Renderer))
	}
	entry.Current = true
	return entry
}
