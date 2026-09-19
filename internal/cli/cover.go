package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// coverRecord is one coverage instruction. Its fields are the batch spelling of
// the cover flags, so an author who already knows the flags does not learn a
// second vocabulary to use stdin. A record references code exactly as a single
// invocation does; batching changes how many instructions are delivered, never
// what each one references.
type coverRecord struct {
	Target       string   `json:"target,omitempty"`
	Path         string   `json:"path,omitempty"`
	Side         string   `json:"side,omitempty"`
	Lines        string   `json:"lines,omitempty"`
	ChangedLines bool     `json:"changed_lines,omitempty"`
	File         bool     `json:"file,omitempty"`
	Commit       string   `json:"commit,omitempty"`
	Refs         []string `json:"refs,omitempty"`
	Note         string   `json:"note,omitempty"`
	Name         string   `json:"name,omitempty"`
}

// needsComparison reports whether the record addresses the comparison's
// sides instead of naming explicit commits.
func (record coverRecord) needsComparison() bool {
	return record.ChangedLines || record.Path != "" && record.Commit == ""
}

// plannedRecord is a fully resolved write that has not happened yet. Planning
// every record before the first write is what makes a batch all-or-nothing: a
// record that cannot be resolved fails the whole batch while the saga is still
// untouched.
type plannedRecord struct {
	targetID string
	file     saga.CodeFile
	dir      string
	path     string
	relative string
}

type coverageMutationOutput struct {
	OK            bool     `json:"ok"`
	DryRun        bool     `json:"dry_run"`
	Records       int      `json:"records"`
	References    int      `json:"references"`
	EvidenceFiles []string `json:"evidence_files"`
}

// coverFlags are the reference-selecting flags cover and replace-coverage
// share.
type coverFlags struct {
	target, repoDir, path, side, lines, commit, note, name, batch *string
	changedLines, file, dryRun, jsonOutput, quiet, allowMismatch  *bool
	refs                                                          stringList
	opening                                                       *openFlags
}

func registerCoverFlags(flags *flag.FlagSet) *coverFlags {
	value := &coverFlags{
		target:        flags.String("target", ".", "section, .fragment, landmark, or Item receiving evidence; accepts <fragment>#<landmark-id>"),
		repoDir:       flags.String("repo", "", "source repository checkout; required when separate"),
		path:          flags.String("path", "", "repository path"),
		side:          flags.String("side", "", "comparison side: new (the head commit) or old (the merge-base, for deleted code)"),
		lines:         flags.String("lines", "", "line ranges on that side, for example 4-9,12"),
		changedLines:  flags.Bool("changed-lines", false, "reference every changed line of --path, and the whole file for file events; optionally one --side"),
		file:          flags.Bool("file", false, "reference the whole file at --path: renames, mode and binary changes, and whole added or deleted files"),
		commit:        flags.String("commit", "", "pin at this revision instead of a comparison side"),
		note:          flags.String("note", "", "optional explanation for report authors"),
		name:          flags.String("name", "", "coverage filename without .json"),
		batch:         flags.String("batch", "", "read coverage records from a JSON file, or - for stdin"),
		dryRun:        flags.Bool("dry-run", false, "resolve and report the coverage records without writing them"),
		jsonOutput:    flags.Bool("json", false, "emit one machine-readable summary instead of every reference"),
		quiet:         flags.Bool("quiet", false, "suppress successful output"),
		allowMismatch: flags.Bool("allow-repository-mismatch", false, "use a checkout whose origin differs from the declared repository"),
	}
	flags.Var(&value.refs, "ref", "code location <commit>:<path>[#L<start>[-L<end>]], the commit any revision; repeatable")
	value.opening = registerOpenFlags(flags)
	return value
}

func (value *coverFlags) record() coverRecord {
	return coverRecord{
		Target: *value.target, Path: *value.path, Side: *value.side, Lines: *value.lines, ChangedLines: *value.changedLines,
		File: *value.file, Commit: *value.commit, Refs: value.refs, Note: *value.note, Name: *value.name,
	}
}

