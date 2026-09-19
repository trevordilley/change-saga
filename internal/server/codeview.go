package server

import (
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// CodeReviewView is the complete template contract for the focused code view.
// Files remains flat for lookup and keyboard navigation; Tree is the nested
// presentation model and SelectedFile is the only file intended for rendering.
type CodeReviewView struct {
	Files              []*FileDiffView
	Tree               ChangedFileTreeView
	SelectedFile       *FileDiffView
	SelectedDiff       *diffAtomView
	SelectedDiffs      []*diffAtomView
	SelectedRef        string
	RelatedSaga        []*RelatedSagaChapterView
	RelatedEmpty       string
	NarrativeOwnership []*FragmentOwnershipView
	ReviewedFiles      int
}

type FileDiffView struct {
	ID             string
	Name           string
	Path           string
	Ref            string
	Href           string
	Atoms          []*diffAtomView
	Lines          []*DiffLineView
	Added          int
	Deleted        int
	Reviewed       bool
	Reviewer       string
	ReviewerDetail string
	Selected       bool
}

// reviewDiffSurfaceView is the shared template contract for a reviewable file
// diff, whether it is opened from Code Diff or beside narrative evidence.
// CodeHref is populated only when the surface needs a route back to Code Diff.
type reviewDiffSurfaceView struct {
	Path     string
	CodeHref string
}

// DiffLineView is presentation-only. Atom is populated for changed lines and
// events so existing fully-qualified comment and suggestion contracts remain
// attached to the same diff atom.
type DiffLineView struct {
	Kind    string
	Path    string
	OldLine int
	NewLine int
	Content string
	Event   string
	OldPath string
	NewPath string
	Atom    *diffAtomView
	// Linked marks a row inside a whole-file diff as evidence for the narrative
	// target that requested it, so a drawer can distinguish the lines its
	// explanation covers from the surrounding file it shows for context.
	Linked bool
}

type ChangedFileTreeView struct {
	Nodes         []*ChangedFileTreeNode
	FileCount     int
	ReviewedCount int
	Added         int
	Deleted       int
}

type ChangedFileTreeNode struct {
	Name          string
	Path          string
	Kind          string
	Depth         int
	Children      []*ChangedFileTreeNode
	File          *FileDiffView
	FileCount     int
	ReviewedCount int
	Added         int
	Deleted       int
	Selected      bool
	Expanded      bool
}

type RelatedSagaChapterView struct {
	ID        string
	Title     string
	Target    string
	Anchor    string
	Href      string
	Deck      bool
	Fragments []*RelatedSagaFragmentView
}

type RelatedSagaFragmentView struct {
	ID      string
	Title   string
	Target  string
	Excerpt string
	Anchor  string
	Href    string
	Refs    []string
	Slide   *SlideReferenceView
}

// SlideReferenceView is the common visual language for a slide referenced
// outside the presentation surface. Evidence remains owned by Items, while
// navigation rolls those references up to the slide a reviewer can recognize.
type SlideReferenceView struct {
	ID          string
	Title       string
	Section     string
	Target      string
	Anchor      string
	Href        string
	URL         string
	MediaType   string
	ReviewState string
	ItemCount   int
}

// FragmentOwnershipView is the forward fragment-to-diff half of the ownership
// contract. Unavailable links retain their original fully-qualified URI.
type FragmentOwnershipView struct {
	ChapterID    string
	ChapterTitle string
	FragmentID   string
	Title        string
	Target       string
	Anchor       string
	Href         string
	Diffs        []*NarrativeDiffView
}

type NarrativeDiffView struct {
	Ref         string
	Note        string
	Available   bool
	Reason      string
	FilePath    string
	Href        string
	MatchedRefs []string
}

type codeSelection struct {
	filePath string
	ref      string
}

type selectionError struct {
	status  int
	message string
}

func (e *selectionError) Error() string { return e.message }

func codeSelectionFromRequest(r *http.Request) codeSelection {
	return codeSelection{filePath: r.URL.Query().Get("file"), ref: r.URL.Query().Get("ref")}
}

// CodeDiffURL creates a stable, escaped URL for a focused file and an optional
// code-location selection. Paths and locations are never shortened.
func CodeDiffURL(filePath, ref string) string {
	return codeDiffURLAt("/", filePath, ref)
}

func codeDiffURLAt(basePath, filePath, ref string) string {
	if basePath == "" || !strings.HasPrefix(basePath, "/") {
		basePath = "/"
	}
	query := url.Values{"view": {"code"}}
	if filePath != "" {
		query.Set("file", filePath)
	}
	if ref != "" {
		query.Set("ref", ref)
	}
	return strings.TrimSuffix(basePath, "?") + "?" + query.Encode()
}

func rebaseCodeReviewURLs(view *CodeReviewView, basePath string) {
	for _, file := range view.Files {
		file.Href = codeDiffURLAt(basePath, file.Path, "")
	}
	for _, ownership := range view.NarrativeOwnership {
		for _, diff := range ownership.Diffs {
			if diff.Available {
				diff.Href = codeDiffURLAt(basePath, diff.FilePath, diff.Ref)
			}
		}
	}
}

func makeCodeReviewView(document *saga.Saga, changes gitdiff.ChangeSet, report coverage.Report, threads map[string][]*threadView, selection codeSelection) (*CodeReviewView, *selectionError) {
	files := makeFileViews(changes, saga.SagaTarget(document.Manifest.ID), document.FileReviews, threads)
	view := &CodeReviewView{Files: files}
	for _, file := range files {
		file.Name = path.Base(file.Path)
		file.Href = CodeDiffURL(file.Path, "")
		if file.Reviewed {
			view.ReviewedFiles++
		}
	}

	selected, selectedAtoms, err := resolveCodeSelection(files, selection, changes.BaseOID, changes.HeadOID)
	if err != nil {
		return nil, err
	}
	view.SelectedFile = selected
	view.SelectedRef = selection.ref
	view.SelectedDiffs = selectedAtoms
	for _, atom := range selectedAtoms {
		atom.Selected = true
	}
	if len(selectedAtoms) == 1 {
		view.SelectedDiff = selectedAtoms[0]
	}
	if selected != nil {
		selected.Selected = true
	}

	view.Tree = makeChangedFileTree(files)
	locations := indexNarrativeFragments(document)
	view.NarrativeOwnership = makeFragmentOwnershipViews(locations, changes, report)
	if selected != nil {
		scope := selected.Atoms
		if selection.ref != "" {
			scope = selectedAtoms
		}
		view.RelatedSaga = makeRelatedSagaViews(locations, scope, report.Ownership)
		if len(view.RelatedSaga) == 0 {
			view.RelatedEmpty = "Nothing in the story explains this selection yet."
		}
	}
	return view, nil
}

func resolveCodeSelection(files []*FileDiffView, selection codeSelection, baseOID, headOID string) (*FileDiffView, []*diffAtomView, *selectionError) {
	var selected *FileDiffView
	if selection.filePath != "" {
		for _, file := range files {
			if file.Path == selection.filePath {
				selected = file
				break
			}
		}
		if selected == nil {
			return nil, nil, &selectionError{status: http.StatusNotFound, message: "changed file not found"}
		}
	}

	var selector coderef.Location
	if selection.ref != "" {
		var err error
		selector, err = coderef.ParseLocation(selection.ref)
		if err != nil {
			return nil, nil, &selectionError{status: http.StatusBadRequest, message: "invalid selected code location"}
		}
		if selector.Commit != baseOID && selector.Commit != headOID {
			return nil, nil, &selectionError{status: http.StatusNotFound, message: "selected code is not part of the comparison"}
		}
		if selected == nil {
			for _, file := range files {
				if fileHasPath(file, selector.Path) {
					selected = file
					break
				}
			}
		}
	}
	if selected == nil && len(files) > 0 {
		selected = files[0]
	}
	if selection.ref == "" {
		return selected, nil, nil
	}
	if selected == nil {
		return nil, nil, &selectionError{status: http.StatusNotFound, message: "selected diff is not part of the comparison"}
	}
	if selector.WholeFile() {
		if !fileHasPath(selected, selector.Path) {
			return nil, nil, &selectionError{status: http.StatusNotFound, message: "selected diff is not part of the changed file"}
		}
		return selected, selected.Atoms, nil
	}
	var atoms []*diffAtomView
	for _, atom := range selected.Atoms {
		if location, err := coderef.ParseLocation(atom.Ref); err == nil && selector.Contains(location) {
			atoms = append(atoms, atom)
		}
	}
	if len(atoms) == 0 {
		return nil, nil, &selectionError{status: http.StatusNotFound, message: "selected diff is not part of the changed file"}
	}
	return selected, atoms, nil
}

func makeChangedFileTree(files []*FileDiffView) ChangedFileTreeView {
	root := &ChangedFileTreeNode{Kind: "folder"}
	for _, file := range files {
		current := root
		parts := strings.Split(file.Path, "/")
		for index, name := range parts {
			if index == len(parts)-1 {
				current.Children = append(current.Children, &ChangedFileTreeNode{Name: name, Path: file.Path, Kind: "file", File: file})
				continue
			}
			folderPath := strings.Join(parts[:index+1], "/")
			var folder *ChangedFileTreeNode
			for _, child := range current.Children {
				if child.Kind == "folder" && child.Name == name {
					folder = child
					break
				}
			}
			if folder == nil {
				folder = &ChangedFileTreeNode{Name: name, Path: folderPath, Kind: "folder"}
				current.Children = append(current.Children, folder)
			}
			current = folder
		}
	}
	finalizeTreeNode(root, -1)
	return ChangedFileTreeView{Nodes: root.Children, FileCount: root.FileCount, ReviewedCount: root.ReviewedCount, Added: root.Added, Deleted: root.Deleted}
}

func finalizeTreeNode(node *ChangedFileTreeNode, depth int) {
	node.Depth = depth
	if node.File != nil {
		node.FileCount, node.Added, node.Deleted = 1, node.File.Added, node.File.Deleted
		node.Selected = node.File.Selected
		if node.File.Reviewed {
			node.ReviewedCount = 1
		}
		return
	}
	sort.SliceStable(node.Children, func(i, j int) bool {
		if node.Children[i].Kind != node.Children[j].Kind {
			return node.Children[i].Kind == "folder"
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	for _, child := range node.Children {
		finalizeTreeNode(child, depth+1)
		node.FileCount += child.FileCount
		node.ReviewedCount += child.ReviewedCount
		node.Added += child.Added
		node.Deleted += child.Deleted
		node.Selected = node.Selected || child.Selected
	}
	// Folders open by default, matching the changed-file trees reviewers use
	// elsewhere; collapsing is an explicit choice, not the starting state.
	node.Expanded = true
}

type narrativeLocation struct {
	fragment         *saga.Fragment
	target           string
	itemID           string
	title            string
	diffs            []saga.CodeFile
	hasDiffs         bool
	chapterID        string
	chapterTitle     string
	chapterTarget    string
	chapterHref      string
	fragmentHref     string
	deckID           string
	deckTitle        string
	deckTarget       string
	deckHref         string
	slideID          string
	slideTitle       string
	slideTarget      string
	slideHref        string
	slideURL         string
	slideMediaType   string
	slideReviewState string
}

func indexNarrativeFragments(document *saga.Saga) []narrativeLocation {
	var result []narrativeLocation
	var walk func(*saga.Section, narrativeLocation)
	walk = func(section *saga.Section, chapter narrativeLocation) {
		if section.Kind == "chapter" {
			chapter.chapterID, chapter.chapterTitle, chapter.chapterTarget = section.ID, section.Title, section.Target
			chapter.chapterHref = sagaHref(section.Target)
		} else if section.Kind == "deck" {
			chapter.deckID, chapter.deckTitle, chapter.deckTarget = section.ID, section.Title, section.Target
			chapter.deckHref = sagaHref(section.Target)
		}
		for _, fragment := range section.Fragments {
			location := chapter
			if location.chapterTitle == "" {
				location.chapterTitle, location.chapterTarget, location.chapterHref = "Overview", document.Section.Target, sagaHref(document.Section.Target)
			}
			location.fragment = fragment
			location.target, location.itemID, location.title, location.diffs, location.hasDiffs = fragment.Target, fragment.ID, fragment.Title, fragment.Code, fragment.HasCode
			location.fragmentHref = sagaHref(fragment.Target)
			if fragment.SlideMeta != nil {
				location.slideID, location.slideTitle, location.slideTarget = fragment.ID, fragment.Title, fragment.Target
				location.slideHref = location.fragmentHref
				location.slideURL = fragmentAssetURL(fragment)
				location.slideMediaType = fragment.MediaType
				location.slideReviewState, _, _, _ = latestReview(fragment.Reviews)
			}
			result = append(result, location)
			for index := range fragment.Landmarks {
				landmark := &fragment.Landmarks[index]
				landmarkLocation := location
				landmarkLocation.target, landmarkLocation.itemID, landmarkLocation.title, landmarkLocation.diffs, landmarkLocation.hasDiffs = landmark.Target, landmark.ID, landmark.Label, landmark.Code, landmark.HasCode
				landmarkLocation.fragmentHref = sagaHref(fragment.Target) + "--" + landmark.ID
				result = append(result, landmarkLocation)
			}
		}
		for _, child := range section.Children {
			walk(child, chapter)
		}
	}
	walk(document.Section, narrativeLocation{})
	return result
}

func makeRelatedSagaViews(locations []narrativeLocation, atoms []*diffAtomView, ownership map[string][]coverage.Assignment) []*RelatedSagaChapterView {
	ownedURIs := map[string][]string{}
	for _, atom := range atoms {
		for _, assignment := range ownership[atom.Key] {
			uris := ownedURIs[assignment.Target]
			if !contains(uris, atom.Ref) {
				ownedURIs[assignment.Target] = append(uris, atom.Ref)
			}
		}
	}
	return makeRelatedSagaViewsForTargets(locations, ownedURIs)
}

func makeRelatedSagaViewsForTargets(locations []narrativeLocation, ownedURIs map[string][]string) []*RelatedSagaChapterView {
	groups := map[string]*RelatedSagaChapterView{}
	slides := map[string]*RelatedSagaFragmentView{}
	var result []*RelatedSagaChapterView
	for _, location := range locations {
		uris := ownedURIs[location.target]
		if len(uris) == 0 {
			continue
		}
		groupKey := location.chapterTarget
		groupID, groupTitle, groupTarget, groupHref := location.chapterID, location.chapterTitle, location.chapterTarget, location.chapterHref
		if location.slideTarget != "" {
			groupKey = location.deckTarget
			groupID, groupTitle, groupTarget, groupHref = location.deckID, location.deckTitle, location.deckTarget, location.deckHref
		}
		group := groups[groupKey]
		if group == nil {
			group = &RelatedSagaChapterView{ID: groupID, Title: groupTitle, Target: groupTarget, Anchor: domID(groupTarget), Href: groupHref, Deck: location.slideTarget != ""}
			groups[groupKey] = group
			result = append(result, group)
		}
		if location.slideTarget != "" {
			key := groupKey + "\x00" + location.slideTarget
			view := slides[key]
			if view == nil {
				view = &RelatedSagaFragmentView{
					ID: location.slideID, Title: location.slideTitle, Target: location.slideTarget,
					Anchor: strings.TrimPrefix(location.slideHref, "#"), Href: location.slideHref,
					Slide: &SlideReferenceView{
						ID: location.slideID, Title: location.slideTitle, Target: location.slideTarget,
						Anchor: strings.TrimPrefix(location.slideHref, "#"), Href: location.slideHref,
						URL: location.slideURL, MediaType: location.slideMediaType, ReviewState: location.slideReviewState,
					},
				}
				slides[key] = view
				group.Fragments = append(group.Fragments, view)
			}
			for _, uri := range uris {
				if !contains(view.Refs, uri) {
					view.Refs = append(view.Refs, uri)
				}
			}
			if location.target != location.slideTarget {
				view.Slide.ItemCount++
			}
			continue
		}
		title := location.title
		if title == "" {
			title = location.itemID
		}
		group.Fragments = append(group.Fragments, &RelatedSagaFragmentView{
			ID: location.itemID, Title: title, Target: location.target,
			Excerpt: fragmentExcerpt(location.fragment), Anchor: strings.TrimPrefix(location.fragmentHref, "#"), Href: location.fragmentHref, Refs: uris,
		})
	}
	return result
}

func makeFragmentOwnershipViews(locations []narrativeLocation, changes gitdiff.ChangeSet, report coverage.Report) []*FragmentOwnershipView {
	stale := map[ownershipReferenceID]coverage.StaleReference{}
	for _, value := range report.StaleReferences {
		key := ownershipReferenceKey(value.Assignment.Target, value.Assignment.EvidenceFile, value.Assignment.Reference)
		stale[key] = value
	}
	// Coverage has already parsed every selector and recorded its exact
	// target/file/index assignment. Index those results once instead of
	// reparsing and testing every changed atom for every narrative reference.
	matchedAtoms := map[ownershipReferenceID][]*gitdiff.Atom{}
	for atomIndex := range changes.Atoms {
		atom := &changes.Atoms[atomIndex]
		for _, assignment := range report.Ownership[atom.Key] {
			key := ownershipReferenceKey(assignment.Target, assignment.EvidenceFile, assignment.Reference)
			matchedAtoms[key] = append(matchedAtoms[key], atom)
		}
	}
	var result []*FragmentOwnershipView
	for _, location := range locations {
		view := &FragmentOwnershipView{
			ChapterID: location.chapterID, ChapterTitle: location.chapterTitle,
			FragmentID: location.itemID, Title: location.title,
			Target: location.target, Anchor: domID(location.target), Href: location.fragmentHref,
		}
		for _, diffFile := range location.diffs {
			for index, reference := range diffFile.References {
				link := &NarrativeDiffView{Ref: reference.Location().String(), Note: reference.Note}
				key := ownershipReferenceKey(location.target, diffFile.Path, index+1)
				if value, ok := stale[key]; ok {
					link.Reason = value.Reason
					view.Diffs = append(view.Diffs, link)
					continue
				}
				matched := matchedAtoms[key]
				for _, atom := range matched {
					link.Available = true
					link.MatchedRefs = append(link.MatchedRefs, atom.Ref)
				}
				if link.Available {
					link.FilePath = effectiveAtomPath(*matched[0])
					link.Href = CodeDiffURL(link.FilePath, spanningLocation(changes, matched))
				} else {
					link.Reason = "the referenced code is unchanged in this comparison"
				}
				view.Diffs = append(view.Diffs, link)
			}
		}
		result = append(result, view)
	}
	return result
}

type ownershipReferenceID struct {
	target string
	file   string
	index  int
}

func ownershipReferenceKey(target, file string, index int) ownershipReferenceID {
	return ownershipReferenceID{target: target, file: file, index: index}
}

func effectiveAtomPath(atom gitdiff.Atom) string {
	if atom.NewPath != "" {
		return atom.NewPath
	}
	return atom.Path
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

var (
	nonContentHTML = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)
	htmlTag        = regexp.MustCompile(`(?s)<[^>]*>`)
	markdownMark   = regexp.MustCompile(`(?m)(^|\s)[#>*_~` + "`" + `]+`)
	headingAnchor  = regexp.MustCompile(`\{#[A-Za-z0-9_-]+\}`)
	// A heading is a label, not the sentence a reviewer wants to skim, and the
	// title is already shown beside the excerpt. Dropping headings stops them
	// running into the following paragraph.
	markdownHeading = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+.*$`)
	// Unwrap inline spans so the excerpt keeps the words and loses the syntax.
	// Stripping only leading markers used to leave the closing delimiter behind.
	markdownCode     = regexp.MustCompile("`+([^`]*)`+")
	markdownEmphasis = regexp.MustCompile(`\*{1,3}([^*\n]+)\*{1,3}`)
)

func fragmentExcerpt(fragment *saga.Fragment) string {
	fallback := fragment.Title
	if fallback == "" {
		fallback = fragment.ID
	}
	if !strings.HasPrefix(fragment.MediaType, "text/") && fragment.MediaType != "image/svg+xml" {
		return fallback
	}
	file, err := openFragmentEntrypoint(fragment)
	if err != nil {
		return fallback
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64<<10))
	if err != nil || !utf8.Valid(data) {
		return fallback
	}
	text := string(data)
	if fragment.MediaType == "text/html" || fragment.MediaType == "image/svg+xml" {
		text = nonContentHTML.ReplaceAllString(text, " ")
		text = htmlTag.ReplaceAllString(text, " ")
		text = stdhtml.UnescapeString(text)
	} else if fragment.MediaType == "text/markdown" {
		text = nonContentHTML.ReplaceAllString(text, " ")
		text = htmlTag.ReplaceAllString(text, " ")
		// Stable heading anchors are addressing syntax, not prose.
		text = headingAnchor.ReplaceAllString(text, " ")
		if prose := strings.TrimSpace(markdownHeading.ReplaceAllString(text, " ")); prose != "" {
			text = prose
		}
		text = markdownCode.ReplaceAllString(text, "$1")
		text = markdownEmphasis.ReplaceAllString(text, "$1")
		text = markdownMark.ReplaceAllString(text, "$1")
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return fallback
	}
	return truncateExcerpt(text, 180)
}

func openFragmentEntrypoint(fragment *saga.Fragment) (*os.File, error) {
	realRoot, err := filepath.EvalSymlinks(fragment.Directory)
	if err != nil {
		return nil, err
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint)))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(realRoot, realPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("fragment entrypoint escapes package")
	}
	return os.Open(realPath)
}

func truncateExcerpt(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	cut := limit
	for cut > limit-30 && cut > 0 && !strings.ContainsRune(" \t\n", runes[cut-1]) {
		cut--
	}
	if cut <= limit-30 {
		cut = limit
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

// fileHasPath reports whether a changed file is known by path on either side.
func fileHasPath(file *FileDiffView, value string) bool {
	if file.Path == value {
		return true
	}
	for _, atom := range file.Atoms {
		if atom.Path == value || atom.OldPath == value || atom.NewPath == value {
			return true
		}
	}
	return false
}

// spanningLocation is the smallest comparison location holding the first
// matched atom's side of the file: a line range when the atoms are lines on
// one side, and the whole file otherwise.
func spanningLocation(changes gitdiff.ChangeSet, atoms []*gitdiff.Atom) string {
	first := changes.Location(*atoms[0])
	if first.WholeFile() {
		return first.String()
	}
	span := first
	for _, atom := range atoms[1:] {
		location := changes.Location(*atom)
		if location.Commit != span.Commit || location.Path != span.Path || location.WholeFile() {
			continue
		}
		if location.Start < span.Start {
			span.Start = location.Start
		}
		if location.End > span.End {
			span.End = location.End
		}
	}
	return span.String()
}
