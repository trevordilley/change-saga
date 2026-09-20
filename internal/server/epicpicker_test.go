package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func pickerSaga() *saga.Saga {
	return &saga.Saga{
		Manifest: saga.Manifest{ID: appNavSaga},
		Section:  &saga.Section{ID: appNavSaga, Target: saga.SagaTarget(appNavSaga)},
		Epics: []*saga.Epic{
			appNavEpic("billing", "Billing"),
			appNavEpic("catalog", "Catalog"),
		},
	}
}

func pickerRequest(cookie string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: currentEpicCookie, Value: cookie})
	}
	return request
}

// The sidebar shows one epic, resolved in three steps: the page being read,
// then the reader's last choice, then the first epic an author introduced.
func TestTheCurrentEpicIsThePageThenTheChoiceThenTheFirst(t *testing.T) {
	document := pickerSaga()
	for _, want := range []struct {
		name     string
		pageEpic string
		cookie   string
		epic     string
	}{
		{"the page wins over the choice", "catalog", "billing", "catalog"},
		{"the choice is used off an epic's pages", "", "catalog", "catalog"},
		{"no choice falls back to the first epic", "", "", "billing"},
		{"a choice that is not an epic falls back", "", "retired-epic", "billing"},
		{"a page epic that is not an epic falls back", "gone", "catalog", "catalog"},
	} {
		t.Run(want.name, func(t *testing.T) {
			if got := resolveCurrentEpic(document, want.pageEpic, pickerRequest(want.cookie)); got != want.epic {
				t.Fatalf("current epic = %q, want %q", got, want.epic)
			}
		})
	}
	// An app with no epics has none to show, and says so rather than guessing.
	empty := &saga.Saga{Manifest: saga.Manifest{ID: appNavSaga}}
	if got := resolveCurrentEpic(empty, "billing", pickerRequest("billing")); got != "" {
		t.Fatalf("an app with no epics resolved %q", got)
	}
}

