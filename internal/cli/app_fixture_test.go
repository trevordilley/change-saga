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

// Every fixture app Saga gets one feature and one persona so tests can author
// feature content and stories the way an author would after init.
const (
	testFeature = "core"
	testPersona = "user"
	// testPersonaURN is the fixture persona of the "atomic" Saga most tests use.
	testPersonaURN = "urn:change-saga:atomic:persona:" + testPersona
)

// testFeatureRel is the app-relative directory of the fixture feature.
var testFeatureRel = applayout.FeatureRel(testFeature)

// personaURNFor returns the fixture persona URN of a Saga with sagaID.
func personaURNFor(sagaID string) string {
	return "urn:change-saga:" + sagaID + ":persona:" + testPersona
}

// addTestApp adds the fixture feature, persona, and overview description to an
// initialized Saga and returns the persona URN.
func addTestApp(t *testing.T, root string) string {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Feature(context.Background(), []string{"add", "--id", testFeature, "--title", "Core", root}, &output); err != nil {
		t.Fatalf("feature add: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Persona(context.Background(), []string{"add", "--id", testPersona, "--name", "User", "--description", "Someone who uses the app", root}, &output); err != nil {
		t.Fatalf("persona add: %v\n%s", err, output.String())
	}
	overviewFragment(root)
	return personaURNFor(manifest.ID)
}

// testFeatureDir returns the absolute directory of the fixture feature.
func testFeatureDir(root string) string { return applayout.FeatureDir(root, testFeature) }

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

// shortTempDir is t.TempDir() with a short leaf name. The portable deck path
// budget (saga.FlatMaxPath) is measured on the absolute path, and a platform
// temp prefix plus a long test name can spend most of it before the Saga's own
// ___features/<id>.feature/___slides/<id>.deck directories are counted.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
