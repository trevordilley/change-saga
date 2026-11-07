// Package planstore writes Plans and their Tasks. A Plan is plain current state:
// each write replaces a small record, and Git keeps the history. Every write that
// touches the Task graph checks the whole resulting graph first, so a Plan never
// holds a Task that leads to no user story.
package planstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// mutate runs operation on the freshly loaded, valid Saga under the writer lock.
func mutate(root string, operation func(*saga.Saga) error) error {
	return store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, validation, err := saga.Load(root)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("cannot write into a structurally invalid Saga; run change-saga validate")
		}
		return operation(document)
	})
}

// editable finds a Plan that may still change.
func editable(document *saga.Saga, planID string) (*saga.Plan, error) {
	plan := document.FindPlan(planID)
	if plan == nil {
		return nil, fmt.Errorf("plan %q does not exist%s", planID, knownPlans(document))
	}
	if plan.Delivered() {
		return nil, fmt.Errorf("plan %q was delivered on %s and no longer changes; write a new Plan for further work", planID, plan.DeliveredAt.Format("2006-01-02"))
	}
	return plan, nil
}

func knownPlans(document *saga.Saga) string {
	if len(document.Plans) == 0 {
		return "; this Saga has no Plans yet"
	}
	ids := make([]string, 0, len(document.Plans))
	for _, plan := range document.Plans {
		ids = append(ids, plan.ID)
	}
	return "; plans: " + strings.Join(ids, ", ")
}

func writePlan(plan *saga.Plan) error {
	return store.WriteJSON(filepath.Join(plan.Directory, saga.PlanManifestName), plan.PlanManifest, false)
}

func writeTask(task *saga.Task) error {
	return store.WriteJSON(filepath.Join(task.Directory, saga.TaskManifestName), task.TaskManifest, false)
}

// Create writes a new Plan: plan.json and its deck record.
func Create(root string, manifest saga.PlanManifest, deck saga.DeckManifest) error {
	manifest.Schema, manifest.Version = saga.PlanSchemaURL, saga.PlanVersion
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	if err := saga.ValidatePlanManifest(manifest); err != nil {
		return err
	}
	deck.Version, deck.Role = saga.DeckRecordVersion, saga.DeckRolePlan
	if !saga.ValidID(deck.ID) || strings.TrimSpace(deck.Title) == "" || strings.TrimSpace(deck.Objective) == "" || utf8.RuneCountInString(deck.Objective) > 240 {
		return fmt.Errorf("the plan deck needs a stable id, a title, and an objective of 1 to 240 characters")
	}
	return mutate(root, func(document *saga.Saga) error {
		if document.FindPlan(manifest.ID) != nil {
			return fmt.Errorf("plan %q already exists", manifest.ID)
		}
		deckTarget := saga.PlanDeckTarget(document.Manifest.ID, manifest.ID, deck.ID)
		name, err := saga.FlatDeckFilename(deckTarget, deck.Rank)
		if err != nil {
			return err
		}
		plans, err := store.EnsureDirWithin(document.Root, filepath.Join(document.Root, saga.PlansDir))
		if err != nil {
			return err
		}
		dir := filepath.Join(plans, manifest.ID+saga.PlanSuffix)
		err = store.CommitDir(document.Root, dir, func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(stage, saga.PlanManifestName), manifest, true); err != nil {
				return err
			}
			if err := os.Mkdir(filepath.Join(stage, saga.PlanDeckDir), 0o755); err != nil {
				return err
			}
			return store.WriteJSON(filepath.Join(stage, saga.PlanDeckDir, name), deck, true)
		})
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("plan %q already exists", manifest.ID)
		}
		return err
	})
}

// AddPhase appends a phase to the Plan's order.
// AddPhase adds a phase at the end, or right after the phase named by after.
func AddPhase(root, planID string, phase saga.PlanPhase, after string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		at := len(plan.Phases)
		if after != "" {
			index := slices.IndexFunc(plan.Phases, func(existing saga.PlanPhase) bool { return existing.ID == after })
			if index < 0 {
				return fmt.Errorf("phase %q is not a phase of plan %q", after, planID)
			}
			at = index + 1
		}
		plan.Phases = slices.Insert(plan.Phases, at, phase)
		if err := saga.ValidatePlanManifest(plan.PlanManifest); err != nil {
			return err
		}
		return writePlan(plan)
	})
}

