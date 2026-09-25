package server

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

// reviewSnapshot is the immutable, expensive half of the review application:
// the saga structure, source comparison, coverage result, and reverse indexes.
// Review records are deliberately absent from document. They live in a small
// review generation and are projected onto the structure without rebuilding
// any of these fields.
type reviewSnapshot struct {
	document        *saga.Saga
	validation      saga.Validation
	changes         gitdiff.ChangeSet
	report          coverage.Report
	diffErr         error
	identity        string
	fileOrder       []string
	fileAtoms       map[string][]int
	fileLines       map[string][]int
	atomByKey       map[string]int
	atomPathByURI   map[string]string
	targetAtoms     map[string][]int
	targetOrder     []string
	targetFiles     map[string][]string
	targetFileAtoms map[string]map[string][]int
	fileOwners      map[string][]string
	fileSummaries   map[string]FileDiffView
	fileCoverage    map[string]ManifestFileView
	locations       map[string]manifestTargetLocation
	mutationIndex   saga.MutationIndex
}

// snapshotCache owns the expensive structural/source generation. Reviews
// under ___reviews are read directly for each review surface, so recording an
// approval never rebuilds the comparison.
type snapshotCache struct {
	mutex sync.Mutex

	saga    string
	source  string
	current *reviewSnapshot

	building bool
	buildErr error

	// builds counts structural/source work.
	builds int
}

// snapshot returns a ready request generation. The ordinary in-process test
// and embedding path builds synchronously on a miss. ListenManaged starts the
// same build asynchronously, in which case requests receive nil while the
// explicit building response is served instead of waiting for a full page.
func (a *app) snapshot(ctx context.Context) *reviewSnapshot {
	a.cache.mutex.Lock()
	if a.cache.building {
		a.cache.mutex.Unlock()
		return nil
	}
	if current := a.cache.current; current != nil {
		sagaPrint, sourcePrint := a.fingerprints(ctx, current.document.Manifest)
		if sagaPrint != "" && sagaPrint == a.cache.saga && sourcePrint != "" && sourcePrint == a.cache.source {
			a.cache.mutex.Unlock()
			return current
		}
	}
	a.cache.current = nil
	a.cache.saga, a.cache.source = "", ""
	a.cache.building, a.cache.buildErr = true, nil
	a.cache.mutex.Unlock()

	return a.finishSnapshotBuild(ctx)
}

// startSnapshotBuild makes cold-start work observable without blocking the
// listener. It returns false when a generation is already ready or building.
func (a *app) startSnapshotBuild(ctx context.Context, done func(error)) bool {
	a.cache.mutex.Lock()
	if a.cache.current != nil || a.cache.building {
		a.cache.mutex.Unlock()
		return false
	}
	a.cache.building, a.cache.buildErr = true, nil
	a.cache.mutex.Unlock()
	go func() {
		result := a.finishSnapshotBuild(ctx)
		a.cache.mutex.Lock()
		err := a.cache.buildErr
		a.cache.mutex.Unlock()
		if result == nil && err == nil {
			err = fmt.Errorf("review cache build did not publish a generation")
		}
		if done != nil {
			done(err)
		}
	}()
	return true
}

// finishSnapshotBuild performs one structural/source build and atomically
// publishes both its immutable snapshot and the review generation observed
// alongside it. Callers must have changed cache.building from false to true.
func (a *app) finishSnapshotBuild(ctx context.Context) *reviewSnapshot {
	built, err := a.loadComparison(ctx)
	var sagaPrint, sourcePrint string
	if err == nil {
		sagaPrint, sourcePrint = a.fingerprints(ctx, built.document.Manifest)
	}

	a.cache.mutex.Lock()
	defer a.cache.mutex.Unlock()
	a.cache.building = false
	a.cache.buildErr = err
	if err != nil {
		return nil
	}
	a.cache.saga, a.cache.source, a.cache.current = sagaPrint, sourcePrint, built
	return built
}

func (a *app) loadComparison(ctx context.Context) (*reviewSnapshot, error) {
	if a.comparisonLoader != nil {
		built, err := a.comparisonLoader(ctx)
		if err != nil {
			return nil, err
		}
		if built.mutationIndex.Root == "" {
			built.mutationIndex = saga.MutationIndexFromDocument(built.document)
		}
		return built, nil
	}
	return a.buildSnapshot(ctx)
}

// cachedCoverageTotals reads an already-published generation without starting
// or waiting for a build. The root shell can therefore report ready totals but
// stays bounded and responsive while the comparison index is cold.
func (a *app) cachedCoverageTotals() *coverageTotalsView {
	a.cache.mutex.Lock()
	defer a.cache.mutex.Unlock()
	current := a.cache.current
	if current == nil || current.diffErr != nil {
		return nil
	}
	return &coverageTotalsView{
		Files: len(current.fileOrder), Total: current.report.Summary.Total,
		Covered: current.report.Summary.Covered, Uncovered: current.report.Summary.Uncovered,
		Overlapping: current.report.Summary.Overlapping, Orphaned: current.report.Summary.Stale,
		Complete: current.report.Complete,
	}
}