// Cover attaches code references to a narrative target. os.Stdin is bound
// here rather than read inside the command so tests drive --batch
// deterministically.
func Cover(ctx context.Context, args []string, out io.Writer) error {
	err := cover(ctx, args, out, os.Stdin)
	if err != nil && jsonFlagRequested(args) {
		return reportJSONMutationFailure(out, err)
	}
	return err
}

func cover(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	flags := commandFlags("cover", commandUsage["cover"], out)
	options := registerCoverFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["cover"])
	}
	if *options.jsonOutput && *options.quiet {
		return fmt.Errorf("--json and --quiet cannot be combined")
	}
	document, _, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	records, err := coverRecords(*options.batch, stdin, options.record(), flags)
	if err != nil {
		return err
	}
	files, err := buildCoverageFiles(ctx, document, records, *options.repoDir, options.opening.rng(), flagWasSet(flags, "head"), *options.allowMismatch)
	if err != nil {
		return err
	}

	var planned []plannedRecord
	plan := func(locked *saga.Saga) error {
		var planErr error
		planned, planErr = planCoverage(locked, records, files)
		return planErr
	}
	if *options.dryRun {
		// A dry run still resolves targets and names against the real saga so
		// its report matches what a real run would do, but it takes no lock and
		// creates nothing.
		if err := plan(document); err != nil {
			return err
		}
		if *options.quiet {
			return nil
		}
		if *options.jsonOutput {
			return writeJSON(out, coverageOutput(planned, true))
		}
		for _, record := range planned {
			fmt.Fprintf(out, "Would add %s (%s)\n", record.relative, record.targetID)
			for _, reference := range record.file.References {
				fmt.Fprintf(out, "  %s\n", reference.Location())
			}
		}
		fmt.Fprintf(out, "Dry run: %d coverage record(s) resolved, nothing written\n", len(planned))
		return nil
	}

	if err := authorMutation(flags.Arg(0), func(locked *saga.Saga) error {
		if err := plan(locked); err != nil {
			return err
		}
		if err := ensureCoverageDirectories(locked.Root, planned); err != nil {
			return err
		}
		return writeCoverage(planned)
	}); err != nil {
		return err
	}
	if *options.quiet {
		return nil
	}
	if *options.jsonOutput {
		return writeJSON(out, coverageOutput(planned, false))
	}
	for _, record := range planned {
		fmt.Fprintf(out, "Added %s\n", record.relative)
	}
	return nil
}

func coverageOutput(planned []plannedRecord, dryRun bool) coverageMutationOutput {
	result := coverageMutationOutput{OK: true, DryRun: dryRun, Records: len(planned), EvidenceFiles: make([]string, 0, len(planned))}
	for _, record := range planned {
		result.EvidenceFiles = append(result.EvidenceFiles, record.relative)
		result.References += len(record.file.References)
	}
	return result
}

