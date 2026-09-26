// Package semanticcheck compares Change Saga requirements at explicit Git
// refs. It is deliberately read-only: it extracts committed snapshots into
// temporary directories and never checks out, merges, or updates a ref.
package semanticcheck

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/sagalineage"
)

const intentThreshold = 0.60

const (
	maxRefs         = 32
	maxArchiveFiles = 100_000
	maxArchiveBytes = 512 << 20
)

var (
	flatDeckIdentityName  = regexp.MustCompile(`^10-d-[0-9]{4}-[0-9a-f]{12}\.json$`)
	flatSlideIdentityName = regexp.MustCompile(`^20-s-[0-9a-f]{12}-[0-9]{4}-[0-9a-f]{12}\.json$`)
)

type Options struct {
	Repository string
	SagaPath   string
	Refs       []string
}

type Provenance struct {
	Ref      string `json:"ref"`
	Commit   string `json:"commit"`
	SagaPath string `json:"saga_path"`
}

type Collision struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	Provenance  []Provenance      `json:"provenance"`
	Paths       map[string]string `json:"paths_by_ref"`
	Explanation string            `json:"explanation"`
}

type CompetingHeads struct {
	Story          string              `json:"story"`
	RevisionHeads  map[string][]string `json:"revision_heads"`
	LifecycleHeads map[string][]string `json:"lifecycle_heads"`
	Provenance     []Provenance        `json:"provenance"`
	Explanation    string              `json:"explanation"`
}

type IntentCandidate struct {
	Left        string     `json:"left"`
	Right       string     `json:"right"`
	Score       float64    `json:"score"`
	SharedTerms []string   `json:"shared_terms"`
	Provenance  Provenance `json:"left_provenance"`
	Other       Provenance `json:"right_provenance"`
	Explanation string     `json:"explanation"`
}

type Report struct {
	ReadOnly         bool              `json:"read_only"`
	Heuristic        string            `json:"heuristic"`
	Refs             []Provenance      `json:"refs"`
	Collisions       []Collision       `json:"stable_id_collisions"`
	CompetingHeads   []CompetingHeads  `json:"competing_heads"`
	IntentCandidates []IntentCandidate `json:"overlapping_intent_candidates"`
}

type snapshot struct {
	Provenance Provenance
	Document   requirements.Document
	Identities map[string]identityRecord
}

type identityRecord struct {
	digest string
	path   string
}

// Check resolves and reads only the requested refs. A minimum of two explicit
// refs is required so HEAD is never silently substituted for author intent.
func Check(ctx context.Context, options Options) (Report, error) {
	if len(options.Refs) < 2 {
		return Report{}, fmt.Errorf("at least two explicit --ref values are required")
	}
	if len(options.Refs) > maxRefs {
		return Report{}, fmt.Errorf("at most %d refs may be compared at once", maxRefs)
	}
	repo, err := filepath.Abs(options.Repository)
	if err != nil {
		return Report{}, err
	}
	sagaPath := filepath.ToSlash(filepath.Clean(options.SagaPath))
	if sagaPath == "." || filepath.IsAbs(options.SagaPath) || sagaPath == ".." || strings.HasPrefix(sagaPath, "../") {
		return Report{}, fmt.Errorf("--saga must be a repository-relative Saga path")
	}
	// A ref from before the Saga was renamed reads it where it was then.
	lineage := sagalineage.Of(ctx, repo, sagaPath)
	seen := map[string]bool{}
	snapshots := make([]snapshot, 0, len(options.Refs))
	for _, ref := range options.Refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return Report{}, fmt.Errorf("each --ref must be non-empty and unique")
		}
		seen[ref] = true
		commit, err := resolveRef(ctx, repo, ref)
		if err != nil {
			return Report{}, err
		}
		at, exists := lineage.PathAt(ctx, repo, commit)
		if !exists {
			return Report{}, fmt.Errorf("read %s at ref %q: the Saga did not exist yet at %s", sagaPath, ref, commit)
		}
		value, err := readSnapshot(ctx, repo, at, ref, commit)
		if err != nil {
			return Report{}, err
		}
		snapshots = append(snapshots, value)
	}
	return analyze(snapshots), nil
}

