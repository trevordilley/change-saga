package server

import (
	"html/template"
	"os"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

// slideNotes holds the rendered notes of one diagram-sourced slide's
// elements, in document order. A note is authored Markdown, so it is rendered
// here with the reviewer's sanitizing helper and never reaches the page raw.
type slideNotes struct {
	order    []string
	elements map[string]diagram.Element
	html     map[string]template.HTML
}

// loadSlideNotes reads the notes of the diagram source pin names. A slide
// without a source, or whose source cannot be read, simply has no notes.
func loadSlideNotes(directory string, pin *saga.DiagramSource, namespace string) slideNotes {
	notes := slideNotes{elements: map[string]diagram.Element{}, html: map[string]template.HTML{}}
	if pin == nil {
		return notes
	}
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(pin.Source)))
	if err != nil {
		return notes
	}
	source, err := diagram.Decode(data)
	if err != nil {
		return notes
	}
	for _, element := range source.Elements {
		if element.Note == "" || element.Decorative {
			continue
		}
		notes.order = append(notes.order, element.ID)
		notes.elements[element.ID] = element
		notes.html[element.ID] = markdownWithAnchors(element.Note, namespace+"--note-"+element.ID)
	}
	return notes
}

// unbound lists the noted elements no Item selects. An Item's hotspot already
// carries its element's note; these need a hotspot of their own.
func (notes slideNotes) unbound(bound map[string]bool) []elementNoteView {
	views := []elementNoteView{}
	for _, id := range notes.order {
		if !bound[id] {
			views = append(views, elementNoteView{ElementID: id, Name: diagram.AccessibleName(notes.elements[id]), Note: notes.html[id]})
		}
	}
	return views
}

// elementNoteView is a noted diagram element that no Item selects.
type elementNoteView struct {
	ElementID string
	Name      string
	Note      template.HTML
}

func (v elementNoteView) Popover() notePopoverView { return notePopoverView{Note: v.Note} }

// notePopoverView contains explicit detail, opened from an element's info
// control. Ordinary labels and descriptions do not create redundant popovers.
type notePopoverView struct {
	Label       string
	Description string
	Body        string
	Note        template.HTML
}

func (v *reviewItemView) Popover() notePopoverView {
	popover := notePopoverView{Label: v.Item.Label, Note: v.Note}
	if v.Item.Kind == "callout" {
		popover.Body = v.Item.Body
	}
	return popover
}

func (v *landmarkView) Popover() notePopoverView {
	popover := notePopoverView{Label: v.Label, Note: v.Note}
	if v.ItemMeta != nil && v.ItemMeta.Kind == "callout" {
		popover.Body = v.ItemMeta.Body
	}
	return popover
}

// elementNoteTemplates are shared by the review deck and the Saga's decks.
// A hotspot's popover content waits in an inert template beside its landmark
// target; the page script clones it into one popover on the page itself, over
// the sandboxed slide frame rather than inside it.
const elementNoteTemplates = `{{define "element-note-popover"}}<div class="element-note">{{if .Label}}<p class="element-note-label">{{.Label}}</p>{{end}}{{if .Description}}<p class="element-note-description">{{.Description}}</p>{{end}}{{if .Body}}<p class="element-note-body">{{.Body}}</p>{{end}}{{if .Note}}<div class="element-note-markdown">{{.Note}}</div>{{end}}</div>{{end}}
{{define "element-note-template"}}{{if or .Note .Body}}<template data-landmark-note-template>{{template "element-note-popover" .}}</template>{{end}}{{end}}
{{define "element-note-targets"}}{{range .}}<span class="element-note-target" data-element-note-target data-element-id="{{.ElementID}}" data-element-name="{{.Name}}" hidden>{{template "element-note-template" .Popover}}</span>{{end}}{{end}}
`
