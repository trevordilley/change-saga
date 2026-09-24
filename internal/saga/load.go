package saga

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
)

type loadOptions struct {
	outline      bool
	skipCoverage bool
}

// hierarchyRoot distinguishes the two package trees that reuse the authored
// chapter/section/fragment machinery. Only the Saga root may contain living
// resource directories; the design root is an authored hierarchy root, not a
// second Saga container.
type hierarchyRoot uint8

const (
	nestedHierarchy hierarchyRoot = iota
	// sagaHierarchy is the app root: saga.json, the app-level roots, the
	// features, and the review overlay. It holds no report content of its own.
	sagaHierarchy
	// designHierarchy is an authored report root that is not a feature:
	// a feature's ___design, and the app's ___overview and ___designsystem.
	designHierarchy
	// featureHierarchy is one feature directory: report content plus the feature's
	// capability roots.
	featureHierarchy
	// overviewHierarchy is the app's ___overview. It is formal rather than
	// free report content: an elevator pitch fragment, a description fragment,
	// and the terms directory the requirements loader owns. The project name
	// is saga.json's title. Every part is optional.
	overviewHierarchy
)

// overviewPartError is the one message for anything else in ___overview.
const overviewPartError = "the overview holds only " + applayout.OverviewPitch + ", " + applayout.OverviewDescription + ", and " + applayout.OverviewTerms + "/"

// overviewEntry reports whether a directory named name belongs in ___overview,
// and whether it is a fragment the report loader reads.
func overviewEntry(name string) (allowed, fragment bool) {
	switch name {
	case applayout.OverviewPitch, applayout.OverviewDescription:
		return true, true
	case applayout.OverviewTerms:
		return true, false
	}
	return false, false
}

var fullLoadCount atomic.Uint64

// FullLoadCount reports process-local full Saga loads. It is diagnostic
// instrumentation used by scale budgets to keep review mutations off this
// path; review-only and mutation-index loads do not increment it.
func FullLoadCount() uint64 { return fullLoadCount.Load() }

// ManifestName is the Change Saga root manifest.
const ManifestName = "saga.json"

// ReadManifest reads only the root manifest. Front ends use it to refuse an
// unsupported directory before opening a heavier application view.
func ReadManifest(root string) (Manifest, error) {
	return readManifest(root)
}

// readManifest reads saga.json and admits only the one Change Saga format.
func readManifest(root string) (Manifest, error) {
	var manifest Manifest
	path := filepath.Join(root, ManifestName)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, fmt.Errorf("%s has no %s; it is not a Change Saga", root, ManifestName)
	}
	if err := readJSON(path, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("read %s: %w", ManifestName, err)
	}
	if manifest.Version != SagaVersion {
		return Manifest{}, fmt.Errorf("%s: unsupported Saga version %d; change-saga reads only version %d", ManifestName, manifest.Version, SagaVersion)
	}
	return manifest, nil
}

func Load(root string) (*Saga, Validation, error) {
	fullLoadCount.Add(1)
	return load(root, loadOptions{})
}

// LoadOutline reads the narrative and review metadata needed to render the
// reviewer shell without opening coverage records, landmark records, claims,
// verifications, diff reviews, fragment content, or message attachments. It is
// deliberately not a replacement for Load: callers that make readiness or
// mutation decisions must still use the complete validated model.
func LoadOutline(root string) (*Saga, Validation, error) {
	return load(root, loadOptions{outline: true, skipCoverage: true})
}

// LoadNarrative reads the complete reviewable narrative and annotations while
// leaving coverage records and diff-review state unopened. Incremental prose
// endpoints use it so reaching a chapter or fragment cannot trigger coverage
// graph construction.
func LoadNarrative(root string) (*Saga, Validation, error) {
	return load(root, loadOptions{skipCoverage: true})
}