// requestSnapshot is the shared cold-cache boundary for comparison endpoints.
// A managed server builds in the background, so these surfaces ask the browser
// to retry instead of blocking a request or misreporting a transient cold cache
// as a broken saga.
func (a *app) requestSnapshot(w http.ResponseWriter, r *http.Request) *reviewSnapshot {
	current := a.snapshot(r.Context())
	if current != nil {
		return current
	}
	if state, _ := a.snapshotState(); state == "building" {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Building review cache", http.StatusAccepted)
		return nil
	}
	http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
	return nil
}

// snapshotState is intentionally tiny because it is used by the HTTP status
// path while the large generation does not exist yet.
func (a *app) snapshotState() (state string, err error) {
	a.cache.mutex.Lock()
	defer a.cache.mutex.Unlock()
	switch {
	case a.cache.building:
		return "building", nil
	case a.cache.current != nil:
		return "ready", nil
	case a.cache.buildErr != nil:
		return "error", a.cache.buildErr
	default:
		return "absent", nil
	}
}

// fingerprints describes only structural saga bytes and the source comparison.
// Review records have their own generation and are intentionally excluded.
func (a *app) fingerprints(ctx context.Context, manifest saga.Manifest) (sagaPrint, sourcePrint string) {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		sagaPrint, _ = structuralFingerprint(a.root)
	}()
	if print, err := a.sourceFingerprint(ctx, manifest); err == nil {
		sourcePrint = print
	}
	group.Wait()
	return sagaPrint, sourcePrint
}

func (a *app) buildSnapshot(ctx context.Context) (*reviewSnapshot, error) {
	structural, validation, err := saga.Load(a.root)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		return nil, fmt.Errorf("saga is structurally invalid; run change-saga validate")
	}
	built := &reviewSnapshot{document: structural, validation: validation, mutationIndex: saga.MutationIndexFromDocument(structural)}
	if a.generations == nil {
		_ = a.populateDerivedSnapshot(ctx, built)
		return built, nil
	}

	treePrint, sourcePrint := a.fingerprints(ctx, structural.Manifest)
	// The derived representation is disposable. Include its format in the
	// content key so a release that changes the on-disk shape builds a fresh
	// generation instead of opening an older generation and failing in place.
	key := snapshotcache.Key{Saga: a.root, Tree: derivedSnapshotFormat + "\x00" + treePrint, Source: sourcePrint}
	populated := false
	dir, _, buildErr := a.generations.Build(key, func(stage string) error {
		if err := a.populateDerivedSnapshot(ctx, built); err != nil {
			return err
		}
		populated = true
		return writeDerivedSnapshot(filepath.Join(stage, derivedSnapshotName), built)
	})
	if buildErr != nil {
		built.diffErr = buildErr
		return built, nil
	}
	if !populated {
		if err := readDerivedSnapshot(filepath.Join(dir, derivedSnapshotName), built); err != nil {
			built.diffErr = fmt.Errorf("read cached review index: %w", err)
		}
	}
	if key.Valid() {
		_ = a.generations.Prune(key, 3)
	}
	return built, nil
}

func (a *app) populateDerivedSnapshot(ctx context.Context, built *reviewSnapshot) error {
	a.cache.mutex.Lock()
	a.cache.builds++
	a.cache.mutex.Unlock()
	built.changes, built.diffErr = gitdiff.ReadRange(ctx, a.sourceDir, built.document.Manifest.Source.Repository, a.rng, gitdiff.ReadOptions{})
	if built.diffErr != nil {
		return built.diffErr
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		built.diffErr = err
		return err
	}
	defer resolver.Close()
	// Eligible Item selections inherit exactly their selected lines, as the
	// CLI's reports do. An unreadable inventory inherits nothing; it never
	// widens coverage.
	var inherited []coverage.InheritedReference
	if inventory, err := requirements.LoadInventory(a.root, built.document.Manifest.ID); err == nil {
		inherited, _ = inventoryview.InheritedReferences(ctx, built.document, &inventory, built.changes.HeadOID, resolver)
	}
	built.report = coverage.EvaluateInherited(ctx, built.document, inherited, built.validation, built.changes, resolver)
	digest := sha256.Sum256([]byte(built.changes.Repository + "\x00" + built.changes.BaseOID + "\x00" + built.changes.HeadOID))
	built.identity = hex.EncodeToString(digest[:16])
	built.indexComparison()
	return nil
}

