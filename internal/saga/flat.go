package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	FlatMaxBasename    = 64
	FlatMaxPath        = 240
	EmbeddedSlidesDir  = "___slides"
	EmbeddedDeckSuffix = ".deck"
	flatMaxRank        = 9999
)

// Deck bundle filenames are a compact, deterministic storage index. Human meaning
// stays in the JSON and target URNs; filenames carry only category, parent,
// ordering, and collision-resistant identity hints.
var (
	flatDeckName        = regexp.MustCompile(`^10-d-([0-9]{4})-([0-9a-f]{12})\.json$`)
	flatSlideName       = regexp.MustCompile(`^20-s-([0-9a-f]{12})-([0-9]{4})-([0-9a-f]{12})\.json$`)
	flatItemName        = regexp.MustCompile(`^30-i-([0-9a-f]{12})-([0-9]{4})-([0-9a-f]{12})\.json$`)
	flatEvidenceName    = regexp.MustCompile(`^40-e-([0-9a-f]{12})-([0-9a-f]{12})\.json$`)
	flatThreadEventName = regexp.MustCompile(`^83-x-([0-9a-f]{12})-([0-9a-f]{12})\.json$`)
	flatReviewName      = regexp.MustCompile(`^84-r-([0-9a-f]{12})-([0-9a-f]{12})\.json$`)
	flatDiffReviewName  = regexp.MustCompile(`^85-f-([0-9a-f]{12})\.json$`)
)

func FlatKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:6])
}

func FlatTargetKey(target string) string { return FlatKey("target\x00" + target) }

func FlatDeckFilename(target string, rank int) (string, error) {
	if err := validFlatRank(rank); err != nil {
		return "", err
	}
	return fmt.Sprintf("10-d-%04d-%s.json", rank, FlatTargetKey(target)), nil
}

func FlatSlideFilename(deckTarget, slideTarget string, rank int) (string, error) {
	if err := validFlatRank(rank); err != nil {
		return "", err
	}
	return fmt.Sprintf("20-s-%s-%04d-%s.json", FlatTargetKey(deckTarget), rank, FlatTargetKey(slideTarget)), nil
}

func FlatSlideAssetFilename(slideManifest, extension string) (string, error) {
	if !strings.HasSuffix(slideManifest, ".json") {
		return "", fmt.Errorf("slide manifest must end in .json")
	}
	extension = strings.ToLower(extension)
	if extension == "" || strings.ContainsAny(extension, `/\\`) || extension[0] != '.' {
		return "", fmt.Errorf("slide content requires a simple file extension")
	}
	name := strings.TrimSuffix(slideManifest, ".json") + extension
	if len(name) > FlatMaxBasename {
		return "", fmt.Errorf("slide content filename exceeds %d characters", FlatMaxBasename)
	}
	return name, nil
}

func FlatItemFilename(slideTarget, itemTarget string, rank int) (string, error) {
	if err := validFlatRank(rank); err != nil {
		return "", err
	}
	return fmt.Sprintf("30-i-%s-%04d-%s.json", FlatTargetKey(slideTarget), rank, FlatTargetKey(itemTarget)), nil
}

func FlatEvidenceFilename(target, identity string) string {
	return fmt.Sprintf("40-e-%s-%s.json", FlatTargetKey(target), FlatKey("evidence\x00"+identity))
}

func validFlatRank(rank int) error {
	if rank < 0 || rank > flatMaxRank {
		return fmt.Errorf("rank must be between 0 and %d for the portable deck layout", flatMaxRank)
	}
	return nil
}

func flatPathIssue(root, name string) string {
	if len(name) > FlatMaxBasename {
		return fmt.Sprintf("portable deck basename %q is %d bytes; Windows portability limit is %d bytes. Restore the canonical filename or recreate the record through the public CLI; do not hand-rename generated records", name, len(name), FlatMaxBasename)
	}
	return FlatPathBudgetIssue(root, len(name))
}

// FlatPathBudgetIssue preserves the absolute-path budget used for Windows
// portability. Creators reserve FlatMaxBasename bytes for future bundle records;
// readers check the actual filename length. Diagnostics distinguish that filename
// allowance from the directory so callers can fix location without changing IDs.
func FlatPathBudgetIssue(directory string, filenameBytes int) string {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Sprintf("cannot resolve deck directory for Windows portability check: %v", err)
	}
	length := len(filepath.Join(abs, "x")) - 1 + filenameBytes
	if length > FlatMaxPath {
		return fmt.Sprintf("portable deck path requires %d bytes (directory %q: %d bytes; filename allowance: %d bytes), exceeding the %d-byte Windows portability budget (%d-byte reduction required). Fix: use a shorter checkout/worktree or Saga location; for a new deck or review, shorter IDs can also help. Preserve existing record IDs and generated filenames, keep the portability guard enabled, then run change-saga validate --json <saga> at the shorter location and retry", length, abs, len(abs), filenameBytes, FlatMaxPath, length-FlatMaxPath)
	}
	return ""
}

func flatRegular(entry fs.DirEntry) bool {
	info, err := entry.Info()
	return err == nil && info.Mode().IsRegular()
}
