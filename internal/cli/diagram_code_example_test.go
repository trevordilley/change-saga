package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestApplySlideCodeExampleKeepsItemEvidence(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	d := diagram.New()
	d.Elements = []diagram.Element{{ID: "worker", Kind: "code", Label: "Invoke the worker", Language: "Go", Code: "result, err := worker.Run(ctx, input)\nif err != nil {\n    return err\n}", X: 80, Y: 120, Width: 1100, Height: 400, Style: "code", LineNumbers: true, HighlightLines: []int{1}}}
	request := diagramSlideRequest(t, repo, base, commit, sagaID, "code-example", "create", "absent", d)
	request.Items[0].Kind = "example"
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load %v; %#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	if slide.Items[0].Kind != "example" || len(slide.Items[0].Code) == 0 {
		t.Fatal("example lost its exact implementation evidence")
	}
	svg, err := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(svg, []byte(`data-diagram-kind="code"`)) {
		t.Fatal("code element was not published")
	}
	var out bytes.Buffer
	if err := Diagram(context.Background(), []string{"describe", "--slide", created.Target, root}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "worker.Run(ctx, input)") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := Diagram(context.Background(), []string{"check", "--slide", created.Target, root}, &out); err != nil {
		t.Fatalf("generated source drift: %v %s", err, out.String())
	}
}