func load(root string, options loadOptions) (*Saga, Validation, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, Validation{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, Validation{}, fmt.Errorf("open saga: %w", err)
	}
	if !info.IsDir() {
		return nil, Validation{}, fmt.Errorf("%s is not a directory", root)
	}

	manifest, err := readManifest(abs)
	if err != nil {
		return nil, Validation{}, err
	}
	validation := Validation{Valid: true, Issues: []Issue{}}
	if !strings.HasSuffix(filepath.Base(abs), ".saga") {
		addIssue(&validation, "error", ".", "saga root directory must end in .saga")
	}
	validateManifest(manifest, ManifestName, &validation)

	section, err := loadSection(abs, abs, manifest, sagaHierarchy, options, &validation)
	if err != nil {
		return nil, validation, err
	}
	app, decks, err := loadAppContent(abs, manifest, section, options, &validation)
	if err != nil {
		return nil, validation, err
	}
	document := &Saga{Root: abs, Manifest: manifest, Section: section, Decks: decks, Overview: app.overview, DesignSystem: app.designSystem, Onboarding: app.onboarding, Features: app.features}
	if metadataDirectorySafe(abs, abs, ReviewsDir, &validation) {
		if document.Reviews, err = loadReviews(abs, manifest, options, &validation); err != nil {
			return nil, validation, err
		}
	}
	if !options.outline && !options.skipCoverage {
		if metadataDirectorySafe(abs, abs, "___claims", &validation) {
			document.Claims, err = loadClaims(abs, &validation)
			if err != nil {
				return nil, validation, err
			}
		}
		if metadataDirectorySafe(abs, abs, "___verifications", &validation) {
			document.Verifications, err = loadVerifications(abs, &validation)
			if err != nil {
				return nil, validation, err
			}
		}
		if metadataDirectorySafe(abs, abs, MergesDir, &validation) {
			document.Merges, err = loadMerges(abs, &validation)
			if err != nil {
				return nil, validation, err
			}
		}
	}
	if _, _, err := ReadCursor(abs); err != nil {
		addIssue(&validation, "error", CursorName, err.Error())
	}
	if options.outline {
		validateOutlineDocument(document, &validation)
	} else {
		validateDocument(document, &validation)
	}
	validation.Valid = !hasErrors(validation.Issues)
	return document, validation, nil
}

func loadSection(root, dir string, manifest Manifest, hierarchy hierarchyRoot, options loadOptions, validation *Validation) (*Section, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, err
	}
	if rel == "." {
		rel = ""
	}
	section := &Section{Path: filepath.ToSlash(rel), Kind: "section"}
	if hierarchy == sagaHierarchy {
		section.Kind = "saga"
		section.ID = manifest.ID + "-root"
		section.Title = manifest.Title
		section.Target = SagaTarget(manifest.ID)
	} else if hierarchy == overviewHierarchy {
		section.Kind = "overview"
		section.ID = manifest.ID + "-overview-root"
		section.Title = "Overview"
	} else if hierarchy == designHierarchy {
		// This synthetic node is used only while the shared loader scans the
		// physical root. Its children and fragments are joined to the Saga root
		// above, so no invented design-root target escapes into the public model.
		section.Kind = "design"
		section.ID = manifest.ID + "-design-root"
		section.Title = "Technical design"
	} else if hierarchy == featureHierarchy {
		// Like the design root, a feature is a synthetic grouping node. Its
		// report content joins the app root so every target index still sees
		// one tree; Saga.Features keeps the grouping for readers.
		section.Kind = "feature"
		section.ID = strings.TrimSuffix(filepath.Base(dir), applayout.FeatureSuffix)
		section.Title = section.ID
	} else {
		if strings.HasSuffix(filepath.Base(dir), ".chapter") {
			section.Kind = "chapter"
			var value ChapterManifest
			path := filepath.Join(dir, "chapter.json")
			if err := readJSON(path, &value); err != nil {
				addIssue(validation, "error", relativePath(root, path), err.Error())
				section.ID = strings.TrimSuffix(filepath.Base(dir), ".chapter")
				section.Title = section.ID
			} else {
				section.ID, section.Title, section.Order = value.ID, value.Title, value.Order
				validateChapterManifest(value, relativePath(root, path), validation)
			}
			section.Target = ChapterTarget(manifest.ID, section.ID)
		} else {
			var value SectionManifest
			path := filepath.Join(dir, "section.json")
			if err := readJSON(path, &value); err != nil {
				addIssue(validation, "error", relativePath(root, path), err.Error())
				section.ID = filepath.Base(dir)
				section.Title = filepath.Base(dir)
			} else {
				section.ID, section.Title, section.Order = value.ID, value.Title, value.Order
				validateSectionManifest(value, relativePath(root, path), validation)
			}
			section.Target = SectionTarget(manifest.ID, section.ID)
		}
	}

	if !options.skipCoverage && metadataDirectorySafe(root, dir, CodeDirName, validation) {
		section.Code, err = loadCode(root, filepath.Join(dir, CodeDirName), validation)
		if err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "___") {
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
				addIssue(validation, "error", displayPath(rel, name), "reserved metadata path must be a real directory")
			} else {
				if name == CodeDirName {
					section.HasCode = true
				}
				if !knownReservedDirectory(name, hierarchy) {
					addIssue(validation, "error", displayPath(rel, name), "unknown reserved directory")
				}
			}
			continue
		}
		if !entry.IsDir() {
			for _, suffix := range []string{".chapter", ".fragment"} {
				if matches, problem := structuralEntry(entry, suffix); matches && problem != "" {
					addIssue(validation, "error", displayPath(rel, name), problem)
				}
			}
			continue
		}
		path := filepath.Join(dir, name)
		if reason := PortabilityWarning(name); reason != "" {
			addIssue(validation, "warning", displayPath(rel, name), fmt.Sprintf("directory name %q %s", name, reason))
		}
		if hierarchy == overviewHierarchy {
			allowed, fragment := overviewEntry(name)
			if !allowed || entry.Type()&fs.ModeSymlink != 0 {
				addIssue(validation, "error", displayPath(rel, name), overviewPartError)
				continue
			}
			if !fragment {
				continue
			}
		}
		if hierarchy == sagaHierarchy {
			addIssue(validation, "error", displayPath(rel, name), "report content belongs in ___overview, ___designsystem, or a feature under ___features, not at the app root")
			continue
		}
		if hierarchy != nestedHierarchy && !strings.HasSuffix(name, ".fragment") && !strings.HasSuffix(name, ".chapter") {
			addIssue(validation, "error", displayPath(rel, name), "direct saga children must be .chapter or .fragment directories")
		}
		if strings.HasSuffix(name, ".chapter") && hierarchy == nestedHierarchy {
			addIssue(validation, "error", displayPath(rel, name), "chapters must be direct children of the saga root")
		}
		if strings.HasSuffix(name, ".fragment") {
			fragment, err := loadFragment(root, path, manifest.ID, options, validation)
			if err != nil {
				return nil, err
			}
			section.Fragments = append(section.Fragments, fragment)
			continue
		}
		child, err := loadSection(root, path, manifest, nestedHierarchy, options, validation)
		if err != nil {
			return nil, err
		}
		section.Children = append(section.Children, child)
	}
	sortSectionContents(section)
	return section, nil
}

