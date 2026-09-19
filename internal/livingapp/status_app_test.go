package livingapp

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// appSaga authors, through the domain writers, an app Saga with two epics:
// main holds wallet (serving shopper, gated by wallet-flag); legacy holds
// fax-a and fax-b (serving only faxer, gated by legacy-flag through their
// epic). faxer is retired, and clerk is an active persona nothing serves.
func appSaga(t *testing.T) string {
	t.Helper()
	root := livingFixture(t)
	if _, err := applayout.WriteEpic(root, applayout.EpicManifest{ID: "legacy", Title: "Legacy", CreatedAt: fixtureTime}); err != nil {
		t.Fatal(err)
	}
	persona := func(id string) string {
		t.Helper()
		if _, err := requirements.AddPersona(root, "test", requirements.AddPersonaInput{ID: id, RevisionID: "r1", EventID: "active", Name: id, Description: "The " + id, CreatedAt: fixtureTime, RequestID: "persona-" + id}); err != nil {
			t.Fatal(err)
		}
		urn, _ := requirements.PersonaURN("test", id)
		return urn
	}
	faxer := persona("faxer")
	persona("clerk")
	story := func(epic, id string, personas ...string) {
		t.Helper()
		if _, err := requirements.AddStory(root, "test", requirements.AddStoryInput{Epic: epic, Personas: personas, ID: id, RevisionID: "r1", EventID: "proposed", Title: id, Statement: "Deliver " + id, Priority: "high", Citations: []string{}, AcceptanceCriteria: []requirements.Criterion{{ID: "works", Statement: id + " works"}}, CreatedAt: fixtureTime, RequestID: "story-" + id}); err != nil {
			t.Fatal(err)
		}
		urn, _ := livingid.Story("test", id)
		parent, _ := requirements.StoryEventURN("test", id, "proposed")
		if _, err := requirements.SetStoryState(root, "test", requirements.SetStoryStateInput{Story: urn, ID: "accepted", Parents: []string{parent}, State: requirements.StateAccepted, CreatedAt: fixtureTime.Add(time.Minute), RequestID: "accept-" + id}); err != nil {
			t.Fatal(err)
		}
	}
	story(fixtureEpic, "wallet", fixturePersona())
	story("legacy", "fax-a", faxer)
	story("legacy", "fax-b", faxer)
	parent, _ := requirements.PersonaEventURN("test", "faxer", "active")
	if _, err := requirements.SetPersonaState(root, "test", requirements.SetPersonaStateInput{Persona: faxer, ID: "retired", Parents: []string{parent}, State: requirements.PersonaRetired, CreatedAt: fixtureTime.Add(2 * time.Minute), RequestID: "retire-faxer"}); err != nil {
		t.Fatal(err)
	}
	wallet, _ := livingid.Story("test", "wallet")
	flag := func(id, target string) {
		t.Helper()
		if _, err := requirements.AddFlag(root, "test", requirements.AddFlagInput{ID: id, RevisionID: "r1", EventID: "off", Description: "Gate " + id, Targets: []string{target}, State: requirements.FlagOff, CreatedAt: fixtureTime, RequestID: "flag-" + id}); err != nil {
			t.Fatal(err)
		}
	}
	flag("wallet-flag", wallet)
	flag("legacy-flag", applayout.EpicURN("test", "legacy"))
	return root
}

