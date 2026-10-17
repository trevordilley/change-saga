package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
)

// deckSections marks the start of each contiguous run of a named section in
// deck order. Empty labels end a run; a later repeated label starts another.
// Dividers decorate the next slide, never create destinations or change order.
// Both deck contexts use this metadata-only projection without loading visuals.
type deckSections struct {
	previous string
}

func (sections *deckSections) next(slide *saga.SlideManifest) string {
	label := ""
	if slide != nil {
		label = strings.TrimSpace(slide.Section)
	}
	previous := sections.previous
	sections.previous = label
	if label != previous {
		return label
	}
	return ""
}

// Keep section presentation shared by implementation and review outlines. The
// divider belongs outside the selectable card, so selection and numbering stay
// attached to the slide alone.
const deckSectionTemplates = `{{define "slide-section-divider"}}{{if .}}<div class="slide-section-divider" data-slide-section>{{.}}</div>{{end}}{{end}}`
