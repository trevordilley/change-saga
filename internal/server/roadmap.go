package server

import (
	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
	"sort"
)

// featureCollection changes only the projection. Every feature, story, essay,
// deck and theme retains its canonical identity, storage, and renderer.
func featureCollection(document *saga.Saga, planned bool) *saga.Saga {
	view := *document
	view.Features = []*saga.Feature{}
	for _, feature := range document.Features {
		if feature.Plan.Planned() == planned {
			view.Features = append(view.Features, feature)
		}
	}
	if planned {
		sort.SliceStable(view.Features, func(i, j int) bool {
			return applayout.PriorityBefore(view.Features[i].Plan.Priority, view.Features[j].Plan.Priority)
		})
	}
	return &view
}

func featureCollectionDirectory(document *saga.Saga, graph *appGraph, current, query, mode string, planned bool) *directoryView {
	return groupedFeaturesDirectory(featureCollection(document, planned), graph, current, query, mode, planned)
}
