package server

import (
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Stable document IDs connect the essay, navigation and independent histories.
var technicalDesignBoards = []struct{ ID, Title, Note string }{
	{"data-model", "ERD", "Entities and relationships, using crow's-foot notation to show cardinality and optionality."},
	{"infrastructure", "Infrastructure", "The actual layout of platforms, deployed services, resources and their connections."},
	{"data-flows", "Data flows", "How data enters, moves through and leaves the application, including key services, stores and transformations."},
	{"architecture", "Architecture", "How the codebase fits together: responsibility boundaries, dependency direction, state ownership, design patterns and the tradeoffs behind technology choices."},
}

func technicalDesignBoardOf(id string) (struct{ ID, Title, Note string }, bool) {
	for _, board := range technicalDesignBoards {
		if board.ID == id {
			return board, true
		}
	}
	return struct{ ID, Title, Note string }{}, false
}

func technicalEssayHref(pkg string) string {
	if pkg == applayout.OverviewTechnicalDesign {
		return technicalPath
	}
	return technicalPath + "/pages/" + url.PathEscape(strings.TrimSuffix(strings.TrimPrefix(pkg, "technical-design-"), ".fragment"))
}

func technicalEssayPages(document *saga.Saga) []traceLink {
	var pages []traceLink
	if document.Overview == nil {
		return pages
	}
	for _, fragment := range document.Overview.Fragments {
		pkg := strings.TrimPrefix(fragment.Path, applayout.OverviewDir+"/")
		if pkg != applayout.OverviewTechnicalDesign && applayout.IsTechnicalDesignPart(pkg) {
			pages = append(pages, traceLink{Title: fragment.Title, Href: technicalEssayHref(pkg), Target: fragment.Target})
		}
	}
	return pages
}
