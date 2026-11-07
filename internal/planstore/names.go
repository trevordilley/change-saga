package planstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// refuseNames turns the problems a write would leave in a Plan's names into one
// error naming each.
func refuseNames(problems []saga.NameProblem) error {
	if len(problems) == 0 {
		return nil
	}
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		lines = append(lines, problem.String())
	}
	return fmt.Errorf("refused, because the Plan's names would be left like this:\n  %s", strings.Join(lines, "\n  "))
}

func writeName(name *saga.PlanName) error {
	return store.WriteJSON(filepath.Join(name.Directory, saga.NameManifestName), name.NameManifest, false)
}

// AddName writes a new name. Its parent and links must already be names of the
// Plan, so a hierarchy is written from the top down.
func AddName(root, planID string, name saga.NameManifest) (string, error) {
	name.Schema, name.Version = saga.NameSchemaURL, saga.PlanVersion
	if name.CreatedAt.IsZero() {
		name.CreatedAt = time.Now().UTC()
	}
	var created string
	err := mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		if err := saga.ValidateNameManifest(document.Manifest.ID, name); err != nil {
			return err
		}
		if plan.FindName(name.ID) != nil {
			return fmt.Errorf("name %q already exists in plan %q", name.ID, planID)
		}
		if err := refuseNames(saga.PlanNameProblems(append(plan.NameManifests(), name), plan.Manifests())); err != nil {
			return err
		}
		names, err := store.EnsureDirWithin(document.Root, filepath.Join(plan.Directory, saga.PlanNamesDir))
		if err != nil {
			return err
		}
		err = store.CommitDir(document.Root, filepath.Join(names, name.ID+saga.NameSuffix), func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			return store.WriteJSON(filepath.Join(stage, saga.NameManifestName), name, true)
		})
		if err != nil {
			return err
		}
		created = saga.NameTarget(document.Manifest.ID, planID, name.ID)
		return nil
	})
	return created, err
}

// UpdateName applies change to one name and checks the resulting hierarchy.
func UpdateName(root, planID, nameID string, change func(*saga.NameManifest) error) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		name := plan.FindName(nameID)
		if name == nil {
			return fmt.Errorf("name %q does not exist in plan %q%s", nameID, planID, knownNames(plan))
		}
		if err := change(&name.NameManifest); err != nil {
			return err
		}
		if err := saga.ValidateNameManifest(document.Manifest.ID, name.NameManifest); err != nil {
			return err
		}
		if err := refuseNames(saga.PlanNameProblems(plan.NameManifests(), plan.Manifests())); err != nil {
			return err
		}
		return writeName(name)
	})
}

// RemoveName deletes a name. It is refused while a child, a link, or a Task still
// names it, and the refusal names each, so nothing is left pointing at nothing.
func RemoveName(root, planID, nameID string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		name := plan.FindName(nameID)
		if name == nil {
			return fmt.Errorf("name %q does not exist in plan %q%s", nameID, planID, knownNames(plan))
		}
		children, linkedFrom, tasks := saga.NameReferences(plan.NameManifests(), plan.Manifests(), nameID)
		var reasons []string
		if len(children) > 0 {
			reasons = append(reasons, "names beneath it: "+strings.Join(children, ", ")+" (move them with plan set-name --parent)")
		}
		if len(linkedFrom) > 0 {
			reasons = append(reasons, "names linking to it: "+strings.Join(linkedFrom, ", ")+" (change their links with plan set-name --link)")
		}
		if len(tasks) > 0 {
			reasons = append(reasons, "tasks naming it: "+strings.Join(tasks, ", ")+" (change them with task set --name)")
		}
		if len(reasons) > 0 {
			return fmt.Errorf("name %q is still named by\n  %s", nameID, strings.Join(reasons, "\n  "))
		}
		return os.RemoveAll(name.Directory)
	})
}

func knownNames(plan *saga.Plan) string {
	if len(plan.Names) == 0 {
		return "; it has no names yet"
	}
	ids := make([]string, 0, len(plan.Names))
	for _, name := range plan.Names {
		ids = append(ids, name.ID)
	}
	return "; names: " + strings.Join(ids, ", ")
}
