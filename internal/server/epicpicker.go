package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The sidebar shows one epic at a time.
//
// Which one is a reading preference, never a fact about the Saga. It is
// resolved in three steps: the epic of the page being read, then the last epic
// the reader chose, then the first epic an author introduced. The reader's
// choice is kept in a cookie because the reviewer is server-rendered: a
// preference only the browser knew would arrive after the first paint, and the
// sidebar would flip epics under the reader. Nothing about it is ever written
// into the Saga.

// currentEpicCookie holds the last epic the reader chose.
const currentEpicCookie = "change-saga-epic"

// epicsIndexHref is the browsable index of every epic. It is also where the
// picker sends a reader whose browser runs no JavaScript.
const epicsIndexHref = "/epics"

// epicChoiceView is one epic as the picker and the full list show it.
type epicChoiceView struct {
	ID      string
	Title   string
	Href    string
	Current bool
	// OptionID is the DOM id the enhanced picker points aria-activedescendant
	// at; it is also what an arrow key moves between.
	OptionID string
}

// epicPickerView is the dropdown the current epic's row carries: every epic,
// in creation order, with the current one marked.
type epicPickerView struct {
	Current   epicChoiceView
	Choices   []epicChoiceView
	IndexHref string
}

// epicChoices names every epic in creation order, marking current.
func epicChoices(document *saga.Saga, current string) []epicChoiceView {
	choices := make([]epicChoiceView, 0, len(document.Epics))
	for _, epic := range document.Epics {
		title := strings.TrimSpace(epic.Title)
		if title == "" {
			title = epic.ID
		}
		choices = append(choices, epicChoiceView{
			ID: epic.ID, Title: title, Href: epicHref(epic.ID),
			Current: epic.ID == current, OptionID: "epic-option-" + domID(epic.ID),
		})
	}
	return choices
}

// makeEpicPicker builds the dropdown for the current epic.
func makeEpicPicker(document *saga.Saga, current string) *epicPickerView {
	choices := epicChoices(document, current)
	if len(choices) == 0 {
		return nil
	}
	picker := &epicPickerView{Choices: choices, IndexHref: epicsIndexHref, Current: choices[0]}
	for _, choice := range choices {
		if choice.Current {
			picker.Current = choice
		}
	}
	return picker
}

// resolveCurrentEpic answers which epic the sidebar shows: the epic of the
// page being read, else the reader's last choice, else the first epic.
func resolveCurrentEpic(document *saga.Saga, pageEpic string, r *http.Request) string {
	if len(document.Epics) == 0 {
		return ""
	}
	if hasEpic(document, pageEpic) {
		return pageEpic
	}
	if chosen, ok := chosenEpic(r); ok && hasEpic(document, chosen) {
		return chosen
	}
	return document.Epics[0].ID
}

// chosenEpic reads the reader's stored choice.
func chosenEpic(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(currentEpicCookie)
	if err != nil {
		return "", false
	}
	value, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", false
	}
	return value, value != ""
}

func hasEpic(document *saga.Saga, id string) bool {
	if id == "" {
		return false
	}
	for _, epic := range document.Epics {
		if epic.ID == id {
			return true
		}
	}
	return false
}

// pageEpic names the epic of the page being read: an epic's own page, a story
// or criterion of one, or a test case of one. A chapter redirects to its
// epic's page before it reaches here, and every slide in the sidebar already
// belongs to the epic the sidebar shows, so both arrive as "epic".
func pageEpic(route appRoute, page *requirementsPageView, tests quality.Document) string {
	switch route.kind {
	case "epic":
		return route.id
	case "requirements":
		if page != nil && page.Story != nil {
			return page.Story.Epic
		}
	case "test":
		for _, testCase := range tests.TestCases {
			if testCase.Identity.ID == route.id {
				return testCase.Epic
			}
		}
	}
	return ""
}

// rememberEpic stores the epic a reader opened, so the sidebar still shows it
// on the app's own pages. It is a reading preference: same-site, session
// scoped, and never read by anything but the sidebar.
func rememberEpic(w http.ResponseWriter, r *http.Request, epic string) {
	if epic == "" {
		return
	}
	if chosen, ok := chosenEpic(r); ok && chosen == epic {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: currentEpicCookie, Value: url.QueryEscape(epic), Path: "/",
		SameSite: http.SameSiteStrictMode, HttpOnly: false,
	})
}

// ----- The epics index -----

// epicsIndexView is every epic, in creation order, with what each one holds.
type epicsIndexView struct {
	Epics []epicIndexRowView
}

type epicIndexRowView struct {
	ID          string
	Title       string
	Href        string
	Description string
	Current     bool
	Stories     int
	TestCases   int
	Slides      int
}

// epicsIndex counts what each epic holds. The counts are facts, never a score:
// an epic with nothing in it is listed like any other.
func epicsIndex(document *saga.Saga, records requirements.Document, tests quality.Document, current string) *epicsIndexView {
	rows := make([]epicIndexRowView, 0, len(document.Epics))
	descriptions := map[string]string{}
	for _, manifest := range records.Epics {
		descriptions[manifest.ID] = manifest.Description
	}
	for _, choice := range epicChoices(document, current) {
		row := epicIndexRowView{
			ID: choice.ID, Title: choice.Title, Href: choice.Href,
			Description: descriptions[choice.ID], Current: choice.Current,
		}
		for _, story := range records.Stories {
			if story.Epic == choice.ID {
				row.Stories++
			}
		}
		for _, testCase := range tests.TestCases {
			if testCase.Epic == choice.ID {
				row.TestCases++
			}
		}
		for _, epic := range document.Epics {
			if epic.ID != choice.ID {
				continue
			}
			for _, deck := range epic.Decks {
				row.Slides += len(deck.Slides)
			}
		}
		rows = append(rows, row)
	}
	return &epicsIndexView{Epics: rows}
}
