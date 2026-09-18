package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// A v3 upgrade changes only the container contract. Requirements, design, and
// work-plan capabilities are adopted lazily by their owning mutations so an
// ordinary review-only Saga does not acquire unused structure.
var livingRootDirectories = []string{}

// upgradeFaultHook is a package-local test seam for proving that every failure
// before commit restores the original v2 tree.
var upgradeFaultHook func(string) error

func Upgrade(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("upgrade", commandUsage["upgrade"], out)
	target := flags.Int("to", 0, "target Saga format version (3 or 5)")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	dryRun := flags.Bool("dry-run", false, "stage and validate the upgrade, report capability states, and publish nothing")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || !flagWasSet(flags, "to") {
		return fmt.Errorf("usage: %s", commandUsage["upgrade"])
	}
	if *target != saga.CurrentSagaVersion && *target != saga.ReportV5SagaVersion {
		return fmt.Errorf("--to must be %d or %d", saga.CurrentSagaVersion, saga.ReportV5SagaVersion)
	}

	root := flags.Arg(0)
	var from int
	var report upgradeReport
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, validation, err := saga.Load(root)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("cannot upgrade an invalid Saga: %s", validationSummary(validation))
		}
		from = document.Manifest.Version
		if err := checkUpgradeTransition(from, *target); err != nil {
			return err
		}

		parent := filepath.Dir(document.Root)
		stage, err := os.MkdirTemp(parent, ".change-saga-upgrade-*.saga")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		if err := copySagaForUpgrade(document.Root, stage); err != nil {
			return fmt.Errorf("stage Saga upgrade: %w", err)
		}

		// Only the manifest version and schema identifier change. Every
		// component record is byte-for-byte the staged copy of the original.
		upgraded := document.Manifest
		upgraded.Version = *target
		upgraded.Schema = saga.SagaSchemaURL(*target)
		if err := store.WriteJSON(filepath.Join(stage, "saga.json"), upgraded, false); err != nil {
			return err
		}
		for _, name := range livingRootDirectories {
			if err := os.Mkdir(filepath.Join(stage, name), 0o755); err != nil {
				return err
			}
		}
		if err := syncUpgradeTree(stage); err != nil {
			return err
		}
		if hook := upgradeFaultHook; hook != nil {
			if err := hook("after-stage"); err != nil {
				return err
			}
		}
		if from == saga.LegacySagaVersion {
			// The v2 -> v3 path is intentionally unchanged: the container
			// contract is the only thing it adopts.
			_, stagedValidation, err := saga.Load(stage)
			if err != nil {
				return fmt.Errorf("validate staged Saga upgrade: %w", err)
			}
			if !stagedValidation.Valid {
				return fmt.Errorf("staged Saga upgrade is invalid: %s", validationSummary(stagedValidation))
			}
		} else {
			report = validateReportContainerStage(stage, document.Manifest.ID, *target)
			if *dryRun {
				return nil
			}
			if len(report.Blockers) > 0 {
				return fmt.Errorf("staged Saga %s is invalid: %s", upgradeVerb(from, *target), strings.Join(report.Blockers, "; "))
			}
		}
		if *dryRun {
			return nil
		}
		return publishSagaUpgrade(document.Root, stage)
	})
	if err != nil {
		return err
	}
	if from == saga.LegacySagaVersion {
		if *dryRun {
			if *jsonOutput {
				return writeJSON(out, map[string]any{"ok": true, "operation": "upgrade", "dry_run": true, "from": from, "to": *target, "created": livingRootDirectories})
			}
			fmt.Fprintf(out, "Dry run: %s can upgrade from Saga format v%d to v%d; nothing was written\n", root, from, *target)
			return nil
		}
		if *jsonOutput {
			return writeJSON(out, map[string]any{
				"ok": true, "operation": "upgrade", "from": saga.LegacySagaVersion, "to": saga.CurrentSagaVersion,
				"created": livingRootDirectories,
			})
		}
		fmt.Fprintf(out, "Upgraded %s from Saga format v%d to v%d\n", root, saga.LegacySagaVersion, saga.CurrentSagaVersion)
		return nil
	}

	operation := "upgrade"
	if *target < from {
		operation = "downgrade"
	}
	if *jsonOutput {
		if err := writeJSON(out, map[string]any{
			"ok": len(report.Blockers) == 0, "operation": operation, "dry_run": *dryRun, "from": from, "to": *target,
			"capabilities": report.Capabilities, "blockers": report.Blockers,
		}); err != nil {
			return err
		}
	} else {
		switch {
		case *dryRun && len(report.Blockers) > 0:
			fmt.Fprintf(out, "Dry run: %s cannot %s from Saga format v%d to v%d; nothing was written\n", root, operation, from, *target)
			for _, blocker := range report.Blockers {
				fmt.Fprintf(out, "  blocker: %s\n", blocker)
			}
		case *dryRun:
			fmt.Fprintf(out, "Dry run: %s can %s from Saga format v%d to v%d; nothing was written\n", root, operation, from, *target)
		default:
			fmt.Fprintf(out, "%sd %s from Saga format v%d to v%d\n", strings.ToUpper(operation[:1])+operation[1:], root, from, *target)
		}
		for _, capability := range report.Capabilities {
			fmt.Fprintf(out, "  %s: %s\n", capability.Name, capability.State)
		}
	}
	if len(report.Blockers) > 0 {
		return fmt.Errorf("dry run found %d blocking issue(s)", len(report.Blockers))
	}
	return nil
}

