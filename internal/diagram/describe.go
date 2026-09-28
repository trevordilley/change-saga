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
	Note        string `json:"note,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Icon        string `json:"icon,omitempty"`
	// An edge's terminators and end labels, when they say more than a plain
	// arrow: ERD cardinality, UML ends, or a start marker.
	Tail      string   `json:"tail,omitempty"`
	Head      string   `json:"head,omitempty"`
	TailLabel string   `json:"tail_label,omitempty"`
	HeadLabel string   `json:"head_label,omitempty"`
	Fields    Fields   `json:"fields,omitempty"`
	FromField string   `json:"from_field,omitempty"`
	ToField   string   `json:"to_field,omitempty"`
	About     string   `json:"about,omitempty"`
	Members   []string `json:"members,omitempty"`
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
		summary := Summary{ID: e.ID, Kind: e.Kind, Shape: describedShape(e.Shape), Label: e.Label, Detail: e.Detail, Description: e.Description, Note: e.Note, From: e.From, To: e.To, Parent: e.Parent, Icon: e.Icon, TailLabel: e.TailLabel, HeadLabel: e.HeadLabel, Fields: e.Fields, FromField: e.FromField, ToField: e.ToField, About: e.About, Members: sectionMembers(d, e)}
		if e.Tail != "none" {
			summary.Tail = e.Tail
		}
		if e.Head != "arrow" && e.Head != "none" {
			summary.Head = e.Head
		}
		all = append(all, summary)
	}
	offset = max(0, min(offset, len(all)))
	end := min(offset+max(limit, 0), len(all))
	return Description{Elements: all[offset:end], Offset: offset, Total: len(all), NextOffset: end, HasMore: end < len(all), Omitted: append([]string{}, Omitted...)}
}

var textSections = []struct{ kind, heading string }{{"section", "Sections"}, {"group", "Groups"}, {"node", "Nodes"}, {"edge", "Edges"}, {"text", "Text"}, {"graphic", "Graphics"}, {"note", "Notes"}}

// WriteText renders the elements as compact Graphviz-like reading text in
// sections, preserving document order within each section.
func (v Description) WriteText(b *strings.Builder) {
	for _, section := range textSections {
		heading := false
		for _, e := range v.Elements {
			if textSection(e) != section.kind {
				continue
			}
			if !heading {
				fmt.Fprintf(b, "\n%s:\n", section.heading)
				heading = true
			}
			b.WriteString("  " + e.ID)
			shape := e.Shape
			switch section.kind {
			case "edge":
				b.WriteString(": " + endpoint(e.From, e.FromField) + " -> " + endpoint(e.To, e.ToField))
			case "section":
				shape = ""
			case "note":
				b.WriteString(": " + noteWord(e))
				if e.About != "" {
					b.WriteString(" about " + e.About)
				}
				shape = ""
			}
			if e.Label != "" {
				b.WriteString(" " + strconv.Quote(e.Label))
			}
			for _, attr := range [][2]string{{"shape", shape}, {"in", e.Parent}, {"icon", e.Icon}, {"tail", e.Tail}, {"head", e.Head}} {
				if attr[1] != "" {
					fmt.Fprintf(b, " %s=%s", attr[0], attr[1])
				}
			}
			for _, attr := range [][2]string{{"tail_label", e.TailLabel}, {"head_label", e.HeadLabel}} {
				if attr[1] != "" {
					fmt.Fprintf(b, " %s=%s", attr[0], strconv.Quote(attr[1]))
				}
			}
			b.WriteString("\n")
			if len(e.Fields) > 0 {
				b.WriteString("    fields: " + e.Fields.String() + "\n")
			}
			for _, field := range [][2]string{{"members", strings.Join(e.Members, ", ")}, {"detail", e.Detail}, {"description", e.Description}, {"note", e.Note}} {
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