func (s *reviewSnapshot) indexComparison() {
	s.targetAtoms = map[string][]int{}
	for index := range s.changes.Atoms {
		atom := &s.changes.Atoms[index]
		seen := map[string]bool{}
		for _, owner := range s.report.Ownership[atom.Key] {
			if !seen[owner.Target] {
				s.targetAtoms[owner.Target] = append(s.targetAtoms[owner.Target], index)
				seen[owner.Target] = true
			}
		}
	}
	s.fileAtoms = map[string][]int{}
	s.fileLines = map[string][]int{}
	s.atomByKey = make(map[string]int, len(s.changes.Atoms))
	s.atomPathByURI = make(map[string]string, len(s.changes.Atoms))
	renameTo := map[string]string{}
	for index := range s.changes.Atoms {
		atom := &s.changes.Atoms[index]
		if atom.Kind == "event" && atom.Event == "rename" && atom.OldPath != "" && atom.NewPath != "" {
			renameTo[atom.OldPath] = atom.NewPath
		}
	}
	for index := range s.changes.Atoms {
		atom := &s.changes.Atoms[index]
		path := atom.Path
		if path == "" {
			path = atom.NewPath
		}
		if renamed := renameTo[path]; renamed != "" {
			path = renamed
		}
		s.fileAtoms[path] = append(s.fileAtoms[path], index)
		s.atomByKey[atom.Key], s.atomPathByURI[atom.Ref] = index, path
	}
	for index := range s.changes.DisplayLines {
		line := &s.changes.DisplayLines[index]
		path := line.Path
		if renamed := renameTo[path]; renamed != "" {
			path = renamed
		}
		line.Path = path
		s.fileLines[path] = append(s.fileLines[path], index)
	}
	for path := range s.fileAtoms {
		s.fileOrder = append(s.fileOrder, path)
	}
	sort.Strings(s.fileOrder)
	s.fileSummaries = make(map[string]FileDiffView, len(s.fileOrder))
	s.fileCoverage = make(map[string]ManifestFileView, len(s.fileOrder))
	for _, path := range s.fileOrder {
		digest := sha256.Sum256([]byte(path))
		file := FileDiffView{ID: fmt.Sprintf("diff-%x", digest[:8]), Path: path, Ref: fileLocation(s.changes.BaseOID, s.changes.HeadOID, path, s.fileDeleted(path))}
		for _, index := range s.fileAtoms[path] {
			atom := &s.changes.Atoms[index]
			if atom.Side == "new" {
				file.Added++
			} else if atom.Side == "old" {
				file.Deleted++
			}
		}
		s.fileSummaries[path] = file
		coverageFile := ManifestFileView{Path: path, HasDiff: true}
		for _, index := range s.fileAtoms[path] {
			atom := &s.changes.Atoms[index]
			coverageFile.AtomCount++
			if atom.Kind == "event" {
				coverageFile.Events++
			} else if atom.Side == "old" {
				coverageFile.Deleted++
			} else {
				coverageFile.Added++
			}
			if len(s.report.Ownership[atom.Key]) == 0 {
				coverageFile.Uncovered++
			} else {
				coverageFile.Covered++
			}
		}
		s.fileCoverage[path] = coverageFile
	}
	s.locations = indexManifestTargets(s.document)
	s.fileOwners = map[string][]string{}
	for path, indexes := range s.fileAtoms {
		seen := map[string]bool{}
		for _, index := range indexes {
			atom := &s.changes.Atoms[index]
			for _, assignment := range s.report.Ownership[atom.Key] {
				seen[assignment.Target] = true
			}
		}
		for target := range seen {
			s.fileOwners[path] = append(s.fileOwners[path], target)
		}
		sort.SliceStable(s.fileOwners[path], func(i, j int) bool {
			left, leftOK := s.locations[s.fileOwners[path][i]]
			right, rightOK := s.locations[s.fileOwners[path][j]]
			if leftOK && rightOK && left.order != right.order {
				return left.order < right.order
			}
			return s.fileOwners[path][i] < s.fileOwners[path][j]
		})
	}
	for target := range s.targetAtoms {
		s.targetOrder = append(s.targetOrder, target)
	}
	sort.SliceStable(s.targetOrder, func(i, j int) bool {
		left, leftOK := s.locations[s.targetOrder[i]]
		right, rightOK := s.locations[s.targetOrder[j]]
		if leftOK && rightOK && left.order != right.order {
			return left.order < right.order
		}
		return s.targetOrder[i] < s.targetOrder[j]
	})
	s.targetFiles = make(map[string][]string, len(s.targetAtoms))
	s.targetFileAtoms = make(map[string]map[string][]int, len(s.targetAtoms))
	for target, indexes := range s.targetAtoms {
		byPath := map[string][]int{}
		for _, index := range indexes {
			path := effectiveAtomPath(s.changes.Atoms[index])
			byPath[path] = append(byPath[path], index)
		}
		paths := make([]string, 0, len(byPath))
		for path := range byPath {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		s.targetFiles[target] = paths
		s.targetFileAtoms[target] = byPath
	}
}

const (
	// v4: coverage includes lines Items inherit through inventory selections.
	derivedSnapshotFormat = "review-index-v4"
	derivedSnapshotName   = derivedSnapshotFormat + ".json.gz"
)

type persistedDerivedSnapshot struct {
	Version      int                              `json:"version"`
	Changes      gitdiff.ChangeSet                `json:"changes"`
	DisplayLines []gitdiff.DisplayLine            `json:"display_lines"`
	Report       coverage.Report                  `json:"report"`
	Ownership    map[string][]coverage.Assignment `json:"ownership"`
}

func writeDerivedSnapshot(path string, snapshot *reviewSnapshot) error {
	value := persistedDerivedSnapshot{
		Version: 3, Changes: snapshot.changes, DisplayLines: snapshot.changes.DisplayLines,
		Report: snapshot.report, Ownership: snapshot.report.Ownership,
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	compressed, err := gzip.NewWriterLevel(file, gzip.BestSpeed)
	if err != nil {
		_ = file.Close()
		return err
	}
	encoder := json.NewEncoder(compressed)
	if err := encoder.Encode(value); err != nil {
		_ = compressed.Close()
		_ = file.Close()
		return err
	}
	if err := compressed.Close(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readDerivedSnapshot(path string, snapshot *reviewSnapshot) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	var value persistedDerivedSnapshot
	if err := json.NewDecoder(compressed).Decode(&value); err != nil {
		return err
	}
	if value.Version != 3 {
		return fmt.Errorf("unsupported review index version %d", value.Version)
	}
	value.Changes.DisplayLines = value.DisplayLines
	value.Report.Ownership = value.Ownership
	snapshot.changes, snapshot.report = value.Changes, value.Report
	snapshot.diffErr = nil
	digest := sha256.Sum256([]byte(snapshot.changes.Repository + "\x00" + snapshot.changes.BaseOID + "\x00" + snapshot.changes.HeadOID))
	snapshot.identity = hex.EncodeToString(digest[:16])
	snapshot.indexComparison()
	return nil
}

// structuralFingerprint hashes every saga entry except ___reviews. Recording
// an approval or comment therefore cannot select a new structural generation.
func structuralFingerprint(root string) (string, error) {
	return filteredTreeFingerprint(root, func(_ string, entry fs.DirEntry) (include, skip bool) {
		if entry.IsDir() && entry.Name() == saga.ReviewsDir {
			return false, true
		}
		return true, false
	})
}

// treeFingerprint keeps the historical helper contract for callers and tests
// that need the complete saga tree. Structural cache keys use the filtered
// variant above.
func treeFingerprint(root string) (string, error) {
	return filteredTreeFingerprint(root, func(string, fs.DirEntry) (bool, bool) { return true, false })
}

func filteredTreeFingerprint(root string, selectEntry func(string, fs.DirEntry) (include, skip bool)) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		include, skip := selectEntry(relative, entry)
		if skip && entry.IsDir() {
			return filepath.SkipDir
		}
		if !include {
			return nil
		}
		if entry.IsDir() {
			fmt.Fprintf(digest, "d\x00%s\x00", filepath.ToSlash(relative))
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(digest, "f\x00%s\x00%d\x00%d\x00", filepath.ToSlash(relative), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// sourceFingerprint identifies the exact comparison gitdiff.ReadRange would
// produce for the range the reviewer was opened with.
func (a *app) sourceFingerprint(ctx context.Context, manifest saga.Manifest) (string, error) {
	var remote string
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		remote, _ = gitOutput(ctx, a.sourceDir, "config", "--get", "remote.origin.url")
	}()
	against := a.rng.Against
	if a.rng.Observe() {
		against = a.rng.HeadRevision()
	}
	revisions, err := gitOutput(ctx, a.sourceDir, "rev-parse", against+"^{commit}", a.rng.HeadRevision()+"^{commit}")
	group.Wait()
	if err != nil {
		return "", err
	}
	return a.rng.Mode() + "\x00" + manifest.Source.Repository + "\x00" + strings.Join(strings.Fields(revisions), " ") + "\x00" + remote, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// fileDeleted reports whether the comparison deletes path.
func (s *reviewSnapshot) fileDeleted(path string) bool {
	for _, index := range s.fileAtoms[path] {
		if atom := &s.changes.Atoms[index]; atom.Kind == "event" && atom.Event == "delete" {
			return true
		}
	}
	return false
}
