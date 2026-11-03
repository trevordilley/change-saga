package coderesolve

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

func TestPreparedResolverPreservesFindingsAndHandlesExpandedOrUnplannedPaths(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "first.go", "one\ntwo\nthree\n")
	write(t, repo, "second.go", "one\ntwo\nthree\n")
	pin := commit(t, repo, "pin")
	ctx, end := gitexec.Begin(context.Background())
	defer end()
	full, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer full.Close()
	first, err := full.Author(ctx, coderef.Location{Commit: pin, Path: "first.go", Start: 2, End: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := full.Author(ctx, coderef.Location{Commit: pin, Path: "second.go", Start: 2, End: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	write(t, repo, "first.go", "inserted\none\ntwo\nthree\n")
	write(t, repo, "second.go", "one\nTWO\nthree\n")
	view := commit(t, repo, "shift one reference and invalidate the other")
	for _, expand := range []bool{false, true} {
		planned, err := New(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		planned.PrepareReferences([]coderef.Reference{first}, view)
		if got, want := planned.Resolve(ctx, first, view), full.Resolve(ctx, first, view); !reflect.DeepEqual(got, want) || !got.Current() || !got.Moved {
			t.Fatalf("prepared remapping: %+v, want %+v", got, want)
		}
		if expand {
			planned.PrepareReferences([]coderef.Reference{second}, view)
		}
		if got, want := planned.Resolve(ctx, second, view), full.Resolve(ctx, second, view); !reflect.DeepEqual(got, want) || got.Current() {
			t.Fatalf("expanded=%v: second path: %+v, want %+v", expand, got, want)
		}
		planned.Close()
	}
	// Optimization failures preserve the complete resolver's provisional
	// diagnostics, rather than inventing a different settled stale result.
	missingView := strings.Repeat("e", 40)
	planned, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer planned.Close()
	planned.PrepareReferences([]coderef.Reference{first}, missingView)
	if got, want := planned.Resolve(ctx, first, missingView), full.Resolve(ctx, first, missingView); !reflect.DeepEqual(got, want) || !got.Provisional {
		t.Fatalf("missing viewed commit: %+v, want %+v", got, want)
	}
}

func TestWarmReferencesPreservesSerialResultsAcrossManyPinsAndCancellation(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	var pins []string
	for index := range 8 {
		write(t, repo, "app.go", strings.Repeat("prefix\n", index+1)+"keep\nend\n")
		pins = append(pins, commit(t, repo, "pin"))
	}
	write(t, repo, "app.go", strings.Repeat("prefix\n", 10)+"keep\nend\n")
	view := commit(t, repo, "shift")
	ctx, end := gitexec.Begin(context.Background())
	defer end()
	serial, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer serial.Close()
	var references []coderef.Reference
	for index, pin := range pins {
		reference, err := serial.Author(ctx, coderef.Location{Commit: pin, Path: "app.go", Start: index + 2, End: index + 2}, "")
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
	}
	missing := references[0]
	missing.Commit = strings.Repeat("f", 40)
	tampered := references[0]
	tampered.Digest = coderef.DigestBytes([]byte("wrong"))
	references = append(references, missing, tampered)
	parallel, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer parallel.Close()
	parallel.PrepareReferences(references, view)
	parallel.WarmReferences(ctx)
	if len(parallel.selected) != len(pins) {
		t.Fatalf("warmed %d comparisons, want %d available pins", len(parallel.selected), len(pins))
	}
	for _, reference := range references {
		if got, want := parallel.Resolve(ctx, reference, view), serial.Resolve(ctx, reference, view); !reflect.DeepEqual(got, want) {
			t.Fatalf("parallel result %+v, serial %+v", got, want)
		}
	}
	cancelled, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelled.Close()
	cancelled.PrepareReferences(references, view)
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	cancelled.WarmReferences(stopped)
	if len(cancelled.selected) != 0 {
		t.Fatal("cancelled warming retained a failed comparison")
	}
	if got, want := cancelled.Resolve(ctx, references[0], view), serial.Resolve(ctx, references[0], view); !reflect.DeepEqual(got, want) {
		t.Fatalf("cancelled warming poisoned the next resolution: %+v, want %+v", got, want)
	}
}