// checkUpgradeTransition admits exactly the explicit transitions this command
// owns: v2 -> v3, v3 -> v5, and the reversible v5 -> v3 downgrade. v2 is not
// chained to v5 so each container change stays a separate, reviewable step,
// and v4 slide-native Sagas are never converted into a v5 report.
func checkUpgradeTransition(from, to int) error {
	if from == to {
		return fmt.Errorf("Saga is already format v%d", to)
	}
	switch {
	case from == saga.LegacySagaVersion && to == saga.CurrentSagaVersion:
		return nil
	case from == saga.CurrentSagaVersion && to == saga.ReportV5SagaVersion:
		return nil
	case from == saga.ReportV5SagaVersion && to == saga.CurrentSagaVersion:
		return nil
	case from == saga.LegacySagaVersion && to == saga.ReportV5SagaVersion:
		return fmt.Errorf("cannot upgrade Saga format v2 directly to v5; run change-saga upgrade --to 3 first, then upgrade --to 5")
	case from == saga.SlideSagaVersion && to == saga.ReportV5SagaVersion:
		return fmt.Errorf("cannot upgrade slide-native Saga format v4 to v5; v5 is a report container and embeds v4 decks under %s instead", saga.EmbeddedSlidesDir)
	default:
		return fmt.Errorf("cannot upgrade Saga format v%d to v%d", from, to)
	}
}

func upgradeVerb(from, to int) string {
	if to < from {
		return "downgrade"
	}
	return "upgrade"
}

type upgradeCapability struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type upgradeReport struct {
	Capabilities []upgradeCapability `json:"capabilities"`
	Blockers     []string            `json:"blockers"`
}

