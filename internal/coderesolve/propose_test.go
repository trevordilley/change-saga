package coderesolve

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

func TestProposeRange(t *testing.T) {
	cases := []struct {
		name         string
		hunks        []gitdiff.Hunk
		start, end   int
		wantStart    int
		wantEnd      int
		ok, widened  bool
		reasonSubstr string
	}{
		{"no hunks", nil, 5, 9, 5, 9, true, false, "only moved"},
		{"insert before", []gitdiff.Hunk{{OldStart: 2, OldCount: 0, NewStart: 3, NewCount: 3}}, 5, 9, 8, 12, true, false, "only moved"},
		{"insert inside widens", []gitdiff.Hunk{{OldStart: 6, OldCount: 0, NewStart: 7, NewCount: 2}}, 5, 9, 5, 11, true, false, "inserted"},
		{"insert inside after a shift", []gitdiff.Hunk{{OldStart: 1, OldCount: 0, NewStart: 2, NewCount: 1}, {OldStart: 6, OldCount: 0, NewStart: 8, NewCount: 1}}, 5, 9, 6, 11, true, false, "inserted"},
		{"insert just after is outside", []gitdiff.Hunk{{OldStart: 6, OldCount: 0, NewStart: 7, NewCount: 1}, {OldStart: 9, OldCount: 0, NewStart: 11, NewCount: 2}}, 5, 9, 5, 10, true, false, ""},
		{"change first line", []gitdiff.Hunk{{OldStart: 5, OldCount: 1, NewStart: 5, NewCount: 2}}, 5, 9, 5, 10, true, false, "inserted"},
		{"change last line", []gitdiff.Hunk{{OldStart: 9, OldCount: 1, NewStart: 9, NewCount: 3}}, 5, 9, 5, 11, true, false, ""},
		{"delete inside", []gitdiff.Hunk{{OldStart: 6, OldCount: 2, NewStart: 5, NewCount: 0}}, 5, 9, 5, 7, true, false, ""},
		{"delete first lines", []gitdiff.Hunk{{OldStart: 5, OldCount: 2, NewStart: 4, NewCount: 0}}, 5, 9, 5, 7, true, false, ""},
		{"straddles start", []gitdiff.Hunk{{OldStart: 3, OldCount: 3, NewStart: 3, NewCount: 4}}, 5, 9, 3, 10, true, true, "edge"},
		{"straddles end", []gitdiff.Hunk{{OldStart: 8, OldCount: 4, NewStart: 8, NewCount: 1}}, 5, 9, 5, 8, true, true, "edge"},
		{"replaced exactly", []gitdiff.Hunk{{OldStart: 5, OldCount: 5, NewStart: 5, NewCount: 2}}, 5, 9, 5, 6, true, false, "replaced"},
		{"one line edited", []gitdiff.Hunk{{OldStart: 7, OldCount: 1, NewStart: 8, NewCount: 1}}, 7, 7, 8, 8, true, false, "replaced"},
		{"all removed", []gitdiff.Hunk{{OldStart: 5, OldCount: 5, NewStart: 4, NewCount: 0}}, 5, 9, 0, 0, false, false, "removed"},
		{"rewritten with outside code", []gitdiff.Hunk{{OldStart: 3, OldCount: 10, NewStart: 3, NewCount: 12}}, 5, 9, 0, 0, false, false, "no line anchors"},
	}
	for _, test := range cases {
		got := ProposeRange(test.hunks, test.start, test.end)
		if got.OK != test.ok || got.OK && (got.Start != test.wantStart || got.End != test.wantEnd || got.Widened != test.widened) {
			t.Errorf("%s: got %d-%d ok=%v widened=%v (%s), want %d-%d ok=%v widened=%v", test.name, got.Start, got.End, got.OK, got.Widened, got.Reason, test.wantStart, test.wantEnd, test.ok, test.widened)
		}
		if !strings.Contains(got.Reason, test.reasonSubstr) {
			t.Errorf("%s: reason %q does not mention %q", test.name, got.Reason, test.reasonSubstr)
		}
	}
}

func TestProposeFollowsAnEditInsideTheRange(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "app.go", "package app\n\ntype T struct {\n\tA int\n\tB int\n}\n\nfunc F() {}\n")
	pin := commit(t, repo, "pin")
	write(t, repo, "app.go", "package app\n\n// T is a thing.\ntype T struct {\n\tA int\n\tNew string\n\tB int\n}\n\nfunc F() {}\n")
	view := commit(t, repo, "edit")
	git(t, repo, "rm", "-q", "app.go")
	gone := commit(t, repo, "delete")

	resolver, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "app.go", Start: 3, End: 6}, "the struct")
	if err != nil {
		t.Fatal(err)
	}
	if resolver.Resolve(ctx, reference, view).Current() {
		t.Fatal("an edit inside the range must leave it stale")
	}
	proposal := resolver.Propose(ctx, reference, view)
	if !proposal.Proposed() || proposal.Location.String() != view+":app.go#L4-L8" {
		t.Fatalf("proposal = %+v", proposal)
	}
	if strings.Join(proposal.Diff, "\n") != "@@ -4,0 +6,1 @@\n+\tNew string" {
		t.Fatalf("diff = %q", proposal.Diff)
	}
	if got := resolver.Propose(ctx, reference, gone); got.Proposed() || !strings.Contains(got.Reason, "deleted") {
		t.Fatalf("deleted file proposal = %+v", got)
	}
	whole, err := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "app.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Propose(ctx, whole, view); !got.Proposed() || got.Location.String() != view+":app.go" {
		t.Fatalf("whole-file proposal = %+v", got)
	}
}

func TestRemovedNamesOnlyLinesTheChangeTookOut(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "app.go", "a\nb\nc\nd\ne\n")
	base := commit(t, repo, "base")
	write(t, repo, "app.go", "a\nB\nC\nd\ne\n")
	head := commit(t, repo, "edit")
	resolver, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	for _, test := range []struct {
		start, end int
		want       bool
	}{{2, 3, true}, {3, 3, true}, {2, 4, false}, {1, 1, false}, {0, 0, false}} {
		if got := resolver.Removed(ctx, base, head, "app.go", test.start, test.end); got != test.want {
			t.Errorf("Removed(%d-%d) = %v, want %v", test.start, test.end, got, test.want)
		}
	}
}