func resolveRef(ctx context.Context, repo, ref string) (string, error) {
	if commit, ok := gitexec.ResolveCommit(ctx, repo, ref); ok {
		return commit, nil
	}
	out, err := gitexec.Output(ctx, "-C", repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	commit := strings.TrimSpace(string(out))
	if (len(commit) != 40 && len(commit) != 64) || strings.Trim(commit, "0123456789abcdef") != "" {
		return "", fmt.Errorf("resolve ref %q: Git returned an invalid commit", ref)
	}
	return commit, nil
}

func readSnapshot(ctx context.Context, repo, sagaPath, ref, commit string) (snapshot, error) {
	temporary, err := os.MkdirTemp("", "change-saga-preintegrate-*")
	if err != nil {
		return snapshot{}, err
	}
	defer os.RemoveAll(temporary)
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "archive", "--format=tar", commit, "--", sagaPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	archive, err := cmd.StdoutPipe()
	if err != nil {
		return snapshot{}, err
	}
	if err := cmd.Start(); err != nil {
		return snapshot{}, fmt.Errorf("read %s at ref %q: %w", sagaPath, ref, err)
	}
	if err := extract(archive, temporary); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return snapshot{}, fmt.Errorf("read %s at ref %q: %w", sagaPath, ref, err)
	}
	// The tar reader stops at the end-of-archive marker, but git pads the
	// stream to a whole record after it. Windows pipes buffer far less than
	// that padding, so git blocks writing it unless the rest is read.
	if _, err := io.Copy(io.Discard, archive); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return snapshot{}, fmt.Errorf("read %s at ref %q: %w", sagaPath, ref, err)
	}
	if err := cmd.Wait(); err != nil {
		return snapshot{}, fmt.Errorf("read %s at ref %q: %w: %s", sagaPath, ref, err, strings.TrimSpace(stderr.String()))
	}
	root := filepath.Join(temporary, filepath.FromSlash(sagaPath))
	document, err := requirements.Load(root, "")
	if err != nil {
		return snapshot{}, fmt.Errorf("load %s at ref %q: %w", sagaPath, ref, err)
	}
	identities, err := immutableIdentities(root)
	if err != nil {
		return snapshot{}, fmt.Errorf("scan identities in %s at ref %q: %w", sagaPath, ref, err)
	}
	return snapshot{Provenance: Provenance{Ref: ref, Commit: commit, SagaPath: sagaPath}, Document: document, Identities: identities}, nil
}

func immutableIdentities(root string) (map[string]identityRecord, error) {
	kinds := map[string]string{
		"feature.json": "feature", "story.json": "story", "persona.json": "persona", "flag.json": "flag",
		"term.json": "term", "prototype.json": "prototype", "test-case.json": "test-case",
		"wave.json": "wave", "work-item.json": "work-item", "contract.json": "contract",
	}
	result := map[string]identityRecord{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		kind := kinds[entry.Name()]
		flatDeck := flatDeckIdentityName.MatchString(entry.Name())
		flatSlide := flatSlideIdentityName.MatchString(entry.Name())
		if kind == "" && !flatDeck && !flatSlide {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) > requirements.MaxRecordBytes {
			return fmt.Errorf("%s exceeds the record size limit", entry.Name())
		}
		var identity struct {
			ID   string `json:"id"`
			Deck string `json:"deck"`
		}
		if err := json.Unmarshal(data, &identity); err != nil || identity.ID == "" {
			return fmt.Errorf("read %s immutable identity", entry.Name())
		}
		relative, _ := filepath.Rel(root, path)
		identityBytes := data
		switch {
		case flatDeck:
			kind = "deck"
			owner, relErr := filepath.Rel(root, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}
			identityBytes = []byte("deck-owner\x00" + filepath.ToSlash(owner))
		case flatSlide:
			kind = "slide"
			if identity.Deck == "" {
				return fmt.Errorf("read %s immutable slide ownership", entry.Name())
			}
			owner, relErr := filepath.Rel(root, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}
			identityBytes = []byte("slide-owner\x00" + filepath.ToSlash(owner) + "\x00" + identity.Deck)
		}
		sum := sha256.Sum256(identityBytes)
		key := kind + "\x00" + identity.ID
		if existing, ok := result[key]; ok {
			return fmt.Errorf("duplicate %s id %q at %s and %s", kind, identity.ID, existing.path, filepath.ToSlash(relative))
		}
		result[key] = identityRecord{digest: hex.EncodeToString(sum[:]), path: filepath.ToSlash(relative)}
		return nil
	})
	return result, err
}

func extract(reader io.Reader, destination string) error {
	tr := tar.NewReader(reader)
	files, total := 0, int64(0)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			files++
			total += header.Size
			if files > maxArchiveFiles || header.Size < 0 || total > maxArchiveBytes {
				return fmt.Errorf("Saga archive exceeds the %d-file or %d-byte comparison limit", maxArchiveFiles, maxArchiveBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, io.LimitReader(tr, header.Size))
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("archive contains unsupported entry %q", header.Name)
		}
	}
}