// RemovePhase drops a phase no Task is in, so a Plan's phases can be reordered
// by adding them again.
func RemovePhase(root, planID, phaseID string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(plan.Phases, func(phase saga.PlanPhase) bool { return phase.ID == phaseID })
		if index < 0 {
			return fmt.Errorf("phase %q is not a phase of plan %q", phaseID, planID)
		}
		for _, task := range plan.Tasks {
			if task.Phase == phaseID {
				return fmt.Errorf("phase %q still holds task %s; move it first with task set --phase", phaseID, task.ID)
			}
		}
		plan.Phases = slices.Delete(plan.Phases, index, index+1)
		return writePlan(plan)
	})
}

// LinkReview records a pull request review that implements the Plan.
func LinkReview(root, planID, reviewID string) error {
	return LinkCheckpointReview(root, planID, reviewID, "")
}

// LinkCheckpointReview links a review to the Plan and, given a checkpoint, names
// it as that checkpoint's review.
func LinkCheckpointReview(root, planID, reviewID, checkpointID string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		if document.FindReview(reviewID) == nil {
			return fmt.Errorf("review %q does not exist", reviewID)
		}
		if checkpointID != "" {
			phase, err := checkpointOf(plan, checkpointID)
			if err != nil {
				return err
			}
			phase.Checkpoint.Review = reviewID
		}
		if !slices.Contains(plan.Reviews, reviewID) {
			plan.Reviews = append(plan.Reviews, reviewID)
		}
		return writePlan(plan)
	})
}

func checkpointOf(plan *saga.Plan, checkpointID string) (*saga.PlanPhase, error) {
	for index := range plan.Phases {
		if plan.Phases[index].ID == checkpointID {
			if !plan.Phases[index].IsCheckpoint() {
				return nil, fmt.Errorf("phase %q is not a checkpoint", checkpointID)
			}
			return &plan.Phases[index], nil
		}
	}
	return nil, fmt.Errorf("checkpoint %q is not a phase of plan %q", checkpointID, plan.ID)
}

// SetCheckpointPasser says who passes the Plan's checkpoints.
func SetCheckpointPasser(root, planID, passer string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		plan.CheckpointPasser = passer
		if err := saga.ValidatePlanManifest(plan.PlanManifest); err != nil {
			return err
		}
		return writePlan(plan)
	})
}

// PassCheckpoint records a checkpoint's pass. It is refused until every Task it
// covers is done and every earlier checkpoint has passed, and refused to an agent
// unless whoever invoked the Plan lets agents pass its checkpoints.
func PassCheckpoint(root, planID, checkpointID string, pass saga.CheckpointPass) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		phase, err := checkpointOf(plan, checkpointID)
		if err != nil {
			return err
		}
		if phase.CheckpointPassed() {
			return fmt.Errorf("checkpoint %q already passed on %s", checkpointID, phase.Checkpoint.Passed.At.Format("2006-01-02"))
		}
		if pass.Agent && plan.CheckpointPasser != saga.CheckpointPasserAgent {
			return fmt.Errorf("whoever invokes plan %q passes its checkpoints, not an agent; ask them, or they can let agents with plan set --checkpoint-passer agent", planID)
		}
		reasons := []string{}
		for _, earlier := range plan.Phases {
			if earlier.ID == checkpointID {
				break
			}
			if earlier.IsCheckpoint() && !earlier.CheckpointPassed() {
				reasons = append(reasons, fmt.Sprintf("checkpoint %s before it has not passed", earlier.ID))
			}
		}
		for _, task := range saga.CheckpointScope(plan.Phases, plan.Manifests(), checkpointID) {
			if task.State != saga.TaskDone {
				reasons = append(reasons, fmt.Sprintf("task %s is %s", task.ID, task.State))
			}
		}
		if len(reasons) > 0 {
			return fmt.Errorf("checkpoint %q cannot pass yet: %s", checkpointID, strings.Join(reasons, "; "))
		}
		after := false
		pass.ToCome = nil
		for _, later := range plan.Phases {
			if later.ID == checkpointID {
				after = true
				continue
			}
			if !after {
				continue
			}
			for _, task := range plan.Tasks {
				if task.Phase == later.ID {
					pass.ToCome = append(pass.ToCome, saga.PlannedTask{Task: task.ID, Phase: later.ID})
				}
			}
		}
		phase.Checkpoint.Passed = &pass
		if err := saga.ValidatePlanManifest(plan.PlanManifest); err != nil {
			return err
		}
		return writePlan(plan)
	})
}

