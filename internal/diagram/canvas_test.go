package diagram

import (
	"encoding/json"
	"testing"
)

func TestSpaciousCanvasRuntimeAndSchemaBoundaries(t *testing.T) {
	schema := compileSchema(t)
	for _, tc := range []struct {
		width, height float64
		valid         bool
	}{
		{14400, 24300, true},
		{MaxCanvasDimension, MaxCanvasDimension, true},
		{MaxCanvasDimension + 1, 100, false},
		{100, MaxCanvasDimension + 1, false},
		{0, 100, false},
		{100, -1, false},
	} {
		doc := New()
		doc.Width, doc.Height = tc.width, tc.height
		if valid := doc.Validate() == nil; valid != tc.valid {
			t.Errorf("runtime canvas %gx%g: valid=%v", tc.width, tc.height, valid)
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		if valid := schema.Validate(value) == nil; valid != tc.valid {
			t.Errorf("schema canvas %gx%g: valid=%v", tc.width, tc.height, valid)
		}
	}
}