func sortSectionContents(section *Section) {
	sort.Slice(section.Fragments, func(i, j int) bool {
		if section.Fragments[i].Order == section.Fragments[j].Order {
			return section.Fragments[i].Path < section.Fragments[j].Path
		}
		return section.Fragments[i].Order < section.Fragments[j].Order
	})
	sort.Slice(section.Children, func(i, j int) bool {
		if section.Children[i].Order == section.Children[j].Order {
			return section.Children[i].Path < section.Children[j].Path
		}
		return section.Children[i].Order < section.Children[j].Order
	})
}

func loadFragment(root, dir, sagaID string, options loadOptions, validation *Validation) (*Fragment, error) {
	manifestPath := filepath.Join(dir, "fragment.json")
	var value FragmentManifest
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		value.ID = strings.TrimSuffix(filepath.Base(dir), ".fragment")
	}
	rel, _ := filepath.Rel(root, dir)
	fragment := &Fragment{
		Path: filepath.ToSlash(rel), Directory: dir, ID: value.ID, Title: value.Title,
		MediaType: value.MediaType, Entrypoint: value.Entrypoint, Order: value.Order,
		Target: FragmentTarget(sagaID, value.ID),
	}
	if options.outline {
		validateFragmentOutlineManifest(value, relativePath(root, manifestPath), dir, validation)
	} else {
		validateFragmentManifest(value, relativePath(root, manifestPath), dir, validation)
	}
	if !options.skipCoverage && metadataDirectorySafe(root, dir, CodeDirName, validation) {
		var err error
		fragment.Code, err = loadCode(root, filepath.Join(dir, CodeDirName), validation)
		if err != nil {
			return nil, err
		}
	}
	if !options.outline && metadataDirectorySafe(root, dir, "___landmarks", validation) {
		var err error
		fragment.Landmarks, err = loadLandmarks(root, filepath.Join(dir, "___landmarks"), sagaID, fragment, options, validation)
		if err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == CodeDirName {
			fragment.HasCode = true
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "___") && entry.Name() != CodeDirName && entry.Name() != "___landmarks" {
			addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown reserved directory in fragment")
		}
	}
	return fragment, nil
}

