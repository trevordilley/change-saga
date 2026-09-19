package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
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

// addTestApp adds the fixture epic, persona, and overview description to an
// initialized Saga and returns the persona URN.
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
	overviewFragment(root)
	return personaURNFor(manifest.ID)
}

// testEpicDir returns the absolute directory of the fixture epic.
func testEpicDir(root string) string { return applayout.EpicDir(root, testEpic) }

// overviewFragment returns the overview's description fragment, writing a
// placeholder description the first time so tests have an app-level fragment
// to author into and cover. Init writes no overview part.
func overviewFragment(root string) string {
	dir := filepath.Join(root, applayout.OverviewDir, applayout.OverviewDescription)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		var output bytes.Buffer
		if err := Overview(context.Background(), []string{"set-description", "--text", "The app.", root}, &output); err != nil {
			panic(fmt.Sprintf("write the overview description: %v\n%s", err, output.String()))
		}
	}
	return dir
}
