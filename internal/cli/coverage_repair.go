package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

type coverageRepairOutput struct {
	OK            bool     `json:"ok"`
	Action        string   `json:"action"`
	Removed       string   `json:"removed"`
	DryRun        bool     `json:"dry_run"`
	Records       int      `json:"records"`
	References    int      `json:"references"`
	EvidenceFiles []string `json:"evidence_files"`
}

type coverageRecordLocation struct {
	relative string
	absolute string
}

func RemoveCoverage(ctx context.Context, args []string, out io.Writer) error {
	err := removeCoverage(ctx, args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportJSONMutationFailure(out, err)
	}
	return err
}

func removeCoverage(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("remove-coverage", commandUsage["remove-coverage"], out)
	record := flags.String("record", "", "evidence_file returned by query mappings or fragment-diffs")
	dryRun := flags.Bool("dry-run", false, "validate and report the deletion without writing")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable result")
	quiet := flags.Bool("quiet", false, "suppress successful output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(*record) == "" {
		return fmt.Errorf("usage: %s", commandUsage["remove-coverage"])
	}
	if *jsonOutput && *quiet {
		return fmt.Errorf("--json and --quiet cannot be combined")
	}
	root := flags.Arg(0)
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	location, err := locateCoverageRecord(document, *record)
	if err != nil {
		return err
	}
	result := coverageRepairOutput{OK: true, Action: "remove", Removed: location.relative, DryRun: *dryRun}
	if !*dryRun {
		err = authorMutation(root, func(locked *saga.Saga) error {
			current, locateErr := locateCoverageRecord(locked, *record)
			if locateErr != nil {
				return locateErr
			}
			return removeCoverageFile(current.absolute)
		})
		if err != nil {
			return err
		}
	}
	return writeCoverageRepairResult(out, result, *jsonOutput, *quiet)
}

func ReplaceCoverage(ctx context.Context, args []string, out io.Writer) error {
	err := replaceCoverage(ctx, args, out, os.Stdin)
	if err != nil && jsonFlagRequested(args) {
		return reportJSONMutationFailure(out, err)
	}
	return err
}

func replaceCoverage(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	flags := commandFlags("replace-coverage", commandUsage["replace-coverage"], out)
	recordPath := flags.String("record", "", "evidence_file returned by query mappings or fragment-diffs")
	options := registerCoverFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(*recordPath) == "" {
		return fmt.Errorf("usage: %s", commandUsage["replace-coverage"])
	}
	if *options.jsonOutput && *options.quiet {
		return fmt.Errorf("--json and --quiet cannot be combined")
	}
	root := flags.Arg(0)
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	old, err := locateCoverageRecord(document, *recordPath)
	if err != nil {
		return err
	}
	records, err := coverRecords(*options.batch, stdin, options.record(), flags)
	if err != nil {
		return err
	}
	files, err := buildCoverageFiles(ctx, document, records, *options.repoDir, *options.allowMismatch)
	if err != nil {
		return err
	}
	var planned []plannedRecord
	plan := func(locked *saga.Saga, replaceable string) error {
		var planErr error
		planned, planErr = planCoverage(locked, records, files, replaceable)
		return planErr
	}
	if *options.dryRun {
		if err := plan(document, old.absolute); err != nil {
			return err
		}
		result := repairOutput("replace", old.relative, planned, true)
		return writeCoverageRepairResult(out, result, *options.jsonOutput, *options.quiet)
	}
	err = authorMutation(root, func(locked *saga.Saga) error {
		current, locateErr := locateCoverageRecord(locked, *recordPath)
		if locateErr != nil {
			return locateErr
		}
		if planErr := plan(locked, current.absolute); planErr != nil {
			return planErr
		}
		if ensureErr := ensureCoverageDirectories(locked.Root, planned); ensureErr != nil {
			return ensureErr
		}
		return replaceCoverageFile(current.absolute, planned)
	})
	if err != nil {
		return err
	}
	return writeCoverageRepairResult(out, repairOutput("replace", old.relative, planned, false), *options.jsonOutput, *options.quiet)
}

func locateCoverageRecord(document *saga.Saga, requested string) (coverageRecordLocation, error) {
	if filepath.IsAbs(requested) {
		return coverageRecordLocation{}, fmt.Errorf("--record must be the relative evidence_file returned by query")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(requested)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return coverageRecordLocation{}, fmt.Errorf("--record must stay within the saga")
	}
	var found string
	consider := func(files []saga.CodeFile) {
		for _, file := range files {
			if filepath.ToSlash(file.Path) == clean {
				found = clean
			}
		}
	}
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		consider(section.Code)
		for _, fragment := range section.Fragments {
			consider(fragment.Code)
			for index := range fragment.Landmarks {
				consider(fragment.Landmarks[index].Code)
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	if found == "" {
		return coverageRecordLocation{}, fmt.Errorf("coverage record %q does not exist; use query mappings to list evidence_file values", requested)
	}
	return coverageRecordLocation{relative: found, absolute: filepath.Join(document.Root, filepath.FromSlash(found))}, nil
}

func removeCoverageFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := store.SyncDir(filepath.Dir(path)); err != nil {
		_ = store.WriteFile(path, data, 0o644, true)
		return err
	}
	return nil
}

func replaceCoverageFile(oldPath string, planned []plannedRecord) error {
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	var written []plannedRecord
	rollback := func() {
		for _, record := range written {
			if canonicalCoveragePath(record.path) != canonicalCoveragePath(oldPath) {
				_ = os.Remove(record.path)
				_ = store.SyncDir(record.dir)
			}
		}
		_ = store.WriteFile(oldPath, oldData, 0o644, false)
	}
	for _, record := range planned {
		exclusive := canonicalCoveragePath(record.path) != canonicalCoveragePath(oldPath)
		if err := store.WriteJSON(record.path, record.file, exclusive); err != nil {
			rollback()
			return fmt.Errorf("write replacement coverage: %w; original record restored", err)
		}
		written = append(written, record)
	}
	keptOldPath := false
	for _, record := range planned {
		keptOldPath = keptOldPath || canonicalCoveragePath(record.path) == canonicalCoveragePath(oldPath)
	}
	if !keptOldPath {
		if err := os.Remove(oldPath); err != nil {
			rollback()
			return fmt.Errorf("remove replaced coverage: %w; original record restored", err)
		}
		if err := store.SyncDir(filepath.Dir(oldPath)); err != nil {
			rollback()
			return fmt.Errorf("commit replacement coverage: %w; original record restored", err)
		}
	}
	return nil
}

func repairOutput(action, removed string, planned []plannedRecord, dryRun bool) coverageRepairOutput {
	base := coverageOutput(planned, dryRun)
	return coverageRepairOutput{OK: true, Action: action, Removed: removed, DryRun: dryRun, Records: base.Records, References: base.References, EvidenceFiles: base.EvidenceFiles}
}

func writeCoverageRepairResult(out io.Writer, result coverageRepairOutput, jsonOutput, quiet bool) error {
	if quiet {
		return nil
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	verb := map[bool]string{true: "Would remove", false: "Removed"}[result.DryRun]
	if result.Action == "replace" {
		verb = map[bool]string{true: "Would replace", false: "Replaced"}[result.DryRun]
	}
	fmt.Fprintf(out, "%s %s", verb, result.Removed)
	if result.Action == "replace" {
		fmt.Fprintf(out, " with %d coverage record(s)", result.Records)
	}
	fmt.Fprintln(out)
	return nil
}