// AddPrototype records a prototype that shows what the Plan builds.
func AddPrototype(root, planID, prototype string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		if slices.Contains(plan.Prototypes, prototype) {
			return nil
		}
		plan.Prototypes = append(plan.Prototypes, prototype)
		return writePlan(plan)
	})
}

// Deliver marks the Plan delivered. It is refused unless every Task is done and
// every linked Review has merged, which merged decides. The reasons name what is
// still outstanding.
func Deliver(root, planID string, merged func(*saga.Review) bool) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		reasons := []string{}
		for _, task := range plan.Tasks {
			if task.State != saga.TaskDone {
				reasons = append(reasons, fmt.Sprintf("task %s is %s", task.ID, task.State))
			}
		}
		if len(plan.Reviews) == 0 {
			reasons = append(reasons, "no Review is linked; add one with plan link-review")
		}
		for _, phase := range plan.Phases {
			if phase.IsCheckpoint() && !phase.CheckpointPassed() {
				reasons = append(reasons, fmt.Sprintf("checkpoint %s has not passed", phase.ID))
			}
		}
		for _, id := range plan.Reviews {
			review := document.FindReview(id)
			if review == nil {
				reasons = append(reasons, fmt.Sprintf("review %s does not exist", id))
			} else if !merged(review) {
				reasons = append(reasons, fmt.Sprintf("review %s has not merged", id))
			}
		}
		if len(reasons) > 0 {
			return fmt.Errorf("plan %q cannot be delivered yet:\n  %s", planID, strings.Join(reasons, "\n  "))
		}
		now := time.Now().UTC()
		plan.DeliveredAt = &now
		return writePlan(plan)
	})
}

// refuse turns problems in a candidate graph into one error naming every Task.
func refuse(problems []saga.TaskProblem) error {
	if len(problems) == 0 {
		return nil
	}
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		lines = append(lines, "task "+problem.Task+": "+problem.Message)
	}
	return fmt.Errorf("refused, because the Plan's tasks would be left like this:\n  %s", strings.Join(lines, "\n  "))
}

// replaced returns the Plan's Task manifests with changed standing in for the
// Task of the same id.
func replaced(plan *saga.Plan, changed saga.TaskManifest) []saga.TaskManifest {
	manifests := plan.Manifests()
	for index := range manifests {
		if manifests[index].ID == changed.ID {
			manifests[index] = changed
		}
	}
	return manifests
}

// AddTask writes a new Task, and makes each Task in unblocks depend on it. The
// resulting graph is checked first, so a Task with no story of its own must be
// added with something to unblock.
func AddTask(root, planID string, task saga.TaskManifest, unblocks []string, essay string) (string, error) {
	task.Schema, task.Version = saga.TaskSchemaURL, saga.PlanVersion
	if task.State == "" {
		task.State = saga.TaskPlanned
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	var created string
	err := mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		if err := saga.ValidateTaskManifest(document.Manifest.ID, task); err != nil {
			return err
		}
		if plan.FindTask(task.ID) != nil {
			return fmt.Errorf("task %q already exists in plan %q", task.ID, planID)
		}
		candidate := append(plan.Manifests(), task)
		dependents := make([]*saga.Task, 0, len(unblocks))
		for _, id := range unblocks {
			dependent := plan.FindTask(id)
			if dependent == nil {
				return fmt.Errorf("--unblocks %q is not a task of plan %q", id, planID)
			}
			if !slices.Contains(dependent.DependsOn, task.ID) {
				dependent.DependsOn = append(dependent.DependsOn, task.ID)
			}
			dependents = append(dependents, dependent)
			for index := range candidate {
				if candidate[index].ID == id {
					candidate[index] = dependent.TaskManifest
				}
			}
		}
		if err := refuse(saga.PlanTaskProblems(plan.Phases, candidate)); err != nil {
			return err
		}
		tasks, err := store.EnsureDirWithin(document.Root, filepath.Join(plan.Directory, saga.PlanTasksDir))
		if err != nil {
			return err
		}
		dir := filepath.Join(tasks, task.ID+saga.TaskSuffix)
		err = store.CommitDir(document.Root, dir, func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(stage, saga.TaskManifestName), task, true); err != nil {
				return err
			}
			if essay == "" {
				return nil
			}
			return store.WriteFile(filepath.Join(stage, saga.TaskEssayName), []byte(essay), 0o644, true)
		})
		if err != nil {
			return err
		}
		for _, dependent := range dependents {
			if err := writeTask(dependent); err != nil {
				return err
			}
		}
		created = saga.TaskTarget(document.Manifest.ID, planID, task.ID)
		return nil
	})
	return created, err
}

