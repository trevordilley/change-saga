package diagram

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Field is one row of an entity: a column or attribute, its type, and whether
// it is a primary key, a foreign key, or both.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Key  string `json:"key,omitempty"`
}

var fieldKeys = map[string]bool{"": true, "pk": true, "fk": true, "pk,fk": true}

const (
	maxFields     = 100
	maxFieldRunes = 64
	// Entity metrics are fixed so an author can aim an edge at a field row:
	// row i (from 0) spans y = header + i*entityRowHeight within the node.
	entityRowHeight = 28.0
	entityFieldSize = 16.0
	entityKeySize   = 12.0
	entityPadding   = 12.0
)

// entityHeaderHeight is the height of an entity's name band for its style.
func entityHeaderHeight(fontSize float64) float64 { return fontSize*lineHeight + 16 }

// entityContract publishes the fixed entity metrics, so an author can aim an
// edge's points at a field row.
func entityContract() map[string]any {
	return map[string]any{
		"shape":         "entity",
		"field":         []string{"name", "type", "key"},
		"keys":          []string{"pk", "fk", "pk,fk"},
		"header_height": "the style's font_size * 1.25 + 16",
		"row_height":    entityRowHeight,
		"row_top":       "header_height + index * row_height, local to the entity",
		"edge_fields":   "an edge's from_field or to_field names the entity field it ends on; its points stay explicit",
	}
}

// validateEntity checks entity fields and the field an edge ends on.
func validateEntity(d Document, e Element, byID map[string]Element) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if len(e.Fields) > 0 && (e.Kind != "node" || e.Shape != "entity") {
		add("fields are only valid on an entity node")
	}
	if e.Kind != "edge" && (e.FromField != "" || e.ToField != "") {
		add("from_field and to_field are only valid on edges")
	}
	if e.Kind == "node" && e.Shape == "entity" {
		if strings.TrimSpace(e.Label) == "" {
			add("an entity needs a label naming it")
		}
		if e.Detail != "" {
			add("an entity lists fields instead of a detail; move it to description or note")
		}
		if e.LabelBox != nil {
			add("an entity names itself in its header and takes no label_box")
		}
		if len(e.Fields) > maxFields {
			add("an entity may list at most %d fields", maxFields)
		}
		seen := map[string]bool{}
		for index, field := range e.Fields {
			name := field.Name
			if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\n\r\t") || len([]rune(name)) > maxFieldRunes {
				add("field %d needs a name of 1-%d characters without surrounding spaces or line breaks", index+1, maxFieldRunes)
			} else if seen[name] {
				add("field %s is repeated", name)
			}
			seen[name] = true
			if strings.ContainsAny(field.Type, "\n\r\t") || len([]rune(field.Type)) > maxFieldRunes {
				add("field %s: type must be at most %d characters on one line", name, maxFieldRunes)
			}
			if !fieldKeys[field.Key] {
				add("field %s: key must be pk, fk, or pk,fk", name)
			}
		}
		style, known := d.Style(e.Style)
		if need := entityHeaderHeight(style.FontSize) + float64(len(e.Fields))*entityRowHeight; known && e.Height > 0 && need > e.Height+.01 {
			add("entity needs height %s for its header and %d field rows of %s; it has %s", num(need), len(e.Fields), num(entityRowHeight), num(e.Height))
		}
	}
	if e.Kind == "edge" {
		for _, end := range [][3]string{{"from_field", e.From, e.FromField}, {"to_field", e.To, e.ToField}} {
			if end[2] == "" {
				continue
			}
			node, ok := byID[end[1]]
			if !ok || node.Shape != "entity" {
				add("%s needs its endpoint %q to be an entity", end[0], end[1])
				continue
			}
			if !hasField(node, end[2]) {
				add("%s names %q, which entity %s does not list", end[0], end[2], end[1])
			}
		}
	}
	return problems
}