// buildCoverageFiles resolves every record into the evidence file it will
// write. The comparison is read once for the whole batch, and before any lock
// is taken, so a slow diff neither repeats per record nor stalls other
// writers; each reference's digest is read from the repository here.
//
// A review Item explains its review's change, so with neither --against nor
// --head its records compare the review's own range: the merge-base of its
// base and the head it follows, or its frozen range after merge.
func buildCoverageFiles(ctx context.Context, document *saga.Saga, records []coverRecord, repoDir string, rng gitdiff.Range, headSet, allowMismatch bool) ([]saga.CodeFile, error) {
	checkout := firstNonEmpty(repoDir, document.Root)
	var review *saga.Review
	if rng.Observe() && !headSet {
		var err error
		if review, err = recordsReview(document, records); err != nil {
			return nil, err
		}
	}
	var changes *gitdiff.ChangeSet
	for _, record := range records {
		if !record.needsComparison() {
			continue
		}
		if record.ChangedLines && rng.Observe() && review == nil {
			return nil, fmt.Errorf("--changed-lines needs a comparison: pass --against REV, or target a review Item to compare its review's range")
		}
		options := gitdiff.ReadOptions{AllowRepositoryMismatch: allowMismatch}
		var read gitdiff.ChangeSet
		var err error
		if review != nil {
			reviewRange, rangeErr := reviewstate.ResolveRange(ctx, checkout, review)
			if rangeErr != nil {
				return nil, fmt.Errorf("compare review %s's range (or pass --against REV): %w", review.ID, rangeErr)
			}
			read, err = gitdiff.ReadWithOptions(ctx, checkout, document.Manifest.Source.Repository, reviewRange.BaseOID, reviewRange.HeadOID, options)
		} else {
			read, err = gitdiff.ReadRange(ctx, checkout, document.Manifest.Source.Repository, rng, options)
		}
		if err != nil {
			return nil, fmt.Errorf("read source comparison (use --repo for a separate saga repository): %w", err)
		}
		changes = &read
		break
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return nil, fmt.Errorf("open source repository (use --repo for a separate saga repository): %w", err)
	}
	defer resolver.Close()
	files := make([]saga.CodeFile, len(records))
	for i, record := range records {
		locations, err := recordLocations(ctx, record, changes, checkout)
		if err != nil {
			return nil, recordError(records, i, err)
		}
		file := saga.CodeFile{Version: saga.CurrentVersion}
		seen := map[string]bool{}
		for _, location := range locations {
			if seen[location.String()] {
				continue
			}
			seen[location.String()] = true
			reference, err := resolver.Author(ctx, location, record.Note)
			if err != nil {
				return nil, recordError(records, i, err)
			}
			file.References = append(file.References, reference)
		}
		files[i] = file
	}
	return files, nil
}

// recordsReview is the review whose Items the records addressing a
// comparison target, or nil when they target no review Item. Records that
// would compare different ranges are refused rather than silently read
// against one of them.
func recordsReview(document *saga.Saga, records []coverRecord) (*saga.Review, error) {
	var found *saga.Review
	other := false
	for index, record := range records {
		if !record.needsComparison() {
			continue
		}
		var review *saga.Review
		if _, target, err := resolveTarget(document, record.Target, true); err != nil {
			return nil, recordError(records, index, err)
		} else if strings.Contains(target, ":item:") {
			for _, candidate := range document.Reviews {
				if strings.HasPrefix(target, candidate.Target+":") {
					review = candidate
				}
			}
		}
		if review == nil {
			other = true
		} else if found != nil && found != review {
			return nil, fmt.Errorf("the records target Items of reviews %s and %s, whose ranges differ; cover each review separately, or pass --against REV", found.ID, review.ID)
		} else {
			found = review
		}
		if found != nil && other {
			return nil, fmt.Errorf("the records target both review %s's Items and other targets, whose comparisons differ; cover them separately, or pass --against REV", found.ID)
		}
	}
	return found, nil
}

// recordLocations turns one record into the code locations it references.
func recordLocations(ctx context.Context, record coverRecord, changes *gitdiff.ChangeSet, checkout string) ([]coderef.Location, error) {
	var locations []coderef.Location
	for _, value := range record.Refs {
		location, err := resolveLocation(ctx, checkout, value)
		if err != nil {
			return nil, fmt.Errorf("invalid --ref: %w", err)
		}
		locations = append(locations, location)
	}
	if record.Side != "" && record.Side != "old" && record.Side != "new" {
		return nil, errors.New("--side must be old or new")
	}
	path := filepath.ToSlash(record.Path)
	switch {
	case record.ChangedLines:
		if path == "" {
			return nil, errors.New("--changed-lines requires --path")
		}
		if record.Lines != "" || record.File || record.Commit != "" || len(record.Refs) != 0 {
			return nil, errors.New("--changed-lines cannot be combined with --lines, --file, --commit, or --ref")
		}
		selected := changedLocations(*changes, path, record.Side)
		if len(selected) == 0 {
			return nil, fmt.Errorf("--path %q has no changed atoms%s", path, map[bool]string{true: " on side " + record.Side, false: ""}[record.Side != ""])
		}
		locations = append(locations, selected...)
	case path != "":
		commit := record.Commit
		if commit == "" {
			commit = changes.HeadOID
			if record.Side == "old" {
				commit = changes.BaseOID
			}
		} else {
			if record.Side != "" {
				return nil, errors.New("--side names a comparison commit; it cannot be combined with --commit")
			}
			resolved, err := resolveCommit(ctx, checkout, commit)
			if err != nil {
				return nil, err
			}
			commit = resolved
		}
		if record.File {
			if record.Lines != "" {
				return nil, errors.New("--file references a whole file; it cannot be combined with --lines")
			}
			locations = append(locations, coderef.Location{Commit: commit, Path: path})
			break
		}
		if record.Commit == "" && record.Side == "" {
			return nil, errors.New("line references need --side new or old (or --commit)")
		}
		ranges, err := parseRanges(record.Lines)
		if err != nil {
			return nil, err
		}
		for _, lineRange := range ranges {
			locations = append(locations, coderef.Location{Commit: commit, Path: path, Start: lineRange.Start, End: lineRange.End})
		}
	case record.File || record.Lines != "" || record.Side != "" || record.Commit != "":
		return nil, errors.New("--lines, --file, --side, and --commit require --path")
	}
	if len(locations) == 0 {
		return nil, errors.New("provide --ref, or --path with --lines, --file, or --changed-lines")
	}
	return locations, nil
}