// validateReportContainerStage validates a staged v3 <-> v5 transition under
// the target composition rules. The core loader checks the container and
// reserved roots; every owning component loader then strictly reads its own
// root so an upgrade can never publish a Saga some reader would refuse. A
// downgrade additionally proves no v5-only record would be stranded: the v3
// rules reject ___quality, coverage exceptions, and non-v3 relations, and the
// copy is never allowed to discard them.
func validateReportContainerStage(stage, sagaID string, to int) upgradeReport {
	report := upgradeReport{Capabilities: []upgradeCapability{}, Blockers: []string{}}
	if to == saga.CurrentSagaVersion {
		for _, path := range []string{saga.QualityRootDir, filepath.Join("___requirements", "coverage-exceptions")} {
			if _, err := os.Lstat(filepath.Join(stage, path)); err == nil {
				report.Blockers = append(report.Blockers, fmt.Sprintf("%s is a v5-only record root; a downgrade never discards records", filepath.ToSlash(path)))
			}
		}
	}
	document, validation, err := saga.Load(stage)
	if err != nil {
		report.Blockers = append(report.Blockers, err.Error())
		return report
	}
	for _, issue := range validation.Issues {
		if issue.Severity == "error" {
			report.Blockers = append(report.Blockers, issue.Path+": "+issue.Message)
		}
	}
	rootState := func(name string) string {
		if info, err := os.Lstat(filepath.Join(stage, name)); err == nil && info.IsDir() {
			return "adopted"
		}
		return "not_adopted"
	}
	add := func(name, state string) {
		report.Capabilities = append(report.Capabilities, upgradeCapability{Name: name, State: state})
	}
	if _, err := requirements.Load(stage, sagaID); err != nil {
		report.Blockers = append(report.Blockers, "requirements: "+err.Error())
	}
	add("requirements", rootState("___requirements"))
	if _, err := prototypes.Load(stage, sagaID); err != nil {
		report.Blockers = append(report.Blockers, "prototypes: "+err.Error())
	}
	add("prototypes", rootState(filepath.Join("___requirements", "prototypes")))
	add("design", rootState("___design"))
	if _, planValidation, err := workplan.Load(stage); err != nil {
		report.Blockers = append(report.Blockers, "work plan: "+err.Error())
	} else {
		for _, issue := range planValidation.Issues {
			if issue.Severity == "error" {
				report.Blockers = append(report.Blockers, "work plan: "+issue.Path+": "+issue.Message)
			}
		}
	}
	add("workplan", rootState("___workplan"))
	slides := "not_adopted"
	if document != nil && len(document.Decks) > 0 {
		slides = fmt.Sprintf("adopted (%d embedded deck(s))", len(document.Decks))
	}
	add("slides", slides)
	if to == saga.ReportV5SagaVersion {
		add("coverage_exceptions", rootState(filepath.Join("___requirements", "coverage-exceptions")))
		qualityDocument, err := quality.Load(stage)
		if err != nil {
			report.Blockers = append(report.Blockers, "quality: "+err.Error())
		} else {
			add("quality", string(qualityDocument.Adoption))
		}
	}
	return report
}

func validationSummary(validation saga.Validation) string {
	var messages []string
	for _, issue := range validation.Issues {
		if issue.Severity == "error" {
			messages = append(messages, issue.Path+": "+issue.Message)
		}
	}
	if len(messages) == 0 {
		return "validation failed"
	}
	return strings.Join(messages, "; ")
}

// publishSagaUpgrade commits the already-validated staged manifest and roots
// as one rollback-safe transaction. The old manifest remains available until
// every new entry is durable, so any reported failure restores the exact v2
// manifest and removes every root created by this operation.
func publishSagaUpgrade(root, stage string) (result error) {
	backup, err := os.CreateTemp(root, ".change-saga-upgrade-manifest-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}

	created := make([]string, 0, len(livingRootDirectories))
	manifestBackedUp := false
	manifestPublished := false
	defer func() {
		if result == nil {
			return
		}
		if manifestPublished {
			_ = os.Remove(filepath.Join(root, "saga.json"))
		}
		if manifestBackedUp {
			_ = os.Rename(backupPath, filepath.Join(root, "saga.json"))
		}
		for index := len(created) - 1; index >= 0; index-- {
			_ = os.Remove(filepath.Join(root, created[index]))
		}
		_ = os.Remove(backupPath)
		_ = store.SyncDir(root)
	}()

	for _, name := range livingRootDirectories {
		if err := os.Rename(filepath.Join(stage, name), filepath.Join(root, name)); err != nil {
			return err
		}
		created = append(created, name)
		if hook := upgradeFaultHook; hook != nil {
			if err := hook("after-root:" + name); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(filepath.Join(root, "saga.json"), backupPath); err != nil {
		return err
	}
	manifestBackedUp = true
	if hook := upgradeFaultHook; hook != nil {
		if err := hook("before-manifest"); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(stage, "saga.json"), filepath.Join(root, "saga.json")); err != nil {
		return err
	}
	manifestPublished = true
	if hook := upgradeFaultHook; hook != nil {
		if err := hook("after-manifest"); err != nil {
			return err
		}
	}
	if err := store.SyncDir(root); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	manifestBackedUp = false
	manifestPublished = false
	// The complete v3 tree was synced before removing the private backup. A
	// cleanup sync is best effort: failing it must not report a rolled-back
	// operation after the commit point has passed.
	_ = store.SyncDir(root)
	return nil
}

func copySagaForUpgrade(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			targetValue, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(targetValue, filepath.Join(target, relative))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.Mkdir(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", filepath.ToSlash(relative))
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		if copyErr == nil {
			copyErr = output.Sync()
		}
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func syncUpgradeTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Deep paths first ensures every child entry is durable before its parent.
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := store.SyncDir(directory); err != nil {
			return err
		}
	}
	return nil
}
