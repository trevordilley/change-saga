package requirements

import (
	"testing"
	"time"
)

func TestFeatureFlagStatesAndRetirementHistory(t *testing.T) {
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	flag := "urn:change-saga:test:flag:new-checkout"
	if _, err := AddFlag(root, "test", AddFlagInput{
		ID: "new-checkout", RevisionID: "r1", EventID: "state-off", State: FlagOff,
		Description: "New checkout", Targets: []string{"urn:change-saga:test:story:checkout"}, CreatedAt: testTime,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetFlagState(root, "test", SetFlagStateInput{
		Flag: flag, ID: "state-on", Parents: []string{flag + ":event:state-off"}, State: FlagOn, CreatedAt: testTime.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetFlagState(root, "test", SetFlagStateInput{
		Flag: flag, ID: "state-retired", Parents: []string{flag + ":event:state-on"}, State: FlagRetired,
		Reason: "The rollout completed.", CreatedAt: testTime.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	loaded := document.FindFlag("new-checkout")
	if loaded == nil || loaded.CurrentLifecycle == nil || loaded.CurrentLifecycle.State != FlagRetired {
		t.Fatalf("retired flag = %#v", loaded)
	}
	states := map[FlagState]bool{}
	for _, event := range loaded.Events {
		states[event.State] = true
	}
	if len(loaded.Events) != 3 || !states[FlagOff] || !states[FlagOn] || !states[FlagRetired] {
		t.Fatalf("flag history = %#v", loaded.Events)
	}
	if gates := document.Gates(); len(gates.Off["checkout"]) != 0 || len(gates.On["checkout"]) != 0 {
		t.Fatalf("retired flag still gates checkout: %#v", gates)
	}
}

func TestStoryLifecycleStatesAndRetirementHistory(t *testing.T) {
	root := newSaga(t)
	criteria := []Criterion{{ID: "works", Statement: "The behavior works."}}
	add := func(id string) string {
		t.Helper()
		if _, err := AddStory(root, "test", storyInput(id, "r1", "proposed", criteria)); err != nil {
			t.Fatal(err)
		}
		return "urn:change-saga:test:story:" + id
	}
	set := func(story, id, parent string, state LifecycleState) {
		t.Helper()
		if _, err := SetStoryState(root, "test", SetStoryStateInput{
			Story: story, ID: id, Parents: []string{parent}, State: state,
			Reason: "Exercise the lifecycle state.", CreatedAt: testTime.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}

	add("proposed-story")
	accepted := add("accepted-story")
	deferred := add("deferred-story")
	rejected := add("rejected-story")
	retired := add("retired-story")
	set(accepted, "accepted", accepted+":event:proposed", StateAccepted)
	set(deferred, "deferred", deferred+":event:proposed", StateDeferred)
	set(rejected, "rejected", rejected+":event:proposed", StateRejected)
	set(retired, "accepted", retired+":event:proposed", StateAccepted)
	set(retired, "retired", retired+":event:accepted", StateRetired)

	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]LifecycleState{
		"proposed-story": StateProposed,
		"accepted-story": StateAccepted,
		"deferred-story": StateDeferred,
		"rejected-story": StateRejected,
		"retired-story":  StateRetired,
	}
	for id, state := range want {
		story := document.FindStory(id)
		if story == nil || story.CurrentLifecycle == nil || story.CurrentLifecycle.State != state {
			t.Errorf("%s lifecycle = %#v, want %s", id, story, state)
		}
	}
	retiredStory := document.FindStory("retired-story")
	history := map[LifecycleState]bool{}
	for _, event := range retiredStory.Events {
		history[event.State] = true
	}
	if len(retiredStory.Events) != 3 || !history[StateProposed] || !history[StateAccepted] || !history[StateRetired] || retiredStory.CurrentRevision == nil {
		t.Fatalf("retired story history = %#v", retiredStory)
	}
}
