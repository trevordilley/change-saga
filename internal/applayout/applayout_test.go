package applayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEpicsListValidatesAndResolves(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if epics, err := Epics(root); err != nil || len(epics) != 0 {
		t.Fatalf("app without epics = %v, %v", epics, err)
	}
	for _, id := range []string{"checkout", "catalog"} {
		if _, err := WriteEpic(root, EpicManifest{ID: id, Title: strings.ToUpper(id), CreatedAt: time.Unix(0, 0).UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := WriteEpic(root, EpicManifest{ID: "checkout", Title: "Again"}); err == nil {
		t.Fatal("a second epic with one id was written")
	}
	epics, err := Epics(root)
	if err != nil || len(epics) != 2 || epics[0].ID != "catalog" || epics[1].Rel != "___epics/checkout.epic" {
		t.Fatalf("epics = %+v, %v", epics, err)
	}
	if epic, err := Require(root, "app", "urn:change-saga:app:epic:catalog"); err != nil || epic.ID != "catalog" {
		t.Fatalf("require by URN = %+v, %v", epic, err)
	}
	if _, err := Require(root, "app", ""); err == nil || !strings.Contains(err.Error(), "known epics: catalog, checkout") {
		t.Fatalf("require without an epic = %v", err)
	}
	if got := EpicOfPath("___epics/checkout.epic/___requirements/stories/pay.story"); got != "checkout" {
		t.Fatalf("EpicOfPath = %q", got)
	}
	if got := EpicOfPath("___overview/pitch.fragment"); got != "" {
		t.Fatalf("app-level path belongs to epic %q", got)
	}
	if err := os.Mkdir(filepath.Join(root, EpicsDir, "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Epics(root); err == nil {
		t.Fatal("a directory that is not <id>.epic was accepted")
	}
}

func TestEpicRootsAtTheAppRootAreRefused(t *testing.T) {
	root := t.TempDir()
	if err := RejectEpicRootsAtAppRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, RequirementsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RejectEpicRootsAtAppRoot(root); err == nil {
		t.Fatal("a root-level ___requirements was accepted")
	}
}

func TestUniqueIDsNameBothEpics(t *testing.T) {
	ids := NewUniqueIDs("story")
	if err := ids.Claim("pay", "checkout"); err != nil {
		t.Fatal(err)
	}
	if err := ids.Claim("pay", "catalog"); err == nil || !strings.Contains(err.Error(), `"checkout" and "catalog"`) {
		t.Fatalf("duplicate across epics = %v", err)
	}
}

// Epics are presented in the order they were created, not alphabetically;
// epics created at the same instant keep ID order.
func TestInCreationOrderPresentsEpicsAsTheyWereIntroduced(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 9, 19, 20, 47, second, 0, time.UTC) }
	epics := []Epic{
		{EpicManifest: EpicManifest{ID: "agent-loop", CreatedAt: at(9)}},
		{EpicManifest: EpicManifest{ID: "format", CreatedAt: at(1)}},
		{EpicManifest: EpicManifest{ID: "comparison", CreatedAt: at(5)}},
		{EpicManifest: EpicManifest{ID: "code-evidence", CreatedAt: at(5)}},
	}
	got := []string{}
	for _, epic := range InCreationOrder(epics) {
		got = append(got, epic.ID)
	}
	if strings.Join(got, ",") != "format,code-evidence,comparison,agent-loop" {
		t.Fatalf("order = %v", got)
	}
	if epics[0].ID != "agent-loop" {
		t.Fatal("InCreationOrder must not reorder its argument")
	}
}
