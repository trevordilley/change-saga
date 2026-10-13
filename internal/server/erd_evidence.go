package server

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Evidence is inspected only when the shared documentation drawer is opened.
// Linked references, verified saved bytes, and resolution at the viewed source
// commit are independent facts. None is a claim of complete model coverage.
type erdEvidenceCounts struct {
	Linked, Verified, Resolved, Stale, Unavailable, Unverified int
}

type erdEvidenceMember struct {
	Pin                                  saga.DocumentationLink
	Name, Relationship, PinStatus, State string
	Counts                               erdEvidenceCounts
	references                           []coderef.Reference
}

type erdEvidenceView struct {
	Entities, Relationships, Missing int
	Counts                           erdEvidenceCounts
	Members                          []erdEvidenceMember
}

func (a *app) erdEvidence(ctx context.Context, inventory requirements.Inventory, revision *requirements.TechnicalRevision) *erdEvidenceView {
	view := &erdEvidenceView{}
	var refs []coderef.Reference
	add := func(pin saga.DocumentationLink, name, relationship string, evidence []requirements.Evidence) {
		member := erdEvidenceMember{Pin: pin, Name: name, Relationship: relationship, PinStatus: inventory.LinkStatus(pin), references: requirements.References(evidence)}
		view.Members = append(view.Members, member)
		refs = append(refs, member.references...)
	}
	for _, pin := range revision.Directory {
		view.Entities++
		entity := inventory.Pinned(pin)
		if entity == nil {
			add(pin, pin.Target, "", nil)
			continue
		}
		add(pin, entity.Name, "", entity.Code)
		for _, edge := range entity.Relationships {
			view.Relationships++
			add(pin, makeRelationshipView(inventory, pin, edge, "outgoing").Notation, edge.ID, edge.Code)
		}
	}
	// One shared resolver/read cache for all references in this exact directory.
	code := a.referenceCodeView(ctx, refs, "data model", true)
	index := 0
	for i := range view.Members {
		member := &view.Members[i]
		member.Counts.Linked = len(member.references)
		member.State = "missing"
		if member.Counts.Linked == 0 {
			view.Missing++
		}
		for range member.references {
			c := code[index]
			index++
			if c.Integrity == "verified" {
				member.Counts.Verified++
			}
			if c.Integrity == "unverified" {
				member.Counts.Unverified++
			}
			if c.Integrity == "unavailable" || c.Resolution == "unavailable" {
				member.Counts.Unavailable++
			}
			if c.Resolution == "current" && c.Integrity == "verified" {
				member.Counts.Resolved++
			}
			if c.Resolution == "stale" {
				member.Counts.Stale++
			}
		}
		switch {
		case member.Counts.Unavailable > 0:
			member.State = "unavailable"
		case member.Counts.Unverified > 0:
			member.State = "unverified"
		case member.Counts.Stale > 0:
			member.State = "stale"
		case member.Counts.Linked > 0 && member.Counts.Resolved == member.Counts.Linked:
			member.State = "resolved"
		case member.Counts.Linked > 0:
			member.State = "linked"
		}
		view.Counts.Linked += member.Counts.Linked
		view.Counts.Verified += member.Counts.Verified
		view.Counts.Resolved += member.Counts.Resolved
		view.Counts.Stale += member.Counts.Stale
		view.Counts.Unavailable += member.Counts.Unavailable
		view.Counts.Unverified += member.Counts.Unverified
	}
	return view
}

// Keep the same code resolver and template as slide-deck references. Evidence
// inspection shows saved locations, even if the resolver can remap them today.
func (a *app) pinnedDocumentationCode(ctx context.Context, evidence []requirements.Evidence, subject string) []*termCodeView {
	refs := requirements.References(evidence)
	views := a.referenceCodeView(ctx, refs, subject, true)
	for i, view := range views {
		view.Note = refs[i].Note + " " + view.Note
	}
	return views
}
