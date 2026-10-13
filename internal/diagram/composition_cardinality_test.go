package diagram

import (
	"strings"
	"testing"
)

func TestCompositionPreservesAssociationEndsThroughOverviewAndDescribe(t *testing.T) {
	child := compositionDoc(200, 150, compositionNode("record", 20, 30))
	left := compositionFrame("left", "erd:left", 40, 60, 400, 300)
	right := compositionFrame("right", "erd:right", 800, 60, 200, 150)
	link := Element{ID: "belongs", Kind: "connection", From: "left/record", To: "right/record", Style: "normal", Tail: "zero-or-many", Head: "only-one", HeadSize: 100,
		Source: &SourceReference{Target: "data-entity:record", Revision: "entity-r1", Relationship: "belongs"}}
	c, err := ResolveComposition(compositionDoc(1200, 600, left, right, link), func(SourceReference) (ResolvedSource, error) {
		return ResolvedSource{Document: &child}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := c.Render(Options{Title: "Association"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"detail", "overview"} {
		for _, end := range []struct{ attr, name string }{{"start", "tail"}, {"end", "head"}} {
			id := "diagram-marker-diagram-composition-belongs-" + mode + "-" + end.name
			if !strings.Contains(string(svg), `marker-`+end.attr+`="url(#`+id+`)"`) {
				t.Errorf("%s lost %s cardinality marker", mode, end.name)
			}
		}
	}
	focus, err := c.Describe("left/record")
	if err != nil {
		t.Fatal(err)
	}
	if len(focus.Connections) != 1 {
		t.Fatalf("incident associations: %d", len(focus.Connections))
	}
	got := focus.Connections[0]
	if got.From != link.From || got.To != link.To || got.Element.Tail != link.Tail || got.Element.Head != link.Head || *got.Source != *link.Source {
		t.Fatalf("focus changed semantic endpoints, multiplicity or evidence: %+v", got)
	}
}
