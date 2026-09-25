package draft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) Document {
	t.Helper()
	d := New("atomic", "Atomic diagram publication")
	b, e := os.ReadFile("../examples/scene.json")
	if e != nil {
		t.Fatal(e)
	}
	var els []Element
	if e = Decode(b, &els); e != nil {
		t.Fatal(e)
	}
	for _, el := range els {
		d.Elements[el.ID] = el
	}
	return d
}
func create(t *testing.T) (Store, Result) {
	t.Helper()
	s := Store{Root: t.TempDir()}
	d := fixture(t)
	r, e := s.Apply(Request{Version: 1, RequestID: "create", Expected: "absent", Source: &d}, false)
	if e != nil {
		t.Fatal(e)
	}
	return s, r
}
func TestMoveLeavesEdgesAndLinksUntouched(t *testing.T) {
	s, r := create(t)
	old, _ := s.Load()
	previousVisual, err := s.Visual(old)
	if err != nil {
		t.Fatal(err)
	}
	d := old.Source
	n := d.Elements["validator"]
	n.Evidence = []json.RawMessage{json.RawMessage(`{"references":[{"path":"validate.go","start":12,"end":18}]}`)}
	n.CriterionLinks = []json.RawMessage{json.RawMessage(`{"criterion":"urn:fixture:criterion:valid","story_revision":"urn:fixture:revision:r1"}`)}
	d.Elements[n.ID] = n
	moved, e := Edit(d, []Operation{{Op: "move", ID: n.ID, DX: 20}, {Op: "update", ID: n.ID, Set: json.RawMessage(`{"label":"Validate batch","style":"normal"}`)}})
	if e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(d.Elements["publish"])
	after, _ := json.Marshal(moved.Elements["publish"])
	if !bytes.Equal(before, after) {
		t.Fatal("edge changed")
	}
	m := moved.Elements[n.ID]
	if m.X != n.X+20 || string(m.Evidence[0]) != string(n.Evidence[0]) || string(m.CriterionLinks[0]) != string(n.CriterionLinks[0]) {
		t.Fatal("links or move incorrect")
	}
	_, e = s.Apply(Request{Version: 1, RequestID: "move", Expected: r.Snapshot, Operations: []Operation{{Op: "move", ID: n.ID, DX: 20}}}, false)
	if e != nil {
		t.Fatal(e)
	}
	rec, _ := s.Load()
	svg, e := s.Visual(rec)
	if e != nil {
		t.Fatal(e)
	}
	edge := regexp.MustCompile(`(?s)<g id="publish".*?</g>`)
	if len(edge.Find(svg)) == 0 || !bytes.Equal(edge.Find(svg), edge.Find(previousVisual)) {
		t.Fatal("rendered edge geometry changed after node move")
	}
	broken := bytes.Replace(svg, []byte(`id="validator"`), []byte(`id="wrong"`), 1)
	if CheckSelectors(rec.Source, broken) == nil {
		t.Fatal("broken selector accepted")
	}
}
func TestAtomicBatchRetryRebuild(t *testing.T) {
	s, r := create(t)
	req := Request{Version: 1, RequestID: "edit", Expected: r.Snapshot, Operations: []Operation{{Op: "move", ID: "author", DX: 5}, {Op: "update", ID: "validator", Set: json.RawMessage(`{"label":"Validate batch"}`)}}}
	s.BeforePublish = func() error { return errors.New("injected before publish") }
	if _, e := s.Apply(req, false); e == nil {
		t.Fatal("fault missed")
	}
	rec, _ := s.Load()
	if rec.Snapshot != r.Snapshot {
		t.Fatal("partial source visible")
	}
	if _, e := s.Visual(rec); e != nil {
		t.Fatal(e)
	}
	s.BeforePublish = nil
	next, e := s.Apply(req, false)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.Apply(req, false)
	if e != nil || !replay.Replayed || replay.Snapshot != next.Snapshot {
		t.Fatal("bad replay", e)
	}
	req.Operations[0].DX = 9
	if _, e := s.Apply(req, false); e == nil {
		t.Fatal("request-id collision allowed")
	}
	req.RequestID = "stale"
	if _, e := s.Apply(req, false); e == nil {
		t.Fatal("stale writer allowed")
	}
	rec, _ = s.Load()
	p := s.path("visuals", rec.SVGHash)
	original, _ := os.ReadFile(p)
	corrupt := []byte("external edit")
	if e := os.WriteFile(p, corrupt, 0644); e != nil {
		t.Fatal(e)
	}
	check, _ := s.Check()
	if check["visual_ok"] != false || check["rebuildable"] != true {
		t.Fatal(check)
	}
	result, e := s.Rebuild()
	if e != nil {
		t.Fatal(e)
	}
	saved, _ := os.ReadFile(result["recovered_path"].(string))
	rebuilt, _ := os.ReadFile(p)
	if !bytes.Equal(saved, corrupt) || !bytes.Equal(original, rebuilt) {
		t.Fatal("lost edit or nondeterministic rebuild")
	}
	if e := os.Remove(s.path("inputs", rec.Source.Assets["font:go-regular"].Digest)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Rebuild(); e == nil {
		t.Fatal("missing pinned font substituted")
	}
}
func TestDependencyRemovalAndRollback(t *testing.T) {
	s, r := create(t)
	req := Request{Version: 1, RequestID: "remove", Expected: r.Snapshot, Operations: []Operation{{Op: "move", ID: "author", DX: 55}, {Op: "remove", ID: "validator"}}}
	if _, e := s.Apply(req, false); e == nil {
		t.Fatal("dependent removal allowed")
	}
	rec, _ := s.Load()
	if rec.Snapshot != r.Snapshot {
		t.Fatal("first operation leaked")
	}
	req.Operations = []Operation{{Op: "remove", ID: "validator", Cascade: true}}
	if _, e := s.Apply(req, false); e != nil {
		t.Fatal(e)
	}
	rec, _ = s.Load()
	for _, id := range []string{"validator", "batch", "publish", "failure"} {
		if _, ok := rec.Source.Elements[id]; ok {
			t.Fatal("dependent remains", id)
		}
	}
}
func TestGroupMoveOnlyChangesParentTransform(t *testing.T) {
	s, r := create(t)
	old, _ := s.Load()
	_, e := s.Apply(Request{Version: 1, RequestID: "group", Expected: r.Snapshot, Operations: []Operation{{Op: "move", ID: "revision-boundary", DX: 25, DY: 30}}}, false)
	if e != nil {
		t.Fatal(e)
	}
	rec, _ := s.Load()
	if rec.Source.Elements["revision-area"].X != old.Source.Elements["revision-area"].X || rec.Source.Elements["revision-boundary"].X != 25 {
		t.Fatal("incorrect local transform")
	}
	a, _ := json.Marshal(rec.Source.Elements["publish"])
	b, _ := json.Marshal(old.Source.Elements["publish"])
	if !bytes.Equal(a, b) {
		t.Fatal("unrelated edge moved")
	}
}
func TestConcurrentWritersOneWinner(t *testing.T) {
	s, r := create(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := s.Apply(Request{Version: 1, RequestID: id, Expected: r.Snapshot, Operations: []Operation{{Op: "move", ID: "author", DX: 1}}}, false)
			errs <- e
		}(id)
	}
	wg.Wait()
	close(errs)
	n := 0
	for e := range errs {
		if e == nil {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d winners", n)
	}
}
func TestMergeTextAndSemanticCorrectness(t *testing.T) {
	store, _ := create(t)
	record, _ := store.Load()
	base := record.Source
	ours := clone(base)
	theirs := clone(base)
	a := ours.Elements["author"]
	a.Label = "Author edits"
	ours.Elements[a.ID] = a
	b := theirs.Elements["published"]
	b.Label = "New snapshot"
	theirs.Elements[b.ID] = b
	merged, c, e := Merge(base, ours, theirs)
	if e != nil || len(c) > 0 || merged.Elements[a.ID].Label != a.Label || merged.Elements[b.ID].Label != b.Label {
		t.Fatal(c, e)
	}
	git, e := exec.LookPath("git")
	if e != nil {
		t.Skip("git unavailable")
	}
	dir := t.TempDir()
	paths := []string{}
	for i, d := range []Document{ours, base, theirs} {
		b, _ := json.MarshalIndent(d, "", "  ")
		p := filepath.Join(dir, []string{"ours", "base", "theirs"}[i]+".json")
		if e := os.WriteFile(p, b, 0644); e != nil {
			t.Fatal(e)
		}
		paths = append(paths, p)
	}
	out, e := exec.Command(git, "merge-file", "-p", paths[0], paths[1], paths[2]).Output()
	if e != nil {
		t.Fatal("different-element text merge conflict", e)
	}
	var textMerged Document
	if e := Decode(out, &textMerged); e != nil {
		t.Fatal(e)
	}
	if Snapshot(textMerged) != Snapshot(merged) {
		t.Fatal("text/semantic merge disagree")
	}
	theirs = clone(base)
	b = theirs.Elements["author"]
	b.X += 10
	theirs.Elements[b.ID] = b
	_, c, e = Merge(base, ours, theirs)
	if e != nil || len(c) != 1 || c[0] != "author" {
		t.Fatal("same-element conflict missed", c, e)
	}
	base = New("dependency", "Dependency")
	for _, id := range []string{"a", "b"} {
		base.Elements[id] = Element{ID: id, Kind: "node", Shape: "rect", W: 100, H: 100, Style: "normal"}
	}
	ours = clone(base)
	delete(ours.Elements, "a")
	theirs = clone(base)
	theirs.Elements["edge"] = Element{ID: "edge", Kind: "edge", From: "a", To: "b", Points: []Point{{0, 0}, {100, 0}}, Style: "normal"}
	_, c, e = Merge(base, ours, theirs)
	if e != nil || len(c) == 0 || !strings.HasPrefix(c[0], "semantic-dependency") {
		t.Fatal("dangling-edge merge accepted", c, e)
	}
}
func TestStrictInputAndOverflow(t *testing.T) {
	if Decode([]byte(`{"version":1,"unknown":true}`), &Request{}) == nil {
		t.Fatal("unknown field accepted")
	}
	s, r := create(t)
	for i, patch := range []string{`{"label":"An enormous label that cannot fit into its explicit width"}`, `{"nonsense":1}`, `{"id":"new-id"}`, `{"evidence":[]}`} {
		_, e := s.Apply(Request{Version: 1, RequestID: []string{"long", "unknown", "identity", "evidence"}[i], Expected: r.Snapshot, Operations: []Operation{{Op: "update", ID: "author", Set: json.RawMessage(patch)}}}, false)
		if e == nil {
			t.Fatal("bad patch accepted", patch)
		}
	}
	if _, e := safeFragment(`<script>alert(1)</script>`); e == nil {
		t.Fatal("script accepted")
	}
}

func TestExplicitStylesWrappingAlignmentAndLinkedRemoval(t *testing.T) {
	s, r := create(t)
	style := Style{Fill: "#ffeeee", Stroke: "#ff0000", Ink: "#550000", StrokeWidth: 3, FontSize: 18}
	text := Element{ID: "caption", Kind: "text", Label: "An explicitly wrapped caption with a fixed box", X: 65, Y: 565, W: 300, H: 60, Style: "custom", Wrap: true}
	_, err := s.Apply(Request{Version: 1, RequestID: "style", Expected: r.Snapshot, Operations: []Operation{{Op: "style", ID: "custom", Style: &style}, {Op: "add", Element: &text}, {Op: "align", IDs: []string{"author", "reject"}, Axis: "x", Value: 70}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := s.Load()
	if rec.Source.Elements["reject"].X != 70 {
		t.Fatal("alignment failed")
	}
	d := rec.Source
	e := d.Elements["author"]
	e.Evidence = []json.RawMessage{json.RawMessage(`{"fixture":true}`)}
	d.Elements[e.ID] = e
	if _, err := Edit(d, []Operation{{Op: "remove", ID: e.ID, Cascade: true}}); err == nil {
		t.Fatal("linked removal allowed")
	}
}

func TestDescriptionAndSharedVisualInputs(t *testing.T) {
	s, r := create(t)
	rec, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Reusing an icon must not duplicate SVG IDs. Changing a named style must
	// report every directly affected element, even when element records match.
	style := rec.Source.Styles["normal"]
	style.Fill = "#f1f5f9"
	result, err := s.Apply(Request{Version: 1, RequestID: "shared", Expected: r.Snapshot, Operations: []Operation{
		{Op: "update", ID: "author", Set: json.RawMessage(`{"icon":"lucide:check"}`)},
		{Op: "style", ID: "normal", Style: &style},
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range result.Changed {
		if id == "published" {
			found = true
		}
	}
	if !found {
		t.Fatal("style consumer omitted from changed IDs")
	}
	rec, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Visual(rec); err != nil {
		t.Fatal(err)
	}
	summary := Describe(rec.Source, 0, 100)
	b, _ := json.Marshal(summary)
	for _, text := range []string{"Selectors, references, asset hashes", "Reject invalid batches before publication.", `"from":"validator"`, `"to":"published"`} {
		if !bytes.Contains(b, []byte(text)) {
			t.Fatalf("description omitted meaning: %s", text)
		}
	}
	if summary.Reconstructable || summary.Total != 7 {
		t.Fatal(summary)
	}
	text := summary.Text()
	for _, line := range []string{
		`Diagram: atomic "Atomic diagram publication"`,
		"Snapshot: " + rec.Snapshot,
		`  validator "Validate + stage" icon=lucide:check`,
		"    detail: Selectors, references, asset hashes",
		"    description: Reject invalid batches before publication.",
		`  publish: validator -> published "publish"`,
		"Showing 1-7 of 7.",
	} {
		if !strings.Contains(text, line+"\n") {
			t.Fatalf("text description omitted %q:\n%s", line, text)
		}
	}
	if strings.Contains(text, "revision-area") || len(text) >= len(b) {
		t.Fatalf("text description should omit decorative elements and be smaller than JSON:\n%s", text)
	}
	first := Describe(rec.Source, 0, 2)
	if !first.HasMore || first.NextOffset != 2 || !strings.Contains(first.Text(), "Showing 1-2 of 7; continue with --offset 2.") {
		t.Fatal(first)
	}
	if !strings.Contains(Describe(rec.Source, 50, 2).Text(), "No elements at offset 7 of 7.") {
		t.Fatal("past-the-end page should say it is empty")
	}
}

func TestDescribeTextQuotesStructureBreakingProse(t *testing.T) {
	d := New("quoting", "Line\nbreak")
	d.Elements["loose"] = Element{ID: "loose", Kind: "edge", Style: "normal", Label: `say "hi"`, Detail: "two\nlines", Description: " padded"}
	text := Describe(d, 0, 10).Text()
	for _, line := range []string{
		`Diagram: quoting "Line\nbreak"`,
		`  loose: (unconnected) -> (unconnected) "say \"hi\""`,
		`    detail: "two\nlines"`,
		`    description: " padded"`,
	} {
		if !strings.Contains(text, line+"\n") {
			t.Fatalf("missing %q:\n%s", line, text)
		}
	}
}

func TestSemanticElementsCannotHideInDecorativeGroups(t *testing.T) {
	d := New("hidden", "Hidden")
	d.Elements["frame"] = Element{ID: "frame", Kind: "group", Style: "normal", Decorative: true}
	d.Elements["label"] = Element{ID: "label", Kind: "text", Style: "normal", Label: "Seen", Parent: "frame", W: 100, H: 30}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "semantic element inside decorative group frame") {
		t.Fatalf("expected decorative-group refusal, got %v", err)
	}
	e := d.Elements["label"]
	e.Decorative = true
	d.Elements["label"] = e
	if err := d.Validate(); err != nil {
		t.Fatalf("decorative children of decorative groups stay valid: %v", err)
	}
}

func TestStricterRulesStillAllowReadAndRepair(t *testing.T) {
	s, r := create(t)
	add := func(e Element) Operation { return Operation{Op: "add", Element: &e} }
	_, err := s.Apply(Request{Version: 1, RequestID: "frame", Expected: r.Snapshot, Operations: []Operation{
		add(Element{ID: "frame", Kind: "group", Style: "normal"}),
		add(Element{ID: "caption", Kind: "text", Style: "normal", Label: "Seen", Parent: "frame", X: 65, Y: 600, W: 200, H: 30}),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a record published before the decorative-group rule existed.
	rec, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	frame := rec.Source.Elements["frame"]
	frame.Decorative = true
	rec.Source.Elements["frame"] = frame
	rec.Snapshot = Snapshot(rec.Source)
	b, _ := json.Marshal(rec)
	if err = os.WriteFile(filepath.Join(s.Root, "current.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(); err != nil {
		t.Fatalf("older record must stay readable: %v", err)
	}
	report, err := s.Check()
	if err != nil || report["rebuildable"] != false || !strings.Contains(fmt.Sprint(report["validation_error"]), "inside decorative group frame") {
		t.Fatalf("check must report the rule violation: %v %v", report, err)
	}
	if _, err = s.Apply(Request{Version: 1, RequestID: "unrelated", Expected: rec.Snapshot, Operations: []Operation{{Op: "move", ID: "author", DX: 1}}}, false); err == nil {
		t.Fatal("publishing must still refuse a result that breaks the rule")
	}
	if _, err = s.Apply(Request{Version: 1, RequestID: "repair", Expected: rec.Snapshot, Operations: []Operation{{Op: "update", ID: "frame", Set: json.RawMessage(`{"decorative":false}`)}}}, false); err != nil {
		t.Fatalf("repair edit must publish: %v", err)
	}
}