func analyze(snapshots []snapshot) Report {
	report := Report{ReadOnly: true, Heuristic: "case-folded word tokens; Jaccard intersection/union >= 0.60; candidates are not semantic-equivalence decisions"}
	for _, value := range snapshots {
		report.Refs = append(report.Refs, value.Provenance)
	}
	// Immutable identity bytes distinguish an independently reused stable ID
	// from ordinary revisions of the same story.
	keys := map[string]bool{}
	for _, value := range snapshots {
		for key := range value.Identities {
			keys[key] = true
		}
	}
	for key := range keys {
		digests := map[string]bool{}
		var provenance []Provenance
		paths := map[string]string{}
		for _, value := range snapshots {
			if record, ok := value.Identities[key]; ok {
				digests[record.digest] = true
				provenance = append(provenance, value.Provenance)
				paths[value.Provenance.Ref] = record.path
			}
		}
		if len(digests) > 1 {
			parts := strings.Split(key, "\x00")
			report.Collisions = append(report.Collisions, Collision{Kind: parts[0], ID: parts[1], Provenance: provenance, Paths: paths, Explanation: "the same kind and stable ID has different immutable identity or ownership across refs"})
		}
	}

	type locatedStory struct {
		story requirements.Story
		at    Provenance
	}
	byID := map[string][]locatedStory{}
	var all []locatedStory
	for _, value := range snapshots {
		for _, story := range value.Document.Stories {
			located := locatedStory{story: story, at: value.Provenance}
			byID[story.Identity.ID] = append(byID[story.Identity.ID], located)
			all = append(all, located)
		}
	}
	for id, values := range byID {
		if len(values) < 2 {
			continue
		}
		revisions, lifecycle := map[string][]string{}, map[string][]string{}
		uniqueRevisions, uniqueLifecycle := map[string]bool{}, map[string]bool{}
		var provenance []Provenance
		for _, value := range values {
			revisions[value.at.Ref] = sorted(value.story.RevisionHeads)
			lifecycle[value.at.Ref] = sorted(value.story.LifecycleHeads)
			uniqueRevisions[strings.Join(revisions[value.at.Ref], "\x00")] = true
			uniqueLifecycle[strings.Join(lifecycle[value.at.Ref], "\x00")] = true
			provenance = append(provenance, value.at)
		}
		if len(uniqueRevisions) > 1 || len(uniqueLifecycle) > 1 {
			report.CompetingHeads = append(report.CompetingHeads, CompetingHeads{Story: id, RevisionHeads: revisions, LifecycleHeads: lifecycle, Provenance: provenance, Explanation: "the refs expose different current revision or lifecycle heads; no head is selected automatically"})
		}
	}
	for left := 0; left < len(all); left++ {
		for right := left + 1; right < len(all); right++ {
			if all[left].at.Ref == all[right].at.Ref || all[left].story.Identity.ID == all[right].story.Identity.ID || all[left].story.CurrentRevision == nil || all[right].story.CurrentRevision == nil {
				continue
			}
			leftTerms := tokens(all[left].story.CurrentRevision.Title + " " + all[left].story.CurrentRevision.Statement)
			rightTerms := tokens(all[right].story.CurrentRevision.Title + " " + all[right].story.CurrentRevision.Statement)
			score, shared := jaccard(leftTerms, rightTerms)
			if score < intentThreshold {
				continue
			}
			report.IntentCandidates = append(report.IntentCandidates, IntentCandidate{Left: all[left].story.Identity.ID, Right: all[right].story.Identity.ID, Score: score, SharedTerms: shared, Provenance: all[left].at, Other: all[right].at, Explanation: fmt.Sprintf("token Jaccard %.2f meets the deterministic %.2f candidate threshold; a person must decide meaning", score, intentThreshold)})
		}
	}
	sort.Slice(report.Collisions, func(i, j int) bool {
		return report.Collisions[i].Kind+"\x00"+report.Collisions[i].ID < report.Collisions[j].Kind+"\x00"+report.Collisions[j].ID
	})
	sort.Slice(report.CompetingHeads, func(i, j int) bool { return report.CompetingHeads[i].Story < report.CompetingHeads[j].Story })
	sort.Slice(report.IntentCandidates, func(i, j int) bool {
		left := report.IntentCandidates[i].Left + "\x00" + report.IntentCandidates[i].Right
		right := report.IntentCandidates[j].Left + "\x00" + report.IntentCandidates[j].Right
		return left < right
	})
	return report
}

func sorted(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func tokens(value string) map[string]bool {
	result := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(strings.ToLower(value)))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		word := strings.TrimFunc(scanner.Text(), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if len(word) >= 3 {
			result[word] = true
		}
	}
	return result
}

func jaccard(left, right map[string]bool) (float64, []string) {
	union := map[string]bool{}
	var shared []string
	for value := range left {
		union[value] = true
		if right[value] {
			shared = append(shared, value)
		}
	}
	for value := range right {
		union[value] = true
	}
	sort.Strings(shared)
	if len(union) == 0 {
		return 0, shared
	}
	return float64(len(shared)) / float64(len(union)), shared
}