func loadLandmarks(root, dir, sagaID string, fragment *Fragment, options loadOptions, validation *Validation) ([]Landmark, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []Landmark
	seen := map[string]string{}
	headings := map[string]string{}
	if fragment.MediaType == "text/markdown" {
		if content, readErr := os.ReadFile(filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))); readErr == nil {
			for _, heading := range MarkdownHeadings(string(content)) {
				if heading.Explicit && ValidMarkdownAnchor(heading.Anchor) {
					headings[heading.Anchor] = relativePath(root, filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint)))
				}
			}
		}
	}
	for _, entry := range entries {
		entryPath := filepath.Join(dir, entry.Name())
		if matches, problem := structuralEntry(entry, ".landmark"); !matches || problem != "" {
			addIssue(validation, "error", relativePath(root, entryPath), "landmarks must be real <id>.landmark directories")
			continue
		}
		path := filepath.Join(entryPath, "landmark.json")
		var value Landmark
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			continue
		}
		value.Path = relativePath(root, path)
		value.Directory = entryPath
		value.Target = LandmarkTarget(sagaID, fragment.ID, value.ID)
		if directoryID := strings.TrimSuffix(entry.Name(), ".landmark"); directoryID != value.ID {
			addIssue(validation, "error", value.Path, fmt.Sprintf("landmark id %q must match directory %q", value.ID, directoryID+".landmark"))
		}
		validateLandmark(value, fragment, validation)
		if previous, ok := seen[value.ID]; ok {
			addIssue(validation, "error", value.Path, fmt.Sprintf("landmark id %q is duplicated; first used by %s", value.ID, previous))
		} else if headingPath, ok := headings[value.ID]; ok && value.Selector.Type != "heading" {
			addIssue(validation, "error", value.Path, fmt.Sprintf("landmark id %q conflicts with a Markdown heading in %s", value.ID, headingPath))
		}
		seen[value.ID] = value.Path
		if !options.skipCoverage && metadataDirectorySafe(root, entryPath, CodeDirName, validation) {
			value.Code, err = loadCode(root, filepath.Join(entryPath, CodeDirName), validation)
			if err != nil {
				return nil, err
			}
		}
		landmarkEntries, readErr := os.ReadDir(entryPath)
		if readErr != nil {
			return nil, readErr
		}
		for _, landmarkEntry := range landmarkEntries {
			if landmarkEntry.IsDir() && landmarkEntry.Name() == CodeDirName {
				value.HasCode = true
			}
			if landmarkEntry.IsDir() && strings.HasPrefix(landmarkEntry.Name(), "___") && landmarkEntry.Name() != CodeDirName {
				addIssue(validation, "error", relativePath(root, filepath.Join(entryPath, landmarkEntry.Name())), "unknown reserved directory in landmark")
			}
		}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == result[j].ID {
			return result[i].Path < result[j].Path
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func loadCode(root, dir string, validation *Validation) ([]CodeFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []CodeFile
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var value CodeFile
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			continue
		}
		value.Path = relativePath(root, path)
		validateCodeFile(value, validation)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// LoadTargetCode reads only one validated narrative target's authored
// evidence. It is the bounded mapping seam used by linked-code requests: no
// sibling target's ___code directory is opened.
func LoadTargetCode(index MutationIndex, target string) ([]CodeFile, Validation, error) {
	validation := Validation{Valid: true, Issues: []Issue{}}
	dir, ok := index.Targets[target]
	if !ok {
		addIssue(&validation, "error", ".", "unknown narrative target")
		validation.Valid = false
		return nil, validation, nil
	}
	if index.FlatTargets[target] {
		prefix := "40-e-" + FlatTargetKey(target) + "-"
		recordRoot := dir
		entries, err := os.ReadDir(recordRoot)
		if err != nil {
			return nil, validation, err
		}
		var diffs []CodeFile
		transactionFound := false
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !flatEvidenceName.MatchString(entry.Name()) {
				continue
			}
			var value CodeFile
			if err := readJSON(filepath.Join(recordRoot, entry.Name()), &value); err != nil {
				addIssue(&validation, "error", entry.Name(), err.Error())
				continue
			}
			value.Path = relativePath(index.Root, filepath.Join(recordRoot, entry.Name()))
			validateCodeFile(value, &validation)
			diffs = append(diffs, value)
		}
		// Transactional Items keep their exact evidence inside the one
		// complete-slide commit record, so the bounded target-code seam reads
		// only transaction records in this already-resolved deck bundle.
		for _, entry := range entries {
			if entry.IsDir() || !flatSlideTransactionName.MatchString(entry.Name()) {
				continue
			}
			if info, infoErr := entry.Info(); infoErr != nil || info.Size() > MaxSlideTransactionBytes {
				addIssue(&validation, "error", entry.Name(), fmt.Sprintf("slide transaction exceeds the %d-byte limit", MaxSlideTransactionBytes))
				continue
			}
			var record SlideTransactionRecord
			if err := readJSON(filepath.Join(recordRoot, entry.Name()), &record); err != nil {
				addIssue(&validation, "error", entry.Name(), err.Error())
				continue
			}
			current, currentErr := record.currentRevision()
			if currentErr != nil {
				addIssue(&validation, "error", entry.Name(), currentErr.Error())
				continue
			}
			for _, item := range current.Items {
				if ItemTarget(index.Manifest.ID, record.SlideID, item.Item.ID) != target {
					continue
				}
				if !transactionFound {
					// The transaction is authoritative for an Item migrated
					// from legacy flat records. Do not combine old evidence
					// with the current complete revision.
					diffs = nil
					transactionFound = true
				}
				for evidenceIndex, value := range item.Evidence {
					value.Path = relativePath(index.Root, filepath.Join(recordRoot, entry.Name())) + fmt.Sprintf("#items/%s/evidence/%d", item.Item.ID, evidenceIndex)
					validateCodeFile(value, &validation)
					diffs = append(diffs, value)
				}
			}
		}
		sort.Slice(diffs, func(i, j int) bool { return diffs[i].Path < diffs[j].Path })
		validation.Valid = !hasErrors(validation.Issues)
		return diffs, validation, nil
	}
	if !metadataDirectorySafe(index.Root, dir, CodeDirName, &validation) {
		validation.Valid = false
		return nil, validation, nil
	}
	diffs, err := loadCode(index.Root, filepath.Join(dir, CodeDirName), &validation)
	validation.Valid = !hasErrors(validation.Issues)
	return diffs, validation, err
}

