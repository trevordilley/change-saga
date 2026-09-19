package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestClaimsAndVerificationsAreIndependentAppendOnlyRecords(t *testing.T) {
	root := newAuthoredSaga(t)
	repo, commit := sourceRepo(t, map[string]string{"worker.go": "package worker\n\n// one\n// two\n// three\n// four\n// five\n"})
	location := commit + ":worker.go#L3-L7"
	var output bytes.Buffer
	if err := AddClaim(context.Background(), []string{
		"--id", "single-flight", "--target", "___overview/overview.fragment", "--kind", "invariant",
		"--statement", "Only one sampler can run at a time.", "--repo", repo, "--ref", location, root,
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaim(context.Background(), []string{
		"--id", "single-flight-check", "--claim", "single-flight", "--status", "verified",
		"--method", "test", "--summary", "The concurrency test passed.", "--command", "go test ./...", root,
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaim(context.Background(), []string{
		"--id", "single-flight-recheck", "--claim", "single-flight", "--status", "inconclusive",
		"--method", "analysis", "--summary", "Exception cleanup still needs inspection.", root,
	}, &output); err != nil {
		t.Fatal(err)
	}

	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: validation=%#v err=%v", validation, err)
	}
	if len(document.Claims) != 1 || len(document.Verifications) != 2 {
		t.Fatalf("records were consolidated: claims=%#v verifications=%#v", document.Claims, document.Verifications)
	}
	claim := document.Claims[0]
	if claim.Target != saga.FragmentTarget("atomic", "atomic-overview") || claim.Statement != "Only one sampler can run at a time." || len(claim.Evidence) != 1 {
		t.Fatalf("claim = %#v", claim)
	}
	// The claim pins the exact lines with a digest read from the repository.
	want := sha256Digest("// one\n// two\n// three\n// four\n// five\n")
	if evidence := claim.Evidence[0]; evidence.Location().String() != location || evidence.Digest != want {
		t.Fatalf("claim evidence = %#v, want %s with digest %s", evidence, location, want)
	}
	for _, path := range []string{
		filepath.Join(root, "___claims", "single-flight.json"),
		filepath.Join(root, "___verifications", "single-flight-check.json"),
		filepath.Join(root, "___verifications", "single-flight-recheck.json"),
	} {
		if info, statErr := os.Stat(path); statErr != nil || !info.Mode().IsRegular() {
			t.Fatalf("missing independent record %s: %v", path, statErr)
		}
	}
}

func TestClaimFailuresDoNotWriteRecords(t *testing.T) {
	root := newAuthoredSaga(t)
	repo, commit := sourceRepo(t, map[string]string{"worker.go": "package worker\n"})
	var output bytes.Buffer
	for _, test := range []struct {
		name string
		refs []string
		want string
	}{
		{"not a location", []string{"not-a-location"}, "invalid --ref 1"},
		{"abbreviated commit", []string{commit[:12] + ":worker.go"}, "invalid --ref 1"},
		{"missing file", []string{commit + ":missing.go"}, "does not exist"},
		{"lines past the end", []string{commit + ":worker.go#L1-L9"}, "invalid --ref 1"},
		{"duplicate location", []string{commit + ":worker.go", commit + ":worker.go"}, "--ref 2 duplicates"},
	} {
		args := []string{"--id", "bad", "--target", "___overview/overview.fragment", "--kind", "behavior", "--statement", "This should not be written.", "--repo", repo}
		for _, ref := range test.refs {
			args = append(args, "--ref", ref)
		}
		err := AddClaim(context.Background(), append(args, root), &output)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: invalid evidence error = %v, want %q", test.name, err, test.want)
		}
		if entries, readErr := os.ReadDir(filepath.Join(root, "___claims")); readErr != nil || len(entries) != 0 {
			t.Fatalf("%s: failed claim wrote records: entries=%v err=%v", test.name, entries, readErr)
		}
	}
	err := VerifyClaim(context.Background(), []string{
		"--id", "bad-check", "--claim", "missing", "--status", "verified", "--method", "test",
		"--summary", "No claim exists.", root,
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing claim error = %v", err)
	}
	if entries, readErr := os.ReadDir(filepath.Join(root, "___verifications")); readErr != nil || len(entries) != 0 {
		t.Fatalf("failed verification wrote records: entries=%v err=%v", entries, readErr)
	}
}
