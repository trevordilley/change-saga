package inventoryview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

const ns = "urn:change-saga:test:"

func pin(kind, id, rev string) Pin {
	return Pin{Target: ns + kind + ":" + id, Revision: ns + kind + ":" + id + ":revision:" + rev}
}

// record builds a loaded technical record in memory. revisions are ordered;
// the last one is current unless conflicted is set.
func record(kind, id string, revisions []requirements.TechnicalRevision, state string, conflicted bool) requirements.TechnicalRecord {
	target := ns + kind + ":" + id
	r := requirements.TechnicalRecord{Kind: kind, Target: target, Identity: requirements.RecordIdentity{ID: id}, Revisions: revisions}
	for _, v := range revisions {
		r.RevisionHeads = []string{target + ":revision:" + v.ID}
	}
	if conflicted && len(revisions) > 1 {
		r.RevisionHeads = []string{target + ":revision:" + revisions[len(revisions)-2].ID, target + ":revision:" + revisions[len(revisions)-1].ID}
	} else if len(revisions) > 0 {
		r.CurrentRevision = &r.Revisions[len(r.Revisions)-1]
	}
	r.Events = []requirements.TechnicalEvent{{ID: state, State: state}}
	r.CurrentLifecycle = &r.Events[0]
	return r
}

func revision(id string, code []coderef.Reference, members ...Pin) requirements.TechnicalRevision {
	return requirements.TechnicalRevision{ID: id, TechnicalDefinition: requirements.TechnicalDefinition{Name: id, Explanation: "x", Code: code, Components: members}}
}

func item(featureDeck, slide, id string, doc *Pin) *saga.Item {
	it := &saga.Item{Target: ns + "feature:" + featureDeck + ":slide:" + slide + ":item:" + id}
	it.ID, it.Label, it.Documentation = id, "Label "+id, doc
	return it
}

func document(features map[string][]*saga.Item, reviews map[string][]*saga.Item) *saga.Saga {
	d := &saga.Saga{}
	for id, items := range features {
		deck := &saga.Deck{Target: ns + "feature:" + id + ":deck:impl", Slides: []*saga.Slide{{Target: ns + "feature:" + id + ":slide:s", Items: items}}}
		d.Features = append(d.Features, &saga.Feature{ID: id, Decks: []*saga.Deck{deck}})
	}
	for id, items := range reviews {
		deck := &saga.Deck{Target: ns + "review:" + id + ":deck:d", Slides: []*saga.Slide{{Target: ns + "review:" + id + ":slide:s", Items: items}}}
		d.Reviews = append(d.Reviews, &saga.Review{ReviewManifest: saga.ReviewManifest{ID: id}, Deck: deck})
	}
	return d
}

type repo struct {
	dir     string
	commits []string
}

func run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t testing.TB) *repo {
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	return &repo{dir: dir}
}

func (r *repo) commit(t testing.TB, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(r.dir, name)
		if content == "" {
			run(t, r.dir, "rm", "-q", name)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, r.dir, "add", name)
	}
	run(t, r.dir, "commit", "-q", "--allow-empty", "-m", "c")
	oid := run(t, r.dir, "rev-parse", "HEAD")
	r.commits = append(r.commits, oid)
	return oid
}

func (r *repo) resolver(t testing.TB) *coderesolve.Resolver {
	t.Helper()
	res, err := coderesolve.New(context.Background(), r.dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(res.Close)
	return res
}

func (r *repo) ref(t testing.TB, res *coderesolve.Resolver, commit, path string, start, end int) coderef.Reference {
	t.Helper()
	ref, err := res.Author(context.Background(), coderef.Location{Commit: commit, Path: path, Start: start, End: end}, "exact evidence")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func lines(n int, prefix string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s line %d\n", prefix, i)
	}
	return b.String()
}