func loadClaims(root string, validation *Validation) ([]Claim, error) {
	var claims []Claim
	err := loadFlatRecords(root, filepath.Join(root, "___claims"), "claim", validation, func(path string) {
		var value Claim
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			return
		}
		value.Path = path
		if strings.TrimSuffix(filepath.Base(path), ".json") != value.ID {
			addIssue(validation, "error", relativePath(root, path), fmt.Sprintf("claim id %q must match filename %q", value.ID, filepath.Base(path)))
		}
		validateClaim(value, relativePath(root, path), validation)
		claims = append(claims, value)
	})
	sort.Slice(claims, func(i, j int) bool {
		return earlierRecord(claims[i].CreatedAt, claims[i].ID, claims[j].CreatedAt, claims[j].ID)
	})
	return claims, err
}

func loadVerifications(root string, validation *Validation) ([]Verification, error) {
	var verifications []Verification
	err := loadFlatRecords(root, filepath.Join(root, "___verifications"), "verification", validation, func(path string) {
		var value Verification
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			return
		}
		value.Path = path
		if strings.TrimSuffix(filepath.Base(path), ".json") != value.ID {
			addIssue(validation, "error", relativePath(root, path), fmt.Sprintf("verification id %q must match filename %q", value.ID, filepath.Base(path)))
		}
		validateVerification(value, relativePath(root, path), validation)
		verifications = append(verifications, value)
	})
	sort.Slice(verifications, func(i, j int) bool {
		return earlierRecord(verifications[i].CreatedAt, verifications[i].ID, verifications[j].CreatedAt, verifications[j].ID)
	})
	return verifications, err
}

func loadFlatRecords(root, dir, kind string, validation *Validation, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			addIssue(validation, "error", relativePath(root, path), kind+" records must be regular .json files, not directories or symlinks")
			continue
		}
		fn(path)
	}
	return nil
}

func loadMerges(root string, validation *Validation) ([]Merge, error) {
	var merges []Merge
	err := loadMetaJSON(filepath.Join(root, MergesDir), func(path string) {
		var value Merge
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			return
		}
		value.Path = path
		if err := validateMerge(value, filepath.Base(path)); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
		}
		merges = append(merges, value)
	})
	sort.Slice(merges, func(i, j int) bool { return merges[i].PinnedAt.Before(merges[j].PinnedAt) })
	return merges, err
}