// changedLocations references exactly the changed atoms of one path with the
// fewest locations. A file event is referenced as the whole file on the side
// it exists; changed lines coalesce into dense ranges per side. A dense range
// is not a widened reference: every line it spans is a changed line.
func changedLocations(changes gitdiff.ChangeSet, path, side string) []coderef.Location {
	// A renamed file is one file: either of its paths selects the lines on
	// both sides.
	paths := map[string]bool{path: true}
	for _, atom := range changes.Atoms {
		if atom.Kind == "event" && atom.Event == "rename" && (atom.OldPath == path || atom.NewPath == path) {
			paths[atom.OldPath], paths[atom.NewPath] = true, true
		}
	}
	var locations []coderef.Location
	whole := map[string]bool{}
	lines := map[string][]int{}
	var order []string
	for _, atom := range changes.Atoms {
		if !paths[atom.Path] && !paths[atom.OldPath] && !paths[atom.NewPath] {
			continue
		}
		location := changes.Location(atom)
		atomSide := "new"
		if location.Commit == changes.BaseOID && location.Commit != changes.HeadOID {
			atomSide = "old"
		}
		if side != "" && side != atomSide {
			continue
		}
		key := location.Commit + "\x00" + location.Path
		if atom.Kind == "event" {
			if !whole[key] {
				whole[key] = true
				locations = append(locations, location)
			}
			continue
		}
		if _, ok := lines[key]; !ok {
			order = append(order, key)
		}
		lines[key] = append(lines[key], atom.Line)
	}
	for _, key := range order {
		commit, file, _ := strings.Cut(key, "\x00")
		values := append([]int(nil), lines[key]...)
		sort.Ints(values)
		for index := 0; index < len(values); {
			end := index
			for end+1 < len(values) && values[end+1] <= values[end]+1 {
				end++
			}
			locations = append(locations, coderef.Location{Commit: commit, Path: file, Start: values[index], End: values[end]})
			index = end + 1
		}
	}
	return locations
}