// A page names its epic when it belongs to one, and nothing otherwise, so a
// page that belongs to no epic never overwrites the reader's choice.
func TestThePageEpicIsTheRecordsOwnEpic(t *testing.T) {
	records := requirements.Document{
		SagaID:  appNavSaga,
		Stories: []requirements.Story{appNavStory("catalog", "list-item", 1, requirements.StateProposed)},
	}
	page, _, err := makeRequirementsSurface(records, requirementRoute{active: true, storyID: "list-item"})
	if err != nil {
		t.Fatal(err)
	}
	tests := quality.Document{SagaID: appNavSaga, TestCases: []quality.TestCase{
		{Identity: quality.TestCaseIdentity{ID: "checkout-charges"}, Epic: "billing"},
	}}
	for _, want := range []struct {
		route appRoute
		epic  string
	}{
		{appRoute{kind: "epic", id: "catalog"}, "catalog"},
		{appRoute{kind: "requirements"}, "catalog"},
		{appRoute{kind: "test", id: "checkout-charges"}, "billing"},
		{appRoute{kind: "test", id: "unknown"}, ""},
		{appRoute{kind: "overview"}, ""},
		{appRoute{kind: "terms"}, ""},
		{appRoute{kind: "persona", id: "buyer"}, ""},
		// The index browses every epic; browsing must not change which one
		// the sidebar shows.
		{appRoute{kind: "epics"}, ""},
	} {
		if got := pageEpic(want.route, page, tests); got != want.epic {
			t.Fatalf("page epic of %s/%s = %q, want %q", want.route.kind, want.route.id, got, want.epic)
		}
	}
	// The requirements overview names no single story, so it names no epic.
	overview, _, err := makeRequirementsSurface(records, requirementRoute{active: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageEpic(appRoute{kind: "requirements"}, overview, tests); got != "" {
		t.Fatalf("the requirements overview claimed epic %q", got)
	}
}

// The choice is a reading preference: it is stored in a cookie so the next
// server-rendered page opens on the same epic, and nowhere else.
func TestTheReadersEpicIsRememberedInACookieAndNeverWritten(t *testing.T) {
	recorder := httptest.NewRecorder()
	rememberEpic(recorder, pickerRequest(""), "catalog")
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != currentEpicCookie || cookies[0].Value != "catalog" {
		t.Fatalf("remembered cookies = %#v", cookies)
	}
	if cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("the preference cookie is not scoped to this reviewer: %#v", cookies[0])
	}
	// Re-stating the same choice, or none, writes nothing.
	for _, epic := range []string{"catalog", ""} {
		quiet := httptest.NewRecorder()
		rememberEpic(quiet, pickerRequest("catalog"), epic)
		if got := quiet.Result().Cookies(); len(got) != 0 {
			t.Fatalf("remembering %q again wrote %#v", epic, got)
		}
	}
}

// The index lists every epic in creation order with what each one holds. The
// counts are facts, as status reports them: an epic with nothing is listed
// like any other.
func TestTheEpicsIndexCountsWhatEachEpicHolds(t *testing.T) {
	document := pickerSaga()
	deck, _ := appNavDeck("billing-flow", saga.DeckRoleChange, "charge", "refund")
	deck.Slides = []*saga.Slide{
		{SlideManifest: saga.SlideManifest{ID: "charge"}},
		{SlideManifest: saga.SlideManifest{ID: "refund"}},
	}
	document.Epics[0].Decks = []*saga.Deck{deck}
	records := requirements.Document{
		SagaID: appNavSaga,
		Epics: []applayout.Epic{
			{EpicManifest: applayout.EpicManifest{ID: "billing", Title: "Billing", Description: "Money in.", CreatedAt: time.Unix(1, 0)}},
		},
		Stories: []requirements.Story{
			appNavStory("billing", "pay", 1, requirements.StateAccepted),
			appNavStory("billing", "refund", 2, requirements.StateProposed),
		},
	}
	tests := quality.Document{SagaID: appNavSaga, TestCases: []quality.TestCase{
		{Identity: quality.TestCaseIdentity{ID: "charge-succeeds"}, Epic: "billing"},
	}}
	index := epicsIndex(document, records, tests, "catalog")
	if len(index.Epics) != 2 {
		t.Fatalf("index = %#v", index.Epics)
	}
	billing, catalog := index.Epics[0], index.Epics[1]
	if billing.ID != "billing" || billing.Href != epicHref("billing") || billing.Description != "Money in." || billing.Current {
		t.Fatalf("billing row = %#v", billing)
	}
	if billing.Stories != 2 || billing.TestCases != 1 || billing.Slides != 2 {
		t.Fatalf("billing counts = %#v", billing)
	}
	if !catalog.Current || catalog.Stories != 0 || catalog.TestCases != 0 || catalog.Slides != 0 {
		t.Fatalf("an epic with nothing in it must still be listed: %#v", catalog)
	}
}

// Every epic is in the picker whichever one is current, and exactly one is
// marked, so the control always says where the reader is.
func TestThePickerCarriesEveryEpicWithOneMarked(t *testing.T) {
	document := pickerSaga()
	if picker := makeEpicPicker(&saga.Saga{}, ""); picker != nil {
		t.Fatalf("an app with no epics built a picker: %#v", picker)
	}
	// An unknown current epic still produces a usable control rather than an
	// empty one: it falls back to the first.
	for current, want := range map[string]string{"billing": "billing", "catalog": "catalog", "gone": "billing"} {
		picker := makeEpicPicker(document, current)
		if picker == nil || len(picker.Choices) != 2 || picker.Current.ID != want {
			t.Fatalf("picker for %q = %#v", current, picker)
		}
		if picker.Choices[0].OptionID == picker.Choices[1].OptionID {
			t.Fatalf("picker options share a DOM id: %#v", picker.Choices)
		}
		marked := 0
		for _, choice := range picker.Choices {
			if choice.Current {
				marked++
			}
		}
		if marked != 1 || !picker.Current.Current {
			t.Fatalf("picker for %q marks %d epics current: %#v", current, marked, picker.Choices)
		}
	}
}
