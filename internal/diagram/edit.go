package diagram

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Operation is one explicit edit. Operations never lay out, resize, or reroute
// anything they do not name: moving a node leaves its edges where they are.
type Operation struct {
	Op      string          `json:"op"`
	ID      string          `json:"id,omitempty"`
	Element *Element        `json:"element,omitempty"`
	Before  string          `json:"before,omitempty"`
	Set     json.RawMessage `json:"set,omitempty"`
	Style   *Style          `json:"style,omitempty"`
	DX      float64         `json:"dx,omitempty"`
	DY      float64         `json:"dy,omitempty"`
	Cascade bool            `json:"cascade,omitempty"`
	IDs     []string        `json:"ids,omitempty"`
	Axis    string          `json:"axis,omitempty"`
	Value   *float64        `json:"value,omitempty"`
	Gap     *float64        `json:"gap,omitempty"`
	Start   *float64        `json:"start,omitempty"`
}

// OperationNames lists supported operations for discovery.
var OperationNames = []string{"add", "update", "move", "remove", "style", "align", "distribute", "canvas"}

// DecodeOperations strictly reads a JSON array of operations.
func DecodeOperations(data []byte) ([]Operation, error) {
	var operations []Operation
	if err := decodeStrict(data, &operations); err != nil {
		return nil, err
	}
	return operations, nil
}

// Edit applies operations in order to a copy of d and returns it with the IDs
// whose rendering may have changed. It checks each operation's own shape;
// callers validate the complete result with Validate.
func Edit(d Document, operations []Operation) (Document, []string, error) {
	d = clone(d)
	changed := map[string]bool{}
	for index, op := range operations {
		fail := func(format string, args ...any) (Document, []string, error) {
			return Document{}, nil, fmt.Errorf("operation %d (%s): %s", index, op.Op, fmt.Sprintf(format, args...))
		}
		switch op.Op {
		case "add":
			if op.Element == nil {
				return fail("element is required")
			}
			if d.Index(op.Element.ID) >= 0 {
				return fail("element %s already exists", op.Element.ID)
			}
			position := len(d.Elements)
			if op.Before != "" {
				if position = d.Index(op.Before); position < 0 {
					return fail("before names unknown element %s", op.Before)
				}
			}
			d.Elements = append(d.Elements[:position], append([]Element{*op.Element}, d.Elements[position:]...)...)
			changed[op.Element.ID] = true
		case "update":
			position := d.Index(op.ID)
			if position < 0 {
				return fail("unknown element %s", op.ID)
			}
			updated, err := patch(d.Elements[position], op.Set)
			if err != nil {
				return fail("%v", err)
			}
			d.Elements[position] = updated
			changed[op.ID] = true
		case "move":
			position := d.Index(op.ID)
			if position < 0 {
				return fail("unknown element %s", op.ID)
			}
			if !finite(op.DX) || !finite(op.DY) {
				return fail("dx and dy must be finite")
			}
			d.Elements[position].X += op.DX
			d.Elements[position].Y += op.DY
			changed[op.ID] = true
		case "remove":
			if d.Index(op.ID) < 0 {
				return fail("unknown element %s", op.ID)
			}
			remove := map[string]bool{op.ID: true}
			for again := true; again; {
				again = false
				for _, e := range d.Elements {
					if !remove[e.ID] && (remove[e.Parent] || remove[e.From] || remove[e.To]) {
						if !op.Cascade {
							return fail("%s depends on %s; remove it first or set cascade", e.ID, op.ID)
						}
						remove[e.ID], again = true, true
					}
				}
			}
			kept := d.Elements[:0]
			for _, e := range d.Elements {
				if !remove[e.ID] {
					kept = append(kept, e)
				}
			}
			d.Elements = kept
			for id := range remove {
				changed[id] = true
			}
		case "style":
			if op.Style == nil || !identifier.MatchString(op.ID) {
				return fail("style needs a stable id and a complete style")
			}
			if d.Styles == nil {
				d.Styles = map[string]Style{}
			}
			d.Styles[op.ID] = *op.Style
			for _, e := range d.Elements {
				if e.Style == op.ID {
					changed[e.ID] = true
				}
			}
		case "align", "distribute":
			if len(op.IDs) < 2 || (op.Axis != "x" && op.Axis != "y") {
				return fail("needs at least two ids and axis x or y")
			}
			positions := []int{}
			seen := map[string]bool{}
			for _, id := range op.IDs {
				position := d.Index(id)
				if position < 0 || seen[id] {
					return fail("unknown or repeated element %s", id)
				}
				if d.Elements[position].Parent != d.Elements[d.Index(op.IDs[0])].Parent {
					return fail("selected elements must share a parent")
				}
				seen[id] = true
				positions = append(positions, position)
			}
			if op.Op == "align" {
				if op.Value == nil || !finite(*op.Value) {
					return fail("align needs a finite value")
				}
				for _, position := range positions {
					setAxis(&d.Elements[position], op.Axis, *op.Value)
				}
			} else {
				if op.Gap == nil || !finite(*op.Gap) || (op.Start != nil && !finite(*op.Start)) {
					return fail("distribute needs a finite gap and optional finite start")
				}
				next := axis(d.Elements[positions[0]], op.Axis)
				if op.Start != nil {
					next = *op.Start
				}
				for _, position := range positions {
					setAxis(&d.Elements[position], op.Axis, next)
					next += extent(d.Elements[position], op.Axis) + *op.Gap
				}
			}
			for _, id := range op.IDs {
				changed[id] = true
			}
		case "canvas":
			var settings struct {
				Width      *float64 `json:"width"`
				Height     *float64 `json:"height"`
				Background *string  `json:"background"`
			}
			if err := decodeStrict(op.Set, &settings); err != nil {
				return fail("set must contain only width, height, and background: %v", err)
			}
			if settings.Width != nil {
				d.Width = *settings.Width
			}
			if settings.Height != nil {
				d.Height = *settings.Height
			}
			if settings.Background != nil {
				d.Background = *settings.Background
			}
		default:
			return fail("unknown operation; use one of %v", OperationNames)
		}
	}
	ids := make([]string, 0, len(changed))
	for id := range changed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return d, ids, nil
}