func validateMerge(value Merge, name string) error {
	if value.Version != CurrentVersion || value.PinnedAt.IsZero() {
		return fmt.Errorf("merge record requires version %d and pinned_at", CurrentVersion)
	}
	if !coderef.ValidCommit(value.Commit) || name != MergeFilename(value.Commit) {
		return fmt.Errorf("merge record must be named <commit>.json for its full landed commit")
	}
	if value.Base != "" && !coderef.ValidCommit(value.Base) {
		return fmt.Errorf("merge record base must be a full commit")
	}
	if value.Review != "" && !ValidID(value.Review) {
		return fmt.Errorf("merge record review must be a review id")
	}
	for index, commit := range value.Commits {
		if !coderef.ValidCommit(commit.Commit) || strings.TrimSpace(commit.Subject) == "" || commit.Date.IsZero() {
			return fmt.Errorf("merged commit %d requires a full commit, a date, and a subject", index+1)
		}
	}
	return nil
}

func loadMetaJSON(dir string, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			fn(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

func readJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("parse %s: more than one JSON value", filepath.Base(path))
		}
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func displayPath(section, name string) string {
	if section == "" {
		return name
	}
	return filepath.ToSlash(filepath.Join(section, name))
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// structuralEntry classifies a directory entry that is expected to be a real
// entity directory. os.ReadDir reports a symlink as a non-directory, so an
// unchecked "skip anything that is not a directory" loop would silently drop a
// symlinked chapter, fragment, thread, or message and still report the saga as
// valid.
func structuralEntry(entry fs.DirEntry, suffix string) (matches bool, problem string) {
	if !strings.HasSuffix(entry.Name(), suffix) {
		return false, ""
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		return true, fmt.Sprintf("%s entries must be real directories, not symlinks", suffix)
	}
	if !entry.IsDir() {
		return true, fmt.Sprintf("%s entries must be directories", suffix)
	}
	return true, ""
}

func knownReservedDirectory(name string, hierarchy hierarchyRoot) bool {
	switch hierarchy {
	case sagaHierarchy:
		switch name {
		case CodeDirName, ReviewsDir, "___claims", "___verifications", MergesDir,
			applayout.OverviewDir, applayout.PersonasDir, applayout.DesignSystemDir,
			applayout.OnboardingDir, applayout.FeatureFlagsDir, applayout.FeaturesDir:
			return true
		}
		return false
	case featureHierarchy:
		for _, known := range applayout.FeatureRootDirs {
			if name == known {
				return true
			}
		}
		return false
	}
	return name == CodeDirName
}

func metadataDirectorySafe(root, sectionDir, name string, validation *Validation) bool {
	path := filepath.Join(sectionDir, name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil {
		addIssue(validation, "error", relativePath(root, path), err.Error())
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		addIssue(validation, "error", relativePath(root, path), "reserved metadata path must be a real directory")
		return false
	}
	return true
}

func validateCodeFile(value CodeFile, validation *Validation) {
	if value.Version != CurrentVersion {
		addIssue(validation, "error", value.Path, fmt.Sprintf("unsupported version %d; expected %d", value.Version, CurrentVersion))
	}
	if len(value.References) == 0 {
		// schema/v5/code.schema.json requires references with minItems 1: an
		// evidence file that references nothing is not a valid record.
		addIssue(validation, "error", value.Path, "code evidence must contain at least one code reference")
	}
	seen := map[string]bool{}
	for i, reference := range value.References {
		if err := coderef.Validate(reference); err != nil {
			addIssue(validation, "error", value.Path, fmt.Sprintf("reference %d: %v", i+1, err))
		}
		if seen[reference.Key()] {
			addIssue(validation, "error", value.Path, fmt.Sprintf("reference %d duplicates an earlier reference", i+1))
		}
		seen[reference.Key()] = true
	}
}

// earlierRecord defines the total order the format uses for append-only review
// records. SPEC.md resolves state from the latest record by created_at; two
// records can legitimately share a timestamp, so the record id breaks the tie.
// Without it, "the latest event" would depend on directory listing order and a
// thread could resolve differently on two machines holding the same commit.
func earlierRecord(leftTime time.Time, leftID string, rightTime time.Time, rightID string) bool {
	if !leftTime.Equal(rightTime) {
		return leftTime.Before(rightTime)
	}
	return leftID < rightID
}

func addIssue(validation *Validation, severity, path, message string) {
	validation.Issues = append(validation.Issues, Issue{Severity: severity, Path: path, Message: message})
}
