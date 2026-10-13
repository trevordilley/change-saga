package requirements

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/diagram"
)

// resolveInventoryComposition reuses inventory revision pins, directory members
// and bindings. It never consults CurrentRevision when resolving a source.
func resolveInventoryComposition(root string, inventory *Inventory, document diagram.Document, directory []DocumentationLink) (*diagram.Composition, error) {
	if err := validateDiagramSources(inventory, document, directory); err != nil {
		return nil, err
	}
	return diagram.ResolveComposition(document, func(source diagram.SourceReference) (diagram.ResolvedSource, error) {
		var result diagram.ResolvedSource
		if source.Element != "" || source.Relationship != "" {
			return result, fmt.Errorf("frame must reference a whole ERD revision")
		}
		record, revision, err := resolveDiagramSource(inventory, source)
		if err != nil {
			return result, err
		}
		if record.Kind != KindERD {
			return result, fmt.Errorf("frame source %s is not an ERD", source.Target)
		}
		if err := checkWhiteboardERD(inventory, *revision); err != nil {
			return result, err
		}
		for _, b := range revision.Bindings {
			ref := bindingSource(b)
			label, description := "", ""
			if entity := inventory.Pinned(DocumentationLink{Target: ref.Target, Revision: ref.Revision}); entity != nil {
				label, description = entity.Name, entity.Explanation
				for _, edge := range entity.Relationships {
					if edge.ID == ref.Relationship {
						label, description = edge.Label, edge.Explanation
					}
				}
			}
			result.Bindings = append(result.Bindings, diagram.Binding{ID: b.Element, Source: &ref, Label: label, Description: description})
		}
		if revision.Diagram != nil {
			doc, err := boundERDDiagram(*revision)
			if err != nil {
				return result, err
			}
			if err := validateDiagramSources(inventory, doc, revision.Directory); err != nil {
				return result, err
			}
			result.Document = &doc
			// Structured elements supply their own geometry and injected references.
			result.Bindings = nil
		} else {
			if revision.Visual == nil {
				return result, fmt.Errorf("ERD %s has no visual", source.Revision)
			}
			path := filepath.Join(root, TechnicalPath(record.Kind, record.Identity.ID), filepath.FromSlash(revision.Visual.Path))
			result.SVG, err = os.ReadFile(path)
			if err != nil {
				return result, err
			}
			if err := ValidateVisual(result.SVG, revision.Visual.Digest, revision.Bindings); err != nil {
				return result, err
			}
		}
		return result, nil
	})
}

func bindingSource(b Binding) diagram.SourceReference {
	if b.Entity != nil {
		return diagram.SourceReference{Target: b.Entity.Target, Revision: b.Entity.Revision}
	}
	if b.Relationship == nil {
		return diagram.SourceReference{}
	}
	return diagram.SourceReference{Target: b.Relationship.Owner.Target, Revision: b.Relationship.Owner.Revision, Relationship: b.Relationship.ID}
}

func boundERDDiagram(revision TechnicalRevision) (diagram.Document, error) {
	doc := *revision.Diagram
	doc.Elements = append([]diagram.Element(nil), doc.Elements...)
	for _, b := range revision.Bindings {
		index := doc.Index(b.Element)
		if index < 0 {
			return doc, fmt.Errorf("binding %s names missing structured element %s", b.ID, b.Element)
		}
		source := bindingSource(b)
		if existing := doc.Elements[index].Source; existing != nil && *existing != source {
			return doc, fmt.Errorf("binding %s conflicts with element source", b.ID)
		}
		doc.Elements[index].Source = &source
	}
	return doc, nil
}

func checkWhiteboardERD(inventory *Inventory, revision TechnicalRevision) error {
	for _, pin := range revision.Directory {
		if inventory.Pinned(pin) == nil {
			return fmt.Errorf("ERD directory revision %s does not exist", pin.Revision)
		}
	}
	return checkViewReferences(inventory, revision)
}

func validateDiagramSources(inventory *Inventory, doc diagram.Document, directory []DocumentationLink) error {
	members := map[DocumentationLink]bool{}
	for _, pin := range directory {
		members[pin] = true
	}
	for _, element := range doc.Elements {
		if element.Source == nil {
			continue
		}
		record, _, err := resolveDiagramSource(inventory, *element.Source)
		if err != nil {
			return fmt.Errorf("element %s source: %w", element.ID, err)
		}
		if element.Kind == "frame" && record.Kind != KindERD {
			return fmt.Errorf("frame %s source must be an ERD revision", element.ID)
		}
		if directory != nil && record.Kind == KindDataEntity && !members[DocumentationLink{Target: element.Source.Target, Revision: element.Source.Revision}] {
			return fmt.Errorf("element %s source is outside the ERD directory", element.ID)
		}
	}
	return nil
}

func resolveDiagramSource(inventory *Inventory, source diagram.SourceReference) (*TechnicalRecord, *TechnicalRevision, error) {
	pin := DocumentationLink{Target: source.Target, Revision: source.Revision}
	if !validPin(inventory.SagaID, pin, KindERD, KindDataEntity) {
		return nil, nil, fmt.Errorf("source must pin an ERD or data-entity revision")
	}
	record := inventory.Find(source.Target)
	revision := inventory.Pinned(pin)
	if record == nil || revision == nil {
		return nil, nil, fmt.Errorf("source revision %s does not exist", source.Revision)
	}
	if source.Relationship != "" {
		if record.Kind != KindDataEntity || source.Element != "" {
			return nil, nil, fmt.Errorf("relationship source requires an owning data-entity revision and no element")
		}
		found := false
		for _, edge := range revision.Relationships {
			if edge.ID == source.Relationship {
				found = true
				if inventory.Pinned(edge.Destination) == nil {
					return nil, nil, fmt.Errorf("relationship destination %s does not exist", edge.Destination.Revision)
				}
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("relationship %s does not exist in %s", source.Relationship, source.Revision)
		}
	}
	if source.Element != "" {
		found := false
		if record.Kind == KindERD {
			if revision.Diagram != nil {
				_, found = revision.Diagram.Element(source.Element)
			}
			for _, binding := range revision.Bindings {
				found = found || binding.Element == source.Element
			}
		} else {
			for _, field := range revision.Fields {
				found = found || field.ID == source.Element
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("source element %s does not exist in %s", source.Element, source.Revision)
		}
	}
	return record, revision, nil
}