func resolveCommit(ctx context.Context, checkout, revision string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", checkout, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve revision %q: %s", revision, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// coverRecords returns the batch records, or the single record built from the
// flags. Per-record flags are rejected alongside --batch instead of silently
// losing to the file, because a dropped reference would quietly under-cover.
func coverRecords(batch string, stdin io.Reader, single coverRecord, flags *flag.FlagSet) ([]coverRecord, error) {
	if batch == "" {
		if len(single.Refs) == 0 && single.Path == "" && !single.ChangedLines {
			return nil, fmt.Errorf("provide --ref or --path with --lines, --file, or --changed-lines, or --batch for many records at once")
		}
		return []coverRecord{single}, nil
	}
	for _, conflicting := range []string{"path", "side", "lines", "changed-lines", "file", "commit", "name", "ref"} {
		if flagWasSet(flags, conflicting) {
			return nil, fmt.Errorf("--%s cannot be combined with --batch; put it in the batch record instead", conflicting)
		}
	}
	data, err := readBatch(batch, stdin)
	if err != nil {
		return nil, err
	}
	records, err := parseCoverRecords(data)
	if err != nil {
		return nil, err
	}
	// --target and --note stay usable as batch-wide defaults: a batch is
	// usually many references for one narrative target.
	for i := range records {
		if strings.TrimSpace(records[i].Target) == "" {
			records[i].Target = single.Target
		}
		if records[i].Note == "" {
			records[i].Note = single.Note
		}
	}
	return records, nil
}

func readBatch(source string, stdin io.Reader) ([]byte, error) {
	if source == "-" {
		if stdin == nil {
			return nil, errors.New("--batch - requires records on standard input")
		}
		return io.ReadAll(stdin)
	}
	return os.ReadFile(source)
}

// parseCoverRecords accepts newline-delimited JSON objects or a single JSON
// array of them. Unknown fields are rejected: a misspelled "lines" would
// otherwise be dropped and produce evidence that silently covers nothing.
func parseCoverRecords(data []byte) ([]coverRecord, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("--batch input contained no coverage records")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var records []coverRecord
	if trimmed[0] == '[' {
		if err := decoder.Decode(&records); err != nil {
			return nil, fmt.Errorf("parse --batch records: %w", err)
		}
	} else {
		for {
			var record coverRecord
			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("parse --batch record %d: %w", len(records)+1, err)
			}
			records = append(records, record)
		}
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, errors.New("--batch input contained trailing data after the last record")
	}
	if len(records) == 0 {
		return nil, errors.New("--batch input contained no coverage records")
	}
	return records, nil
}

// planCoverage resolves every target and destination filename up front. Names
// are reserved across the whole batch, so two records in one batch collide with
// each other exactly as loudly as one record collides with a record already on
// disk.
func planCoverage(document *saga.Saga, records []coverRecord, files []saga.CodeFile, replaceable ...string) ([]plannedRecord, error) {
	planned := make([]plannedRecord, 0, len(records))
	claimed := map[string]int{}
	allowed := map[string]bool{}
	for _, path := range replaceable {
		allowed[canonicalCoveragePath(path)] = true
	}
	for i, record := range records {
		targetDir, targetID, err := resolveTarget(document, record.Target, true)
		if err != nil {
			return nil, recordError(records, i, err)
		}
		isItem := strings.Contains(targetID, ":item:")
		if !isItem && (strings.Contains(targetID, ":deck:") || strings.Contains(targetID, ":slide:")) {
			return nil, recordError(records, i, fmt.Errorf("implementation deck evidence must target an Item; deck- and slide-level coverage is refused"))
		}
		if isItem {
			identity := strings.TrimSpace(record.Name)
			if identity != "" {
				identity = store.Slug(identity)
			} else {
				identity = stableGeneratedCoverageName(record, files[i])
			}
			full := filepath.Join(targetDir, saga.FlatEvidenceFilename(targetID, identity))
			canonical := canonicalCoveragePath(full)
			if other, taken := claimed[full]; taken {
				return nil, recordError(records, i, fmt.Errorf("coverage identity collides with record %d, which also writes %s", other+1, filepath.Base(full)))
			}
			if _, statErr := os.Lstat(full); statErr == nil && !allowed[canonical] {
				return nil, recordError(records, i, fmt.Errorf("coverage record %s already exists; use replace-coverage to reconcile it", filepath.Base(full)))
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return nil, recordError(records, i, statErr)
			}
			claimed[full] = i
			relative, _ := filepath.Rel(document.Root, full)
			planned = append(planned, plannedRecord{targetID: targetID, file: files[i], dir: targetDir, path: full, relative: filepath.ToSlash(relative)})
			continue
		}
		diffDir := filepath.Join(targetDir, saga.CodeDirName)
		name, err := coverageName(record, files[i], diffDir, claimed, allowed)
		if err != nil {
			return nil, recordError(records, i, err)
		}
		full := filepath.Join(diffDir, name+".json")
		claimed[full] = i
		relative, _ := filepath.Rel(document.Root, full)
		planned = append(planned, plannedRecord{
			targetID: targetID, file: files[i], dir: diffDir,
			path: full, relative: filepath.ToSlash(relative),
		})
	}
	return planned, nil
}

