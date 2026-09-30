package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// describeCodeLimit caps the code references listed under one Item, so an
// Item covering a thousand lines still reads compactly.
const describeCodeLimit = 8

// SlideDescriptionReference is where one of an Item's code references points,
// so a reader can open the evidence behind the Item's prose. The code itself
// is omitted.
type SlideDescriptionReference struct {
	Path  string `json:"path"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
	// Side is "old" when the reference names lines at a review's base, such
	// as deleted code; it is absent for the head.
	Side string `json:"side,omitempty"`
	Note string `json:"note,omitempty"`
}

// SlideDescriptionItem is one semantic Item in reading order. Its first code
// references are listed and the rest counted; criterion links are counted.
// query slide returns both in full.
type SlideDescriptionItem struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	Description    string `json:"description,omitempty"`
	Element        string `json:"element,omitempty"`
	Region         bool   `json:"region,omitempty"`
	About          string `json:"about,omitempty"`
	Body           string `json:"body,omitempty"`
	CodeFiles      int    `json:"code_files"`
	CriterionLinks int    `json:"criterion_links"`
	// Code lists the Item's first code references; CodeReferences counts all
	// of them and CodeMore those not listed.
	Code           []SlideDescriptionReference `json:"code"`
	CodeReferences int                         `json:"code_references"`
	CodeMore       int                         `json:"code_more,omitempty"`
}

// SlideDescription is the compact reading projection of one slide. Both the
// text and JSON formats of diagram describe render this one value.
type SlideDescription struct {
	Target     string                 `json:"target"`
	Title      string                 `json:"title"`
	Snapshot   string                 `json:"snapshot,omitempty"`
	Takeaway   string                 `json:"takeaway"`
	Intent     string                 `json:"intent"`
	Layout     string                 `json:"layout"`
	Source     string                 `json:"source"`
	MediaType  string                 `json:"media_type"`
	AssetBytes int64                  `json:"asset_bytes"`
	Items      []SlideDescriptionItem `json:"items"`
	// CodeQuery lists every Item's code references when some are not listed.
	CodeQuery string               `json:"code_query,omitempty"`
	Diagram   *diagram.Description `json:"diagram"`
	Omitted   []string             `json:"omitted"`
}

// describeSlide reads slide. base is the review's base commit, which marks
// old-side references, or empty; sagaPath names the Saga in CodeQuery.
func describeSlide(slide *saga.Slide, base, sagaPath string, offset, limit int) (SlideDescription, error) {
	value := SlideDescription{
		Target: slide.Target, Title: slide.Title, Snapshot: slide.AuthoringSnapshot, Takeaway: slide.Takeaway, Intent: slide.Intent,
		Layout: slide.Layout, Source: "asset", MediaType: slide.MediaType, Items: []SlideDescriptionItem{},
		Omitted: []string{"asset_bytes", "evidence_bodies", "criterion_link_bodies"},
	}
	if info, err := os.Stat(filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))); err == nil {
		value.AssetBytes = info.Size()
	}
	byID := map[string]*saga.Item{}
	for _, item := range slide.Items {
		byID[item.ID] = item
	}
	ordered := []*saga.Item{}
	seen := map[string]bool{}
	for _, id := range slide.ReadingOrder {
		if item := byID[id]; item != nil && !seen[id] {
			ordered, seen[id] = append(ordered, item), true
		}
	}
	for _, item := range slide.Items {
		if !seen[item.ID] {
			ordered = append(ordered, item)
		}
	}
	for _, item := range ordered {
		entry := SlideDescriptionItem{ID: item.ID, Kind: item.Kind, Label: item.Label, Description: item.Description, About: item.About, Body: item.Body, CodeFiles: len(item.Code), CriterionLinks: len(item.CriterionLinks), Code: []SlideDescriptionReference{}}
		for _, file := range item.Code {
			for _, reference := range file.References {
				if entry.CodeReferences++; len(entry.Code) < describeCodeLimit {
					entry.Code = append(entry.Code, describeReference(reference, base))
				}
			}
		}
		if entry.CodeMore = entry.CodeReferences - len(entry.Code); entry.CodeMore > 0 && value.CodeQuery == "" {
			value.CodeQuery = fmt.Sprintf("change-saga query slide --saga %s --target %s", sagaPath, slide.Target)
		}
		switch item.Selector.Type {
		case "element":
			entry.Element = item.Selector.ElementID
		case "region":
			entry.Region = true
		}
		value.Items = append(value.Items, entry)
	}
	if slide.Diagram == nil {
		return value, nil
	}
	data, err := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if err != nil {
		return value, err
	}
	source, err := diagram.Decode(data)
	if err != nil {
		return value, fmt.Errorf("diagram source: %w", err)
	}
	description := diagram.Describe(source, offset, limit)
	value.Source, value.Diagram = "diagram", &description
	value.Omitted = append(value.Omitted, diagram.Omitted...)
	return value, nil
}

func describeReference(reference coderef.Reference, base string) SlideDescriptionReference {
	value := SlideDescriptionReference{Path: reference.Path, Start: reference.Start, End: reference.End, Note: reference.Note}
	if base != "" && reference.Commit == base {
		value.Side = "old"
	}
	return value
}

// location is path:start-end, path:line, or the bare path of a whole file,
// marked (old) at a review's base.
func (r SlideDescriptionReference) location(withPath bool) string {
	value := ""
	if withPath {
		value = r.Path
	}
	switch {
	case r.Start == 0 && r.End == 0:
	case r.Start == r.End:
		value += fmt.Sprintf(":%d", r.Start)
	default:
		value += fmt.Sprintf(":%d-%d", r.Start, r.End)
	}
	if r.Side != "" {
		value += " (" + r.Side + ")"
	}
	return value
}

// writeCode prints references one line per run of the same file and note, so
// a note shared by several ranges is printed once.
func writeCode(b *strings.Builder, item SlideDescriptionItem, query string) {
	for index := 0; index < len(item.Code); {
		first := item.Code[index]
		parts := []string{first.location(true)}
		for index++; index < len(item.Code) && item.Code[index].Path == first.Path && item.Code[index].Note == first.Note; index++ {
			parts = append(parts, strings.TrimPrefix(item.Code[index].location(false), ":"))
		}
		fmt.Fprintf(b, "     code: %s", strings.Join(parts, ", "))
		if first.Note != "" {
			fmt.Fprintf(b, " — %s", diagram.Plain(first.Note))
		}
		b.WriteString("\n")
	}
	if item.CodeMore > 0 {
		fmt.Fprintf(b, "     code: and %d more; `%s` lists them\n", item.CodeMore, query)
	}
}

func (v SlideDescription) writeText(out io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Slide: %s %s\n", v.Target, strconv.Quote(v.Title))
	if v.Snapshot != "" {
		fmt.Fprintf(&b, "Snapshot: %s\n", v.Snapshot)
	}
	fmt.Fprintf(&b, "Takeaway: %s\nIntent: %s; layout: %s\n", diagram.Plain(v.Takeaway), v.Intent, v.Layout)
	if v.Source == "diagram" {
		fmt.Fprintf(&b, "Source: diagram rendered to %s (%d bytes); omits %s; cannot rebuild the drawing\n", v.MediaType, v.AssetBytes, strings.Join(v.Omitted, ", "))
	} else {
		fmt.Fprintf(&b, "Source: hand-authored %s (%d bytes, not shown); read it with `change-saga query slide`\n", v.MediaType, v.AssetBytes)
	}
	if len(v.Items) > 0 {
		b.WriteString("\nItems (reading order):\n")
	}
	for index, item := range v.Items {
		fmt.Fprintf(&b, "  %d. %s [%s] %s", index+1, item.ID, item.Kind, strconv.Quote(item.Label))
		switch {
		case item.Element != "":
			fmt.Fprintf(&b, " element=%s", item.Element)
		case item.Region:
			b.WriteString(" region")
		}
		if item.About != "" {
			fmt.Fprintf(&b, " about=%s", item.About)
		}
		fmt.Fprintf(&b, " code_files=%d criterion_links=%d\n", item.CodeFiles, item.CriterionLinks)
		for _, field := range [][2]string{{"description", item.Description}, {"body", item.Body}} {
			if field[1] != "" {
				fmt.Fprintf(&b, "     %s: %s\n", field[0], diagram.Plain(field[1]))
			}
		}
		writeCode(&b, item, v.CodeQuery)
	}
	if v.Diagram != nil {
		v.Diagram.WriteText(&b)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func diagramDescribe(ctx context.Context, args []string, out io.Writer) error {
	name := "diagram describe"
	flags := commandFlags(name, commandUsage[name], out)
	target := flags.String("slide", "", "slide to describe")
	review := flags.String("review", "", "the pull request review whose slide this is")
	format := flags.String("format", "text", "text or json")
	offset := flags.Int("offset", 0, "first diagram element to include")
	limit := flags.Int("limit", 60, "diagram elements per page, 1-100")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *target == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	if *offset < 0 || *limit < 1 || *limit > diagram.MaxDescribeLimit {
		return fmt.Errorf("--offset must be at least 0 and --limit 1-%d", diagram.MaxDescribeLimit)
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("--format must be text or json")
	}
	document, _, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	_, slide, owner, err := findAuthoredSlide(document, *review, *target)
	if err != nil {
		return err
	}
	// A review's old-side references are pinned to its base. When the
	// range cannot be read here, references are listed without a side.
	base := ""
	if found := document.FindReview(owner); owner != "" && found != nil {
		if rng, err := reviewstate.ResolveRange(ctx, document.Root, found); err == nil && rng.BaseOID != rng.HeadOID {
			base = rng.BaseOID
		}
	}
	value, err := describeSlide(slide, base, flags.Arg(0), *offset, *limit)
	if err != nil {
		return err
	}
	if *format == "json" {
		return writeJSON(out, value)
	}
	return value.writeText(out)
}

func diagramGet(args []string, out io.Writer) error {
	name := "diagram get"
	flags := commandFlags(name, commandUsage[name], out)
	target := flags.String("slide", "", "slide whose diagram holds the element")
	review := flags.String("review", "", "the pull request review whose slide this is")
	id := flags.String("id", "", "diagram element id")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *target == "" || *id == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	state, err := loadDiagramRevision(flags.Arg(0), *review, *target, "")
	if err != nil {
		return err
	}
	element, found := state.source.Element(*id)
	if !found {
		return fmt.Errorf("diagram of %s has no element %q", state.slide.Target, *id)
	}
	style, _ := state.source.Style(element.Style)
	return writeJSON(out, map[string]any{"slide": state.slide.Target, "snapshot": state.revision.Snapshot, "element": element, "style": style, "selector": "#" + element.ID})
}