func hasField(e Element, name string) bool {
	for _, field := range e.Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// keyMarkers label an entity row's key column.
var keyMarkers = map[string]string{"pk": "PK", "fk": "FK", "pk,fk": "PK FK"}

// entity draws an ERD table: a header naming the entity, then one row per
// field with its key marker, name, and type. Each row carries data-field so a
// reader, or an edge's to_field, can name it.
func (r *renderer) entity(e Element, style Style, g *node) error {
	w := e.Width
	line := firstColor(style.Stroke, style.Ink)
	paint(g.add("rect", "width", num(w), "height", num(e.Height), "rx", "8"), style)
	header := entityHeaderHeight(style.FontSize)
	g.add("path", "d", fmt.Sprintf("M0 %sV8A8 8 0 0 1 8 0H%sA8 8 0 0 1 %s 8V%sZ", num(header), num(w-8), num(w), num(header)), "fill", line, "fill-opacity", ".12")
	if len(e.Fields) > 0 {
		g.add("path", "d", fmt.Sprintf("M0 %sH%s", num(header), num(w)), "stroke", line, "stroke-width", num(math.Max(style.StrokeWidth, 1)))
	}
	left, width := entityPadding, w-2*entityPadding
	align := e.Align
	if align == "" {
		align = "middle"
	}
	if e.Icon != "" {
		size := e.IconSize
		if size == 0 {
			size = math.Min(24, style.FontSize)
		}
		icon, err := r.icon(e.Icon, left, (header-size)/2, size, style.Ink)
		if err != nil {
			return err
		}
		g.children = append(g.children, icon)
		left += size + 8
		width -= size + 8
	}
	if err := r.text(g, e.Label, Box{X: left, Y: 8, Width: width, Height: style.FontSize * lineHeight}, style.FontSize, style.Ink, false, align); err != nil {
		return err
	}
	markerWidth := 0.0
	for _, field := range e.Fields {
		if field.Key == "" {
			continue
		}
		measured, err := r.measure(keyMarkers[field.Key], entityKeySize)
		if err != nil {
			return err
		}
		markerWidth = math.Max(markerWidth, measured*widthSlack+10)
	}
	for index, field := range e.Fields {
		top := header + float64(index)*entityRowHeight
		if index > 0 {
			g.add("path", "d", fmt.Sprintf("M0 %sH%s", num(top), num(w)), "stroke", line, "stroke-opacity", ".35", "stroke-width", "1")
		}
		row := g.add("g", "data-field", field.Name)
		if field.Key != "" {
			row.set("data-key", field.Key)
			marker := row.add("text", "x", num(entityPadding), "y", num(top+(entityRowHeight+entityKeySize)/2-1), "font-size", num(entityKeySize), "fill", line, "xml:space", "preserve")
			marker.text = keyMarkers[field.Key]
		}
		nameX := entityPadding + markerWidth
		room := w - entityPadding - nameX
		nameWidth, err := r.measure(field.Name, entityFieldSize)
		if err != nil {
			return err
		}
		typeWidth, err := r.measure(field.Type, entityFieldSize)
		if err != nil {
			return err
		}
		need := nameWidth * widthSlack
		if field.Type != "" {
			need += 12 + typeWidth*widthSlack
		}
		if need > room+.01 {
			return fmt.Errorf("text overflow: field %s needs width %.1f for its name and type, its row has %.1f", field.Name, need, room)
		}
		baseline := num(top + (entityRowHeight+entityFieldSize)/2 - 2)
		name := row.add("text", "x", num(nameX), "y", baseline, "font-size", num(entityFieldSize), "fill", style.Ink, "xml:space", "preserve")
		if strings.HasPrefix(field.Key, "pk") {
			name.set("text-decoration", "underline")
		}
		name.text = field.Name
		if field.Type != "" {
			kind := row.add("text", "x", num(w-entityPadding), "y", baseline, "font-size", num(entityFieldSize), "fill", style.Ink, "fill-opacity", ".7", "text-anchor", "end", "xml:space", "preserve")
			kind.text = field.Type
		}
	}
	return nil
}

// Fields are an entity's rows in order.
type Fields []Field

// String is describe's compact reading of fields, such as
// "id uuid pk, customer_id uuid fk, total numeric".
func (fields Fields) String() string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		part := fieldToken(field.Name)
		if field.Type != "" {
			part += " " + fieldToken(field.Type)
		}
		if field.Key != "" {
			part += " " + field.Key
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// fieldToken quotes a name or type that would otherwise blur the list.
func fieldToken(s string) string {
	if strings.ContainsAny(s, " ,\"\\") || strconv.Quote(s)[1:len(strconv.Quote(s))-1] != s {
		return strconv.Quote(s)
	}
	return s
}

// endpoint is an edge end as describe reads it: the node, or node.field.
func endpoint(id, field string) string {
	if field == "" {
		return id
	}
	return id + "." + fieldToken(field)
}