// ensureCoverageDirectories runs only after every record, target, selector,
// and destination name has passed preflight. If directory creation itself
// fails, any empty directories created by this call are removed so a rejected
// batch has no observable filesystem residue.
func ensureCoverageDirectories(root string, planned []plannedRecord) error {
	ensured := map[string]string{}
	var created []string
	for index := range planned {
		dir := planned[index].dir
		if canonical, ok := ensured[dir]; ok {
			planned[index].dir = canonical
			planned[index].path = filepath.Join(canonical, filepath.Base(planned[index].path))
			continue
		}
		_, statErr := os.Lstat(dir)
		canonical, err := store.EnsureDirWithin(root, dir)
		if err != nil {
			for createdIndex := len(created) - 1; createdIndex >= 0; createdIndex-- {
				_ = os.Remove(created[createdIndex])
			}
			return err
		}
		if os.IsNotExist(statErr) {
			created = append(created, canonical)
		}
		ensured[dir] = canonical
		planned[index].dir = canonical
		planned[index].path = filepath.Join(canonical, filepath.Base(planned[index].path))
	}
	return nil
}

// coverageName picks the record filename. An explicit name is an author's
// stable handle. A generated name is the selector identity: it deliberately
// collides when two authors explain the same selectors differently so Git
// exposes the disagreement instead of manufacturing an overlap.
func coverageName(record coverRecord, file saga.CodeFile, dir string, claimed map[string]int, replaceable map[string]bool) (string, error) {
	if strings.TrimSpace(record.Name) != "" {
		name := store.Slug(record.Name)
		full := filepath.Join(dir, name+".json")
		if other, taken := claimed[full]; taken {
			return "", fmt.Errorf("coverage name %q collides with record %d, which also writes %s", record.Name, other+1, filepath.Base(full))
		}
		if _, err := os.Lstat(full); err == nil {
			if !replaceable[canonicalCoveragePath(full)] {
				hint := ""
				if name != record.Name {
					hint = fmt.Sprintf(" (name %q is stored as %q)", record.Name, name)
				}
				return "", fmt.Errorf("coverage record %s already exists%s; choose a different name or omit it to generate one", filepath.Base(full), hint)
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return name, nil
	}
	name := stableGeneratedCoverageName(record, file)
	full := filepath.Join(dir, name+".json")
	if other, taken := claimed[full]; taken {
		return "", fmt.Errorf("coverage selector identity collides with record %d, which also writes %s", other+1, filepath.Base(full))
	}
	if _, err := os.Lstat(full); err == nil {
		if !replaceable[canonicalCoveragePath(full)] {
			return "", fmt.Errorf("coverage record %s already exists for the same selector identity; use replace-coverage to reconcile its explanation", filepath.Base(full))
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return name, nil
}

// canonicalCoveragePath resolves existing parent symlinks without following
// the final file. macOS exposes /var through /private/var, so plain absolute
// strings can name the same evidence record differently and make a same-name
// replacement delete the file it just wrote.
func canonicalCoveragePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return abs
	}
	return filepath.Join(parent, filepath.Base(abs))
}

// writeCoverage publishes a planned batch. Every destination was proven free
// during planning, so a failure here is an I/O fault; the records already
// written are removed rather than left as a partial batch.
func writeCoverage(planned []plannedRecord) error {
	for i, record := range planned {
		if err := store.WriteJSON(record.path, record.file, true); err != nil {
			for _, written := range planned[:i] {
				_ = os.Remove(written.path)
				_ = store.SyncDir(written.dir)
			}
			if len(planned) == 1 {
				return err
			}
			return fmt.Errorf("write coverage record %d of %d: %w; no records were kept", i+1, len(planned), err)
		}
	}
	return nil
}

func recordError(records []coverRecord, index int, err error) error {
	if len(records) == 1 {
		return err
	}
	return fmt.Errorf("batch record %d of %d: %w", index+1, len(records), err)
}