func patch(e Element, set json.RawMessage) (Element, error) {
	var fields map[string]json.RawMessage
	if err := decodeStrict(set, &fields); err != nil || fields == nil {
		return Element{}, fmt.Errorf("set must be a JSON object")
	}
	current, _ := json.Marshal(e)
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(current, &merged)
	for key, value := range fields {
		if key == "id" || key == "kind" {
			return Element{}, fmt.Errorf("%s is immutable; remove and add a new element instead", key)
		}
		if !elementFields[key] {
			return Element{}, fmt.Errorf("unknown element field %q", key)
		}
		if string(value) == "null" {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	data, _ := json.Marshal(merged)
	var updated Element
	if err := decodeStrict(data, &updated); err != nil {
		return Element{}, err
	}
	return updated, nil
}

// elementFields are the exact JSON names update may set. JSON decoding is
// case-insensitive, so a case-variant key would otherwise be applied or
// silently ignored depending on which fields are already present.
var elementFields = func() map[string]bool {
	fields := map[string]bool{}
	elementType := reflect.TypeOf(Element{})
	for index := range elementType.NumField() {
		fields[strings.Split(elementType.Field(index).Tag.Get("json"), ",")[0]] = true
	}
	return fields
}()

func axis(e Element, name string) float64 {
	if name == "x" {
		return e.X
	}
	return e.Y
}

func setAxis(e *Element, name string, value float64) {
	if name == "x" {
		e.X = value
	} else {
		e.Y = value
	}
}

func extent(e Element, name string) float64 {
	if name == "x" {
		return e.Width
	}
	return e.Height
}

func clone(d Document) Document {
	data, _ := json.Marshal(d)
	var copied Document
	_ = json.Unmarshal(data, &copied)
	if copied.Elements == nil {
		copied.Elements = []Element{}
	}
	return copied
}
