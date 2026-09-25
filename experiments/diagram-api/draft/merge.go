package draft

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Merge is a conservative three-way merge by element identity. Same-element
// edits conflict even if Git can merge their different JSON fields cleanly.
func Merge(base, ours, theirs Document) (Document, []string, error) {
	for _, d := range []Document{base, ours, theirs} {
		if err := d.Validate(); err != nil {
			return Document{}, nil, err
		}
	}
	if base.ID != ours.ID || base.ID != theirs.ID {
		return Document{}, nil, fmt.Errorf("diagram identity mismatch")
	}
	equal := func(a, b any) bool { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return bytes.Equal(x, y) }
	bHead, oHead, tHead := clone(base), clone(ours), clone(theirs)
	bHead.Elements = nil
	oHead.Elements = nil
	tHead.Elements = nil
	result := clone(ours)
	conflicts := []string{}
	if !equal(oHead, tHead) && !equal(oHead, bHead) && !equal(tHead, bHead) {
		conflicts = append(conflicts, "document-properties/styles/assets")
	} else if equal(oHead, bHead) {
		result = clone(theirs)
	}
	result.Elements = map[string]Element{}
	ids := map[string]bool{}
	for _, d := range []Document{base, ours, theirs} {
		for id := range d.Elements {
			ids[id] = true
		}
	}
	for _, id := range keys(ids) {
		b, bok := base.Elements[id]
		o, ook := ours.Elements[id]
		t, tok := theirs.Elements[id]
		same := func(x Element, xok bool, y Element, yok bool) bool { return xok == yok && equal(x, y) }
		switch {
		case same(o, ook, t, tok):
			if ook {
				result.Elements[id] = o
			}
		case same(o, ook, b, bok):
			if tok {
				result.Elements[id] = t
			}
		case same(t, tok, b, bok):
			if ook {
				result.Elements[id] = o
			}
		default:
			conflicts = append(conflicts, id)
		}
	}
	if len(conflicts) > 0 {
		return Document{}, conflicts, nil
	}
	if err := result.Validate(); err != nil {
		return Document{}, []string{"semantic-dependency: " + err.Error()}, nil
	}
	return result, nil, nil
}
