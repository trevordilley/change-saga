package livingapp

import (
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Native elements participate in the same forward and reverse code paths as
// slide Items, without acquiring a synthetic deck or slide identity.
func whiteboardReviewEvidence(document *saga.Saga) []reviewEvidence {
	if document.Root == "" {
		return nil
	}
	items, _ := requirements.CurrentWhiteboardItems(document.Root, document.Manifest.ID)
	var result []reviewEvidence
	for _, item := range items {
		add := func(reference coderef.Reference, file string) {
			location := reference.Location()
			value := reviewEvidence{Item: item.Target, Ref: location.String(), Reference: reference, Location: location, EvidenceFile: file, Path: []string{item.Target, location.String()}}
			value.Key = value.Item + "\x00" + value.EvidenceFile + "\x00" + value.Ref
			result = append(result, value)
		}
		for _, file := range item.Code {
			for _, reference := range file.References {
				add(reference, file.Path)
			}
		}
		for _, selection := range item.Selections {
			add(selection.Code, item.Path+"#element="+item.ID+"#selection="+selection.ID)
		}
	}
	return result
}
