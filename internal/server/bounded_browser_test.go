package server

import (
	"bytes"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestRootTemplateDefersCodeAndCoverageModels(t *testing.T) {
	definition := strings.Index(pageTemplate, `{{define "code-view"}}`)
	if definition < 0 {
		t.Fatal("code response template is missing")
	}
	root := pageTemplate[:definition]
	for _, eager := range []string{`{{.Code`, `{{with .Code`, `{{.Manifest`, `{{with .Manifest`, `template "manifest-view"`, `template "code-view"`} {
		if strings.Contains(root, eager) {
			t.Errorf("root page still renders a bounded review model through %q", eager)
		}
	}
	// Each surface names where it loads from; the Review side's Code Diff and
	// coverage name the change under review, so their href is templated.
	for _, contract := range []string{
		`data-review-surface="code" data-surface-href="{{.ReviewCodeHref}}"`,
		`data-review-surface="manifest" data-surface-href="{{if .ReviewSide}}{{.ReviewCoverageHref}}{{else}}/api/coverage?scope=documented{{end}}"`,
		`data-surface-status`,
		`aria-live="polite"`,
	} {
		if !strings.Contains(root, contract) {
			t.Errorf("root page is missing deferred surface contract %q", contract)
		}
	}
}

func TestRootTemplateKeepsCoverageAvailableWithoutComparisonTotals(t *testing.T) {
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	data := pageData{
		Saga: &saga.Saga{Manifest: saga.Manifest{ID: "cold", Title: "Cold review"}},
		Root: &sectionView{Section: &saga.Section{ID: "cold", Title: "Cold review"}, DOMID: "cold"},
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "page", data); err != nil {
		t.Fatal(err)
	}
	page := rendered.String()
	if !strings.Contains(page, `id="view-tab-manifest"`) || !strings.Contains(page, `data-review-surface="manifest"`) {
		t.Fatal("a cold root hid Coverage while its comparison totals were unavailable")
	}
	if strings.Contains(page, `data-coverage-totals`) {
		t.Fatal("a cold root invented comparison totals")
	}
}

func TestDeferredReviewBrowserSupportsBuildingPaginationAndDeepLinks(t *testing.T) {
	for _, contract := range []string{
		"response.status === 202",
		"response.headers.get('Retry-After')",
		"response.dataset.nextCursor",
		"X-Change-Saga-Next-Cursor",
		"url.searchParams.set('cursor', cursor)",
		"destination.append(...inserted)",
		"history.pushState({view}, '', destination)",
		"history.pushState({view:'saga'}, '', sagaURL)",
		"if (id) void activateLandmark()",
		"hydrateRelatedOwners(root)",
		"/api/file-owners?file=",
		"previous?.controller.abort()",
	} {
		if !strings.Contains(appJavaScript, contract) {
			t.Errorf("async browser contract is missing %q", contract)
		}
	}
}

func TestFragmentIntentPrefetchIsBoundedCancellableAndClickPromotable(t *testing.T) {
	for _, contract := range []string{
		"const maxConcurrentTargetCodeLoads = 2",
		"const targetCodeCacheLimit = 64",
		"if (fragment.dataset.fragmentHref) await hydrateFragment(fragment)",
		"priority:href === targets.direct ? 10 : 1",
		"beginFragmentPrefetch(fragment, 'pointer')",
		"endFragmentPrefetch(fragment, 'pointer')",
		"beginFragmentPrefetch(fragment, 'focus')",
		"cancelTargetCodeScope(scope)",
		"job.controller.abort()",
		"requestTargetCode(href, {interactive:true})",
		"installTargetCodeResponse(job.href, html)",
		"response.dataset.targetCodeTarget",
		"anchor.endsWith('--' + landmarkID)",
		"prepareDiffCitations()",
	} {
		if !strings.Contains(appJavaScript, contract) {
			t.Errorf("fragment intent prefetch is missing %q", contract)
		}
	}
}

func TestActiveSlideLoadsItsAggregateDiffSummary(t *testing.T) {
	for _, contract := range []string{
		"const targetCodeButton = q(':scope > .fragment-head [data-target-code-href]', fragment)",
		"if (targetCodeButton) void hydrateTargetCodeSummary(targetCodeButton)",
		"installTargetCodeResponse(href, await requestTargetCode(href, {priority:20}), button)",
	} {
		if !strings.Contains(appJavaScript, contract) {
			t.Errorf("active-slide linked-code summary is missing %q", contract)
		}
	}
}

func TestSlideDiffSummaryPreviewsOnlyLinkedItems(t *testing.T) {
	for _, contract := range []string{
		"function slideDiffSummaryButton(node)",
		"function setSlideDiffPreview(button, reason, visible)",
		"function landmarkOwnsDiffs(target)",
		"markLandmarkDiffOwnership(target, visual)",
		"setSlideDiffPreview(slideDiffButton, 'pointer', true)",
		"setSlideDiffPreview(slideDiffButton, 'pointer', false)",
		`fragment.classList.toggle('preview-linked-items', reasons.size > 0)`,
	} {
		if !strings.Contains(appJavaScript, contract) {
			t.Errorf("slide diff preview is missing %q", contract)
		}
	}
	for _, contract := range []string{
		`.fragment.preview-linked-items .landmark-hotspot[data-landmark-has-diffs="true"]`,
		`.fragment.preview-linked-items .content-landmark-text[data-landmark-has-diffs="true"]`,
	} {
		if !strings.Contains(pageStyles, contract) {
			t.Errorf("slide diff preview styling is missing %q", contract)
		}
	}
}

func TestSlideChromeDoesNotMaskOrIndentContent(t *testing.T) {
	for _, contract := range []string{
		`.doc-deck>.doc-children{margin:0 0 8px;padding:7px 0 2px;border-left:0;`,
		`.slide-thumbnail-hit:hover,.slide-thumbnail-hit:active{background:transparent}`,
		`.landmark-hotspot:hover,.landmark-hotspot:focus-within,.landmark-hotspot.active,.fragment.preview-linked-items .landmark-hotspot[data-landmark-has-diffs="true"]{border-color:#d39418;background:transparent}`,
	} {
		if !strings.Contains(pageStyles, contract) {
			t.Errorf("slide chrome regression contract is missing %q", contract)
		}
	}
}

func TestCoverageContinuouslyLoadsSummariesAndDefersDetails(t *testing.T) {
	for _, contract := range []string{
		"beginContinuousCoverageLoad(surface)",
		"await loadReviewSurfacePage(button)",
		"await new Promise(resolve => setTimeout(resolve, 0))",
		"hydrateCoverageFile(details)",
		"hydrateCoverageTarget(details)",
		`data-coverage-file-href="/api/coverage-file?file=`,
		`data-coverage-target-href="/api/coverage-target?target=`,
		"Files appear automatically as they are ready.",
	} {
		if !strings.Contains(appJavaScript+pageTemplate, contract) {
			t.Errorf("continuous coverage loading is missing %q", contract)
		}
	}
	if strings.Contains(appJavaScript+pageTemplate, "Fetching a bounded page of the comparison.") || strings.Contains(pageTemplate, "The audit stays out of the initial page.") {
		t.Fatal("Coverage exposes implementation constraints instead of loading state")
	}
}
