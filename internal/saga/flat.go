package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
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

func flatPathIssue(root, name string) (severity, message string) {
	return flatPathIssueForOS(root, name, runtime.GOOS)
}

func flatPathIssueForOS(root, name, goos string) (severity, message string) {
	if len(name) > FlatMaxBasename {
		return "error", fmt.Sprintf("portable deck basename %q is %d bytes; format limit is %d bytes. Restore the canonical filename or recreate the record through the public CLI; do not hand-rename generated records", name, len(name), FlatMaxBasename)
	}
	if warning := flatPathBudgetWarning(root, len(name), goos); warning != "" {
		return "warning", warning
	}
	return "", ""
}

// FlatPathBudgetWarning reports a local Windows compatibility concern without
// rejecting otherwise valid content. Creators reserve FlatMaxBasename bytes for
// future bundle records; readers check the actual filename length. Other platforms
// do not emit this environment-specific warning.
func FlatPathBudgetWarning(directory string, filenameBytes int) string {
	return flatPathBudgetWarning(directory, filenameBytes, runtime.GOOS)
}

func flatPathBudgetWarning(directory string, filenameBytes int, goos string) string {
	if goos != "windows" {
		return ""
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Sprintf("cannot resolve deck directory for Windows portability check: %v", err)
	}
	length := len(filepath.Join(abs, "x")) - 1 + filenameBytes
	if length > FlatMaxPath {
		return fmt.Sprintf("deck path requires %d bytes (directory %q: %d bytes; filename allowance: %d bytes), exceeding the %d-byte Windows portability budget (%d-byte reduction required). Some Windows tools may not support this location. Use a shorter checkout/worktree or Saga location for compatibility; for a new deck or review, shorter IDs can also help. Preserve existing record IDs and generated filenames, then run change-saga validate --json <saga> at the shorter location. This warning does not invalidate the Saga or block authoring", length, abs, len(abs), filenameBytes, FlatMaxPath, length-FlatMaxPath)
	}
	return ""
}

func flatRegular(entry fs.DirEntry) bool {
	info, err := entry.Info()
	return err == nil && info.Mode().IsRegular()
}