// UpdateTask applies change to one Task and checks the resulting graph. essay,
// when not nil, replaces the essay; an empty essay removes it.
func UpdateTask(root, planID, taskID string, change func(*saga.TaskManifest) error, essay *string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		task := plan.FindTask(taskID)
		if task == nil {
			return fmt.Errorf("task %q does not exist in plan %q", taskID, planID)
		}
		if err := change(&task.TaskManifest); err != nil {
			return err
		}
		if err := saga.ValidateTaskManifest(document.Manifest.ID, task.TaskManifest); err != nil {
			return err
		}
		if err := refuse(saga.PlanTaskProblems(plan.Phases, replaced(plan, task.TaskManifest))); err != nil {
			return err
		}
		if err := writeTask(task); err != nil {
			return err
		}
		if essay == nil {
			return nil
		}
		path := filepath.Join(task.Directory, saga.TaskEssayName)
		if *essay == "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}
		return store.WriteFile(path, []byte(*essay), 0o644, false)
	})
}

// RemoveTask deletes a Task and every dependency on it. It is refused when that
// would strand another Task, or when a slide still names the Task.
func RemoveTask(root, planID, taskID string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		task := plan.FindTask(taskID)
		if task == nil {
			return fmt.Errorf("task %q does not exist in plan %q", taskID, planID)
		}
		if plan.Deck != nil {
			for _, slide := range plan.Deck.Slides {
				for _, item := range slide.Items {
					if item.Record == task.Target {
						return fmt.Errorf("slide %s still names task %s; change its Item first", slide.ID, taskID)
					}
				}
			}
		}
		candidate := []saga.TaskManifest{}
		dependents := []*saga.Task{}
		for _, other := range plan.Tasks {
			if other.ID == task.ID {
				continue
			}
			if slices.Contains(other.DependsOn, task.ID) {
				other.DependsOn = without(other.DependsOn, task.ID)
				dependents = append(dependents, other)
			}
			candidate = append(candidate, other.TaskManifest)
		}
		if err := refuse(saga.PlanTaskProblems(plan.Phases, candidate)); err != nil {
			return err
		}
		for _, dependent := range dependents {
			if err := writeTask(dependent); err != nil {
				return err
			}
		}
		return os.RemoveAll(task.Directory)
	})
}

func without(values []string, value string) []string {
	result := make([]string, 0, len(values))
	for _, candidate := range values {
		if candidate != value {
			result = append(result, candidate)
		}
	}
	return result
}

// WidenCheckpointReview makes a checkpoint's review show every change since an
// earlier checkpoint's review began, or, given the checkpoint itself, only its own
// stretch again. It changes what is reviewed, never what the review merges into.
func WidenCheckpointReview(root, planID, checkpointID, toID string) error {
	return mutate(root, func(document *saga.Saga) error {
		plan, err := editable(document, planID)
		if err != nil {
			return err
		}
		phase, err := checkpointOf(plan, checkpointID)
		if err != nil {
			return err
		}
		review := document.FindReview(phase.Checkpoint.Review)
		if review == nil {
			return fmt.Errorf("checkpoint %q has no review yet; link one with plan link-review --checkpoint %s", checkpointID, checkpointID)
		}
		if review.Merged != nil {
			return fmt.Errorf("review %q has merged and no longer changes", review.ID)
		}
		since := ""
		if toID != checkpointID {
			to, err := checkpointOf(plan, toID)
			if err != nil {
				return err
			}
			if slices.IndexFunc(plan.Phases, func(p saga.PlanPhase) bool { return p.ID == toID }) > slices.IndexFunc(plan.Phases, func(p saga.PlanPhase) bool { return p.ID == checkpointID }) {
				return fmt.Errorf("checkpoint %q comes after %q; a review widens back to an earlier one", toID, checkpointID)
			}
			if !to.CheckpointPassed() || to.Checkpoint.Passed.Base == "" {
				return fmt.Errorf("checkpoint %q did not record where its review began when it passed, so there is nothing to widen back to", toID)
			}
			since = to.Checkpoint.Passed.Base
		}
		manifest := review.ReviewManifest
		manifest.Since = since
		if err := saga.ValidateReviewManifest(manifest); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(review.Directory, saga.ReviewManifestName), manifest, false)
	})
}
