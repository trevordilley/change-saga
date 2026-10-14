package diagram

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestViewportPreservesRenderedGeometry(t *testing.T) {
	d := sample()
	before := mustRender(t, d)
	d.Viewport = &Box{X: 20, Y: 20, Width: 200, Height: 150}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	after := mustRender(t, d)
	if !bytes.Equal(before, after) {
		t.Fatal("opening camera changed historical renderer bytes")
	}
	encoded, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(value); err != nil {
		t.Fatal(err)
	}
	for _, box := range []Box{{Width: 0, Height: 10}, {X: -1, Width: 10, Height: 10}, {Width: d.Width + 1, Height: 10}, {Width: 10, Height: d.Height + 1}} {
		d.Viewport = &box
		if d.Validate() == nil {
			t.Errorf("accepted invalid opening region: %+v", box)
		}
	}
}

func TestCanvasEditViewportAndReset(t *testing.T) {
	d := sample()
	changed, _, err := Edit(d, []Operation{{Op: "canvas", Set: json.RawMessage(`{"viewport":{"x":0,"y":0,"width":200,"height":150}}`)}})
	if err != nil || changed.Viewport == nil {
		t.Fatalf("set: %v", err)
	}
	reset, _, err := Edit(changed, []Operation{{Op: "canvas", Set: json.RawMessage(`{"viewport":null}`)}})
	if err != nil || reset.Viewport != nil {
		t.Fatalf("reset: %v", err)
	}
}
