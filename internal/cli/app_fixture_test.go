package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Every fixture app Saga gets one epic and one persona so tests can author
// epic content and stories the way an author would after init.
const (
	testEpic    = "core"
	testPersona = "user"
	// testPersonaURN is the fixture persona of the "atomic" Saga most tests use.
	testPersonaURN = "urn:change-saga:atomic:persona:" + testPersona
)

// testEpicRel is the app-relative directory of the fixture epic.
var testEpicRel = applayout.EpicRel(testEpic)

// personaURNFor returns the fixture persona URN of a Saga with sagaID.
func personaURNFor(sagaID string) string {
	return "urn:change-saga:" + sagaID + ":persona:" + testPersona
}

// addTestApp adds the fixture epic and persona to an initialized Saga and
// returns the persona URN.
func addTestApp(t *testing.T, root string) string {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Epic(context.Background(), []string{"add", "--id", testEpic, "--title", "Core", root}, &output); err != nil {
		t.Fatalf("epic add: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Persona(context.Background(), []string{"add", "--id", testPersona, "--name", "User", "--description", "Someone who uses the app", root}, &output); err != nil {
		t.Fatalf("persona add: %v\n%s", err, output.String())
	}
	return personaURNFor(manifest.ID)
}

// testEpicDir returns the absolute directory of the fixture epic.
func testEpicDir(root string) string { return applayout.EpicDir(root, testEpic) }

// overviewFragment is the path of the app overview init writes.
func overviewFragment(root string) string {
	return filepath.Join(root, applayout.OverviewDir, "overview.fragment")
}