func loadAppStatus(t *testing.T, root string) Status {
	t.Helper()
	status, err := LoadStatus(context.Background(), StatusOptions{SagaRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func storyRow(t *testing.T, status Status, id string) StoryStatus {
	t.Helper()
	for _, row := range status.Stories {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("story %s missing from status", id)
	return StoryStatus{}
}

func personaRow(t *testing.T, status Status, id string) PersonaStatus {
	t.Helper()
	for _, row := range status.Personas {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("persona %s missing from status", id)
	return PersonaStatus{}
}

func storyURNs(ids ...string) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		urn, _ := livingid.Story("test", id)
		values = append(values, urn)
	}
	return values
}

func flagURN(id string) string {
	urn, _ := requirements.FlagURN("test", id)
	return urn
}

func TestAppStatusGroupsStoriesByEpic(t *testing.T) {
	status := loadAppStatus(t, appSaga(t))
	if len(status.Epics) != 2 {
		t.Fatalf("epics = %+v", status.Epics)
	}
	byID := map[string]EpicStatus{}
	for _, epic := range status.Epics {
		byID[epic.ID] = epic
	}
	if got := byID["legacy"]; got.Epic != applayout.EpicURN("test", "legacy") || !reflect.DeepEqual(got.Stories, storyURNs("fax-a", "fax-b")) {
		t.Fatalf("legacy epic = %+v", got)
	}
	if got := byID["legacy"].GatedBy; !reflect.DeepEqual(got, []string{flagURN("legacy-flag")}) {
		t.Fatalf("an off flag targeting the epic gates it: %v", got)
	}
	if got := byID[fixtureEpic]; !reflect.DeepEqual(got.Stories, storyURNs("wallet")) || len(got.GatedBy) != 0 {
		t.Fatalf("main epic = %+v", got)
	}
	for _, id := range []string{"fax-a", "fax-b"} {
		if row := storyRow(t, status, id); row.Epic != "legacy" {
			t.Fatalf("%s epic = %q", id, row.Epic)
		}
	}
}

func TestAppStatusReportsPersonaGapsAndOneOrphanGroup(t *testing.T) {
	status := loadAppStatus(t, appSaga(t))

	clerk := personaRow(t, status, "clerk")
	if clerk.State != "active" || !clerk.Gap || len(clerk.ServedBy) != 0 {
		t.Fatalf("an active persona no accepted story serves is a gap: %+v", clerk)
	}
	shopper := personaRow(t, status, fixturePersonaID)
	if shopper.Gap || !reflect.DeepEqual(shopper.ServedBy, storyURNs("wallet")) {
		t.Fatalf("shopper is served by wallet: %+v", shopper)
	}
	faxer := personaRow(t, status, "faxer")
	if faxer.State != "retired" || faxer.Gap {
		t.Fatalf("a retired persona is never a gap: %+v", faxer)
	}

	faxerURN, _ := requirements.PersonaURN("test", "faxer")
	want := []PersonaOrphans{{Personas: []string{faxerURN}, Stories: storyURNs("fax-a", "fax-b")}}
	if !reflect.DeepEqual(status.PersonaOrphans, want) {
		t.Fatalf("retiring faxer leaves one orphan group with both stories: %+v", status.PersonaOrphans)
	}

	// Persona coverage is reported, never blocking.
	if status.PersonaCoverage.Blocking {
		t.Fatal("persona coverage must never block")
	}
	clerkURN, _ := requirements.PersonaURN("test", "clerk")
	orphanFacts, clerkServed := 0, false
	for _, fact := range status.PersonaCoverage.Facts {
		switch fact.Code {
		case "retired_persona_stories_decided":
			orphanFacts++
			if fact.Satisfied {
				t.Fatalf("an undecided orphan group is unsatisfied: %+v", fact)
			}
		case "active_persona_served":
			if fact.Resource == clerkURN {
				clerkServed = true
				if fact.Satisfied {
					t.Fatalf("clerk is not served: %+v", fact)
				}
			}
			if fact.Resource == faxerURN {
				t.Fatalf("a retired persona needs no served fact: %+v", fact)
			}
		}
	}
	if orphanFacts != 1 || !clerkServed {
		t.Fatalf("facts: orphan groups=%d clerk fact=%v in %+v", orphanFacts, clerkServed, status.PersonaCoverage.Facts)
	}
}

func TestOffFlagGatesStoriesDirectlyAndThroughTheirEpic(t *testing.T) {
	root := appSaga(t)
	status := loadAppStatus(t, root)
	wallet := storyRow(t, status, "wallet")
	if !reflect.DeepEqual(wallet.GatedBy, []string{flagURN("wallet-flag")}) || wallet.Availability == AvailabilityEnabled {
		t.Fatalf("an off flag targeting the story gates it: %+v", wallet)
	}
	for _, id := range []string{"fax-a", "fax-b"} {
		row := storyRow(t, status, id)
		if !reflect.DeepEqual(row.GatedBy, []string{flagURN("legacy-flag")}) || row.Availability == AvailabilityEnabled {
			t.Fatalf("an off flag targeting the epic gates %s: %+v", id, row)
		}
	}

	parent, _ := requirements.FlagEventURN("test", "wallet-flag", "off")
	if _, err := requirements.SetFlagState(root, "test", requirements.SetFlagStateInput{Flag: flagURN("wallet-flag"), ID: "on", Parents: []string{parent}, State: requirements.FlagOn, CreatedAt: fixtureTime.Add(time.Hour), RequestID: "wallet-on"}); err != nil {
		t.Fatal(err)
	}
	status = loadAppStatus(t, root)
	if wallet = storyRow(t, status, "wallet"); len(wallet.GatedBy) != 0 {
		t.Fatalf("an on flag no longer gates the story: %+v", wallet)
	}
	for _, flag := range status.Flags {
		if flag.ID == "wallet-flag" && (flag.State != "on" || !reflect.DeepEqual(flag.Stories, storyURNs("wallet"))) {
			t.Fatalf("the on flag still names the story it targets: %+v", flag)
		}
	}
	if row := storyRow(t, status, "fax-a"); len(row.GatedBy) != 1 {
		t.Fatalf("turning one flag on leaves the other gating: %+v", row)
	}

	parent, _ = requirements.FlagEventURN("test", "legacy-flag", "off")
	if _, err := requirements.SetFlagState(root, "test", requirements.SetFlagStateInput{Flag: flagURN("legacy-flag"), ID: "on", Parents: []string{parent}, State: requirements.FlagOn, CreatedAt: fixtureTime.Add(time.Hour), RequestID: "legacy-on"}); err != nil {
		t.Fatal(err)
	}
	status = loadAppStatus(t, root)
	for _, id := range []string{"fax-a", "fax-b"} {
		if row := storyRow(t, status, id); len(row.GatedBy) != 0 {
			t.Fatalf("an on flag targeting the epic no longer gates %s: %+v", id, row)
		}
	}
	for _, epic := range status.Epics {
		if len(epic.GatedBy) != 0 {
			t.Fatalf("an epic whose only flag is on is not gated: %+v", epic)
		}
	}
}

// implementedInputs is the quality fixture narrowed to the one criterion whose
// implementation axis is covered by a slide item that owns a diff, so the
// refund story is fully implemented.
func implementedInputs(t *testing.T) StatusInputs {
	t.Helper()
	in := qualityFixture(t)
	stories := []requirements.Story{refundStory("positive-path")}
	guard := requirements.Relation{
		Version: requirements.V5RelationVersion, ID: "guard-addresses-positive", Type: requirements.RelationAddresses,
		From: in.Decks[0].Slides[0].Items[0].Target, To: criterionURN("positive-path"), Scope: requirements.ScopeSelf,
		FromContentDigest: "sha256:" + strings.Repeat("a", 64), ToRevision: storyR2, State: requirements.RelationActive,
	}
	in.Stories = stories
	in.Links = fixtureLinks(stories, []requirements.Relation{verifies("happy-positive", "happy", "r1", "positive-path", storyR2), guard}, in.Quality)
	in.Exceptions = []coverage.Exception{}
	in.Quality = quality.Document{SagaID: fixtureSaga, TestCases: in.Quality.TestCases, Policies: []quality.Policy{}, PolicySets: in.Quality.PolicySets}
	in.Decks = []*saga.Deck{in.Decks[0]}
	return in
}

func TestGatedImplementedStoryIsImplementedNotEnabled(t *testing.T) {
	in := implementedInputs(t)
	status := Assemble(in)
	row := status.Stories[0]
	if cell := cell(t, status, "positive-path", coverage.AxisImplementation); cell.State != coverage.StateCoveredDirect {
		t.Fatalf("fixture must cover implementation: %+v", cell)
	}
	if row.Availability != AvailabilityEnabled || len(row.GatedBy) != 0 {
		t.Fatalf("an implemented, ungated story is enabled: %+v", row)
	}

	flag := "urn:change-saga:checkout:flag:refunds"
	in.Gates = requirements.Gate{Off: map[string][]string{"refund": {flag}}, On: map[string][]string{}}
	row = Assemble(in).Stories[0]
	if row.Availability != AvailabilityImplementedNotEnabled || !reflect.DeepEqual(row.GatedBy, []string{flag}) {
		t.Fatalf("an implemented story gated by an off flag is implemented_not_enabled: %+v", row)
	}

	in.Gates = requirements.Gate{Off: map[string][]string{}, On: map[string][]string{"refund": {flag}}}
	row = Assemble(in).Stories[0]
	if row.Availability != AvailabilityEnabled || len(row.GatedBy) != 0 {
		t.Fatalf("an on flag enables the implemented story: %+v", row)
	}
}
