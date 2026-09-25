package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

// SlideDescriptionItem is one semantic Item in reading order. Evidence and
// criterion links are counted, not inlined; query slide returns them in full.
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
	Diagram    *diagram.Description   `json:"diagram"`
	Omitted    []string               `json:"omitted"`
}

func describeSlide(slide *saga.Slide, offset, limit int) (SlideDescription, error) {
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
		entry := SlideDescriptionItem{ID: item.ID, Kind: item.Kind, Label: item.Label, Description: item.Description, About: item.About, Body: item.Body, CodeFiles: len(item.Code), CriterionLinks: len(item.CriterionLinks)}
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
	}
	if v.Diagram != nil {
		v.Diagram.WriteText(&b)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func diagramDescribe(args []string, out io.Writer) error {
	name := "diagram describe"
	flags := commandFlags(name, commandUsage[name], out)
	target := flags.String("slide", "", "slide to describe")
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
	slide := findSlide(document, *target)
	if slide == nil {
		return fmt.Errorf("slide %q does not exist", *target)
	}
	value, err := describeSlide(slide, *offset, *limit)
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
	id := flags.String("id", "", "diagram element id")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *target == "" || *id == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	state, err := loadDiagramRevision(flags.Arg(0), *target, "")
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
