package prototypes

import (
	"fmt"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/applayout"
)

// Find returns the loaded prototype with the given stable ID.
func (d Document) Find(id string) *Prototype {
	for i := range d.Prototypes {
		if d.Prototypes[i].Identity.ID == id {
			return &d.Prototypes[i]
		}
	}
	return nil
}

// Revision returns one loaded revision of the prototype.
func (p Prototype) Revision(id string) *Revision {
	for i := range p.Revisions {
		if p.Revisions[i].ID == id {
			return &p.Revisions[i]
		}
	}
	return nil
}

// RevisionDir is the on-disk <id>.revision package of one revision. An html
// revision's files live beneath it, at the paths its Source names.
func (d Document) RevisionDir(p Prototype, revisionID string) (string, error) {
	for _, feature := range d.Features {
		if feature.ID == p.Feature {
			return filepath.Join(feature.Dir, applayout.RequirementsDir, "prototypes", p.Identity.ID+".prototype", "revisions", revisionID+".revision"), nil
		}
	}
	return "", fmt.Errorf("prototype %q has no feature directory", p.Identity.ID)
}
