package saga

import (
	"fmt"
	"path/filepath"
	"testing"
)

// SPEC.md resolves thread state, approval state, and reviewed state from the
// latest record by created_at. Two records may legitimately share a timestamp —
// hand-authored history often has second granularity — so "latest" is only well
// defined once ties break on the record id. Before that rule existed, the file
// name decided the answer, which meant two checkouts of the same commit could
// disagree about whether a thread was resolved.

func TestFragmentsAndSectionsSortStablyOnEqualOrder(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"zebra", "alpha", "middle"} {
		files["one.chapter/"+name+".fragment/fragment.json"] = fmt.Sprintf(`{"version":2,"id":%q,"title":%q,"media_type":"text/markdown","entrypoint":"content.md","order":5}`, name, name)
		files["one.chapter/"+name+".fragment/content.md"] = "Body.\n"
		files["one.chapter/"+name+"/section.json"] = fmt.Sprintf(`{"version":2,"id":%q,"title":%q,"order":5}`, "s-"+name, name)
	}
	files["one.chapter/chapter.json"] = `{"version":2,"id":"one","title":"One"}`
	root := buildSaga(t, files)
	for round := 0; round < 5; round++ {
		document, validation, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if !validation.Valid {
			t.Fatalf("%#v", validation.Issues)
		}
		chapter := document.Section.Children[0]
		var fragments, sections string
		for _, fragment := range chapter.Fragments {
			fragments += filepath.Base(fragment.Path) + ","
		}
		for _, child := range chapter.Children {
			sections += filepath.Base(child.Path) + ","
		}
		if fragments != "alpha.fragment,middle.fragment,zebra.fragment," {
			t.Fatalf("equal-order fragments must fall back to path order, got %q", fragments)
		}
		if sections != "alpha,middle,zebra," {
			t.Fatalf("equal-order sections must fall back to path order, got %q", sections)
		}
	}
}
