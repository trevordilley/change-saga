package server

import (
	"bytes"
	"context"
	"net/http"
	"sort"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A review's Code Diff and its coverage are the two comparison views of the
// change it explains, and they live on the Review side: opening a review
// gives its deck, its Code Diff, and its coverage. Both are read over the
// review's own range, so they are the change under review rather than
// whatever comparison the reviewer happened to be opened with.

// reviewRange loads the review a request names together with its current
// range. It answers the request itself when either cannot be read.
func (a *app) reviewRange(w http.ResponseWriter, r *http.Request) (*saga.Saga, *saga.Review, reviewstate.Range, bool) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return nil, nil, reviewstate.Range{}, false
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil {
		http.NotFound(w, r)
		return nil, nil, reviewstate.Range{}, false
	}
	rng, err := reviewstate.ResolveRange(r.Context(), a.sourceDir, review)
	if err != nil {
		http.Error(w, "The review's range could not be read: "+err.Error(), http.StatusConflict)
		return nil, nil, reviewstate.Range{}, false
	}
	return document, review, rng, true
}

// reviewCatalog is the changed-file metadata of a review's range: the same
// bounded catalog the comparison's Code Diff reads, over base..head instead.
func (a *app) reviewCatalog(ctx context.Context, document *saga.Saga, rng reviewstate.Range) (gitdiff.Catalog, error) {
	return gitdiff.ReadCatalogWithOptions(ctx, a.sourceDir, document.Manifest.Source.Repository, rng.BaseOID, rng.HeadOID, gitdiff.ReadOptions{AllowRepositoryMismatch: true})
}

// reviewCodeSurface is the review's Code Diff: every changed file of its
// range, with the selected file's hunks streamed in. It is the comparison
// Code Diff's page, given the review's catalog and its own routes back.
func (a *app) reviewCodeSurface(w http.ResponseWriter, r *http.Request) {
	document, review, rng, ok := a.reviewRange(w, r)
	if !ok {
		return
	}
	catalog, err := a.reviewCatalog(r.Context(), document, rng)
	if err != nil {
		http.Error(w, "The review's comparison could not be loaded.", http.StatusInternalServerError)
		return
	}
	selectedPath, selectionErr := selectedCatalogPath(catalog, r)
	if selectionErr != nil {
		status := http.StatusBadRequest
		if notFound, ok := selectionErr.(*selectionError); ok {
			status = notFound.status
		}
		http.Error(w, selectionErr.Error(), status)
		return
	}
	base := reviewHref(review.ID)
	window, err := pageRequest(r, "review-code\x00"+sourceCatalogIdentity(catalog)+"\x00"+r.URL.Query().Get("file"), len(catalog.Files), defaultSurfacePageLimit, maxSurfacePageLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fileStart, fileEnd := boundedSlice(window.start, window.end, len(catalog.Files))
	files := make([]*FileDiffView, 0, fileEnd-fileStart)
	for _, summary := range catalog.Files[fileStart:fileEnd] {
		file := catalogFileView(catalog, summary)
		file.Href = codeDiffURLAt(base, file.Path, "")
		file.Selected = summary.Path == selectedPath
		files = append(files, file)
	}
	var selected *FileDiffView
	if selectedPath != "" {
		index := sort.Search(len(catalog.Files), func(index int) bool { return catalog.Files[index].Path >= selectedPath })
		if index < len(catalog.Files) && catalog.Files[index].Path == selectedPath {
			selected = catalogFileView(catalog, catalog.Files[index])
			selected.Href, selected.Selected = codeDiffURLAt(base, selected.Path, ""), true
		}
	}
	result := codePageView{
		Tree: makeChangedFileTree(files), Selected: selected,
		DiffHref: base + "/file-diff", EmptyNote: "This review's range changes no files.",
		// This stream reads every row, so request the supported maximum to
		// avoid repeating validation and selected-file work for tiny pages.
		DiffLimit:  maxDiffPageLimit,
		TotalFiles: len(catalog.Files), NextCursor: window.next, HasMore: window.hasMore(), Returned: window.end - window.start,
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	writePageHeaders(w, window)
	renderHTML(w, a.template, "code-page", result, "The review's code page could not be rendered.")
}

// reviewFileDiffSurface streams one file's hunks from the review's range.
func (a *app) reviewFileDiffSurface(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		http.Error(w, "missing changed file", http.StatusBadRequest)
		return
	}
	document, _, rng, ok := a.reviewRange(w, r)
	if !ok {
		return
	}
	catalog, err := a.reviewCatalog(r.Context(), document, rng)
	if err != nil {
		http.Error(w, "The review's comparison could not be loaded.", http.StatusInternalServerError)
		return
	}
	file, found := catalogFile(catalog, filePath)
	if !found {
		http.Error(w, "changed file not found", http.StatusNotFound)
		return
	}
	changes, err := gitdiff.ReadFile(r.Context(), a.sourceDir, catalog, file)
	if err != nil {
		http.Error(w, "The file diff could not be loaded.", http.StatusInternalServerError)
		return
	}
	selected := catalogFileView(catalog, file)
	for _, candidate := range makeFileViews(changes, saga.SagaTarget(document.Manifest.ID)) {
		if candidate.Path == filePath {
			selected = candidate
			break
		}
	}
	window, err := pageRequest(r, "review-file-diff\x00"+sourceCatalogIdentity(catalog)+"\x00"+filePath, len(selected.Lines), defaultDiffPageLimit, maxDiffPageLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	selected.Lines = selected.Lines[window.start:window.end]
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	writePageHeaders(w, window)
	page := fileDiffPageView{File: selected, NextCursor: window.next, HasMore: window.hasMore(), Returned: window.end - window.start}
	renderHTML(w, a.template, "file-diff-page", page, "The file diff could not be rendered.")
}

// reviewCoverageSurface is how completely the review's deck explains its own
// range, on its own tab rather than crowded beside the slides.
func (a *app) reviewCoverageSurface(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil {
		http.NotFound(w, r)
		return
	}
	report := a.reviewReports(r.Context(), document, []*saga.Review{review})[0]
	view := reviewCoverageSurfaceView{Report: report}
	if report.Coverage != nil {
		view.Coverage = reviewCoverage(report.Coverage)
	}
	var body bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&body, "review-coverage-surface", view); err != nil {
		http.Error(w, "The review's coverage could not be rendered.", http.StatusInternalServerError)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

// reviewCoverageSurfaceView is the coverage tab of one review.
type reviewCoverageSurfaceView struct {
	Report   reviewstate.Report
	Coverage *reviewCoverageView
}
