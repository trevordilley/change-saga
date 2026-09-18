// Package coderef is the Saga's evidence model: a node explains code by
// referencing it at a commit. A reference names a commit, a repository path,
// an optional inclusive line range (absent for a whole file), and a digest of
// the referenced content. The repository is the Saga's declared source
// repository, so references do not repeat it.
//
// A diff is never stored. A comparison between two commits is a view of the
// references: each one is resolved at the commits being compared, remapped
// when its lines only moved, and reported stale when its lines changed.
package coderef

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DigestPrefix names the digest algorithm. It is part of the persisted value so
// a later algorithm can be introduced without guessing which one produced it.
const DigestPrefix = "sha256:"

// Reference is one persisted code reference. Start and End are both zero for a
// whole-file reference; otherwise they are an inclusive 1-based line range.
type Reference struct {
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Start  int    `json:"start,omitempty"`
	End    int    `json:"end,omitempty"`
	Digest string `json:"digest"`
	Note   string `json:"note,omitempty"`
}

// WholeFile reports whether the reference names a file rather than lines.
func (reference Reference) WholeFile() bool {
	return reference.Start == 0 && reference.End == 0
}

// Location is the reference without its digest and note.
func (reference Reference) Location() Location {
	return Location{Commit: reference.Commit, Path: reference.Path, Start: reference.Start, End: reference.End}
}

// Key identifies the referenced location and content, excluding the note.
func (reference Reference) Key() string {
	return reference.Location().String() + "@" + reference.Digest
}

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	rangeSuffix   = regexp.MustCompile(`#L([1-9][0-9]*)(?:-L([1-9][0-9]*))?$`)
)

// ValidCommit reports whether value is a full lowercase Git object name.
func ValidCommit(value string) bool { return commitPattern.MatchString(value) }

// Validate checks the persisted shape. It needs no repository: whether the
// commit exists and the digest matches is decided by a Resolver.
func Validate(reference Reference) error {
	if err := reference.Location().Validate(); err != nil {
		return err
	}
	if !digestPattern.MatchString(reference.Digest) {
		return fmt.Errorf("code reference digest must be %s followed by 64 lowercase hex digits", DigestPrefix)
	}
	return nil
}

// Location is a place in the repository at a commit. Its string spelling is
// Git's <commit>:<path> with GitHub's #L<start>-L<end> line suffix, so it can
// be pasted into tooling and parsed back without ambiguity: the commit is a
// fixed-width object name and the range suffix is anchored at the end.
type Location struct {
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Start  int    `json:"start,omitempty"`
	End    int    `json:"end,omitempty"`
}

func (location Location) WholeFile() bool { return location.Start == 0 && location.End == 0 }

func (location Location) Validate() error {
	if !commitPattern.MatchString(location.Commit) {
		return fmt.Errorf("code reference commit must be a full lowercase Git object name")
	}
	if err := ValidatePath(location.Path); err != nil {
		return err
	}
	if location.WholeFile() {
		return nil
	}
	if location.Start < 1 || location.End < location.Start {
		return fmt.Errorf("code reference line range must satisfy 1 <= start <= end")
	}
	return nil
}

func (location Location) String() string {
	value := location.Commit + ":" + location.Path
	if location.WholeFile() {
		return value
	}
	if location.Start == location.End {
		return value + "#L" + strconv.Itoa(location.Start)
	}
	return value + "#L" + strconv.Itoa(location.Start) + "-L" + strconv.Itoa(location.End)
}

// Contains reports whether other lies inside this location: same commit and
// path, and a line range within this one. A whole-file location contains every
// location in its file.
func (location Location) Contains(other Location) bool {
	if location.Commit != other.Commit || location.Path != other.Path {
		return false
	}
	if location.WholeFile() {
		return true
	}
	return !other.WholeFile() && other.Start >= location.Start && other.End <= location.End
}

// ParseLocation parses the canonical string spelling and rejects any other.
func ParseLocation(value string) (Location, error) {
	colon := strings.IndexByte(value, ':')
	if colon < 0 {
		return Location{}, fmt.Errorf("code location must be <commit>:<path>[#L<start>[-L<end>]]")
	}
	location := Location{Commit: value[:colon]}
	rest := value[colon+1:]
	if match := rangeSuffix.FindStringSubmatchIndex(rest); match != nil {
		location.Start, _ = strconv.Atoi(rest[match[2]:match[3]])
		location.End = location.Start
		if match[4] >= 0 {
			location.End, _ = strconv.Atoi(rest[match[4]:match[5]])
		}
		rest = rest[:match[0]]
	}
	location.Path = rest
	if err := location.Validate(); err != nil {
		return Location{}, err
	}
	if location.String() != value {
		return Location{}, fmt.Errorf("code location is not canonical; canonical form is %s", location.String())
	}
	return location, nil
}

// ValidatePath requires a normalized repository-relative slash path.
func ValidatePath(value string) error {
	if value == "" || value == "." || value == ".." || strings.HasPrefix(value, "../") || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return fmt.Errorf("code reference path must be a normalized repository-relative slash path")
	}
	if len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' {
		return fmt.Errorf("code reference path must be repository-relative")
	}
	return nil
}

// DigestBytes digests exactly the given content.
func DigestBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return DigestPrefix + hex.EncodeToString(sum[:])
}

// Lines splits content into lines that keep their terminating newline, so the
// digest of a range is the digest of exactly the bytes those lines occupy.
func Lines(content []byte) [][]byte {
	var lines [][]byte
	for len(content) > 0 {
		index := bytes.IndexByte(content, '\n')
		if index < 0 {
			lines = append(lines, content)
			break
		}
		lines = append(lines, content[:index+1])
		content = content[index+1:]
	}
	return lines
}

// DigestRange digests lines start..end (inclusive, 1-based) of content, or the
// whole content when both are zero.
func DigestRange(content []byte, start, end int) (string, error) {
	if start == 0 && end == 0 {
		return DigestBytes(content), nil
	}
	return DigestLines(Lines(content), start, end)
}

// DigestLines digests lines start..end of a file already split by Lines.
func DigestLines(lines [][]byte, start, end int) (string, error) {
	if start < 1 || end < start || end > len(lines) {
		return "", fmt.Errorf("line range %d-%d is outside the file's %d lines", start, end, len(lines))
	}
	digest := sha256.New()
	for _, line := range lines[start-1 : end] {
		digest.Write(line)
	}
	return DigestPrefix + hex.EncodeToString(digest.Sum(nil)), nil
}

// FindDigest returns every start line at which a window of width lines has the
// given digest. It is the fallback that finds the same lines in any commit.
func FindDigest(content []byte, width int, digest string) []int {
	return FindDigestLines(Lines(content), width, digest)
}

// FindDigestLines is FindDigest over a file already split by Lines.
func FindDigestLines(lines [][]byte, width int, digest string) []int {
	var matches []int
	for start := 0; width > 0 && start+width <= len(lines); start++ {
		hash := sha256.New()
		for _, line := range lines[start : start+width] {
			hash.Write(line)
		}
		if DigestPrefix+hex.EncodeToString(hash.Sum(nil)) == digest {
			matches = append(matches, start+1)
		}
	}
	return matches
}
