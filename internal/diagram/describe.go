package diagram

import (
	"fmt"
	"strconv"
	"strings"
)

// Summary is one semantic element in the reading view.
type Summary struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Shape       string `json:"shape,omitempty"`
	Label       string `json:"label,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Description string `json:"description,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// Omitted names what every reading view leaves out.
var Omitted = []string{"geometry", "styles", "decorative_elements", "fragment_markup"}

// Description is the single reading projection behind every describe format.
// It cannot rebuild the drawing and is not an editable syntax.
type Description struct {
	Elements   []Summary `json:"elements"`
	Offset     int       `json:"offset"`
	Total      int       `json:"total"`
	NextOffset int       `json:"next_offset"`
	HasMore    bool      `json:"has_more"`
	Omitted    []string  `json:"omitted"`
}

// Describe projects the semantic (non-decorative) elements of d in document
// order, one bounded page at a time.
func Describe(d Document, offset, limit int) Description {
	all := []Summary{}
	for _, e := range d.Elements {
		if e.Decorative {
			continue
		}
		all = append(all, Summary{ID: e.ID, Kind: e.Kind, Shape: e.Shape, Label: e.Label, Detail: e.Detail, Description: e.Description, From: e.From, To: e.To, Parent: e.Parent, Icon: e.Icon})
	}
	offset = max(0, min(offset, len(all)))
	end := min(offset+max(limit, 0), len(all))
	return Description{Elements: all[offset:end], Offset: offset, Total: len(all), NextOffset: end, HasMore: end < len(all), Omitted: append([]string{}, Omitted...)}
}

var textSections = []struct{ kind, heading string }{{"group", "Groups"}, {"node", "Nodes"}, {"edge", "Edges"}, {"text", "Text"}, {"graphic", "Graphics"}}

// WriteText renders the elements as compact Graphviz-like reading text in
// sections, preserving document order within each section.
func (v Description) WriteText(b *strings.Builder) {
	for _, section := range textSections {
		heading := false
		for _, e := range v.Elements {
			if e.Kind != section.kind {
				continue
			}
			if !heading {
				fmt.Fprintf(b, "\n%s:\n", section.heading)
				heading = true
			}
			b.WriteString("  " + e.ID)
			if e.Kind == "edge" {
				b.WriteString(": " + e.From + " -> " + e.To)
			}
			if e.Label != "" {
				b.WriteString(" " + strconv.Quote(e.Label))
			}
			for _, attr := range [][2]string{{"shape", e.Shape}, {"in", e.Parent}, {"icon", e.Icon}} {
				if attr[1] != "" {
					fmt.Fprintf(b, " %s=%s", attr[0], attr[1])
				}
			}
			b.WriteString("\n")
			for _, field := range [][2]string{{"detail", e.Detail}, {"description", e.Description}} {
				if field[1] != "" {
					fmt.Fprintf(b, "    %s: %s\n", field[0], Plain(field[1]))
				}
			}
		}
	}
	switch {
	case v.Total == 0:
		b.WriteString("\nNo semantic diagram elements.\n")
	case len(v.Elements) == 0:
		fmt.Fprintf(b, "\nNo elements at offset %d of %d.\n", v.Offset, v.Total)
	case v.HasMore:
		fmt.Fprintf(b, "\nShowing elements %d-%d of %d; continue with --offset %d.\n", v.Offset+1, v.NextOffset, v.Total, v.NextOffset)
	default:
		fmt.Fprintf(b, "\nShowing elements %d-%d of %d.\n", v.Offset+1, v.NextOffset, v.Total)
	}
}

// Plain leaves ordinary prose unquoted but quotes anything that could break
// line structure or be misread, such as newlines or surrounding spaces.
func Plain(s string) string {
	if q := strconv.Quote(s); q[1:len(q)-1] == s && strings.TrimSpace(s) == s {
		return s
	}
	return strconv.Quote(s)
}
