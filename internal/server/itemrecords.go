package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
)

// RecordLink navigates an Item's explicit product reference. It makes no
// implementation or readiness claim and preserves each record's canonical page.
func (view *landmarkView) RecordLink() *traceLink {
	if view.ItemMeta == nil || view.ItemMeta.Record == "" {
		return nil
	}
	parts := strings.Split(view.ItemMeta.Record, ":")
	owner := strings.Split(view.Target, ":")
	if len(parts) != 5 || len(owner) < 3 || parts[0] != "urn" || parts[1] != "change-saga" || parts[2] != owner[2] || !applayout.ValidID(parts[4]) {
		return nil
	}
	link := &traceLink{Target: view.ItemMeta.Record, Title: parts[3] + " " + parts[4]}
	switch parts[3] {
	case "feature":
		link.Kind, link.Href = "Feature", featureHref(parts[4])
	case "persona":
		link.Kind, link.Href = "Persona", personaHref(parts[4])
	case "story":
		link.Kind, link.Href = "Story", requirementStoryHref(parts[4])
	default:
		return nil
	}
	return link
}
