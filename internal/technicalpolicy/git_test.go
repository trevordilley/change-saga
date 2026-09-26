package technicalpolicy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
)

// All source and Git mutations in this fixture are in t.TempDir. The policy
// sees only the read-only resolver, never the Saga or a publication callback.
func TestTemporaryGitDelivery(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Policy Test", "GIT_AUTHOR_EMAIL=policy@example.test", "GIT_COMMITTER_NAME=Policy Test", "GIT_COMMITTER_EMAIL=policy@example.test")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() string {
		t.Helper()
		git("add", "-A")
		git("-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
		return git("rev-parse", "HEAD")
	}
	git("init", "-q")
	originalContent := "before\nentity evidence\nedge evidence\nafter\n"
	write(originalContent)
	originalOID := commit()
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	var _ EvidenceResolver = resolver
	author := func(start, end int) coderef.Reference {
		t.Helper()
		r, e := resolver.Author(ctx, coderef.Location{Commit: originalOID, Path: "source.txt", Start: start, End: end}, "Exact fixture behavior")
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	own := author(2, 2)
	relationship := author(3, 3)
	span := author(2, 3)
	write("inserted\n" + originalContent)
	movedOID := commit()
	write("inserted\nbefore\nentity evidence\nchanged edge\nafter\n")
	changedOID := commit()
	write("before\nentity evidence\ninside insertion\nedge evidence\nafter\n")
	insideOID := commit()
	write("before\nentity evidence\nafter\n")
	deletedLinesOID := commit()
	if err := os.Remove(filepath.Join(repo, "source.txt")); err != nil {
		t.Fatal(err)
	}
	deletedFileOID := commit()
	makeCandidate := func(oid string) Candidate {
		return Candidate{Pin: Pin{"entity", "r2"}, Intent: Implemented, DeliveryOID: oid, Evidence: []coderef.Reference{own}, Edges: []Edge{{ID: "retained", Intent: Implemented, Destination: Endpoint{Pin{"destination", "r1"}, true, Implemented}, Evidence: []coderef.Reference{relationship}}, {ID: "future", Intent: Proposed}}}
	}
	tests := []struct {
		name  string
		c     Candidate
		codes []Code
	}{
		{"author equals delivery", makeCandidate(originalOID), nil},
		{"pure movement", makeCandidate(movedOID), nil},
		{"retained edge changed", makeCandidate(changedOID), []Code{StaleDelivery}},
		{"edge lines deleted", makeCandidate(deletedLinesOID), []Code{StaleDelivery}},
		{"file deleted", makeCandidate(deletedFileOID), []Code{StaleDelivery, StaleDelivery}},
	}
	inside := makeCandidate(insideOID)
	inside.Evidence = []coderef.Reference{span}
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"inside insertion", inside, []Code{StaleDelivery}})
	// The delivery resolver deliberately supports recovery from unavailable pins;
	// a new implementation assertion must reject that recovery as original proof.
	missing := makeCandidate(movedOID)
	missing.Evidence[0].Commit = strings.Repeat("f", 40)
	if resolved := resolver.Resolve(ctx, missing.Evidence[0], movedOID); !resolved.Current() {
		t.Fatalf("fixture must exercise digest recovery: %+v", resolved)
	}
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"original absent but delivery matches", missing, []Code{InvalidOriginal}})
	invalid := makeCandidate(movedOID)
	invalid.Evidence[0].Start = 1
	invalid.Evidence[0].End = 1
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"wrong original range but delivery matches elsewhere", invalid, []Code{InvalidOriginal}})
	unavailable := makeCandidate(strings.Repeat("e", 40))
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"delivery unavailable", unavailable, []Code{DeliveryUnavailable}})
	// A tree has a full OID but is not a delivery commit.
	notCommit := makeCandidate(git("rev-parse", originalOID+"^{tree}"))
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"delivery tree", notCommit, []Code{DeliveryUnavailable}})

	originalTree := makeCandidate(movedOID)
	originalTree.Evidence[0].Commit = git("rev-parse", originalOID+"^{tree}")
	tests = append(tests, struct {
		name  string
		c     Candidate
		codes []Code
	}{"original tree is not a commit", originalTree, []Code{InvalidOriginal}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := clone(tt.c)
			head := git("rev-parse", "HEAD")
			status := git("status", "--porcelain")
			objects := git("count-objects", "-v")
			got := ValidateCandidate(ctx, tt.c, resolver)
			var codes []Code
			for _, d := range got.Diagnostics {
				codes = append(codes, d.Code)
			}
			if !reflect.DeepEqual(codes, tt.codes) || !reflect.DeepEqual(got.ProposedEdges, []string{"future"}) {
				t.Fatalf("got %+v want %v", got, tt.codes)
			}
			if !reflect.DeepEqual(before, tt.c) {
				t.Fatal("mutated original evidence or intent")
			}
			if head != git("rev-parse", "HEAD") || status != git("status", "--porcelain") || objects != git("count-objects", "-v") {
				t.Fatal("policy wrote source/Git state")
			}
		})
	}
	moved := resolver.Resolve(ctx, own, movedOID)
	if !moved.Current() || !moved.Moved || moved.Location.Start != 3 {
		t.Fatalf("expected movement: %+v", moved)
	}
	// Drift does not demote the original candidate or its implemented edge.
	mixed := makeCandidate(changedOID)
	mixed.Edges[0].Intent = Proposed
	mixed.Edges[0].Evidence = nil
	if got := ValidateCandidate(ctx, mixed, resolver); !got.Valid() || !reflect.DeepEqual(got.ProposedEdges, []string{"retained", "future"}) {
		t.Fatalf("proposed edge must remain work: %+v", got)
	}
}
