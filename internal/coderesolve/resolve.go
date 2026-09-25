// Package coderesolve resolves code references against a repository: it
// verifies each reference at its pinned commit, remaps it to a viewed commit
// when its lines only moved, and reports it stale, with a reason, when they
// changed. Nothing it computes is persisted.
package coderesolve

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/gitexec"
)

type State string

const (
	// Current means the referenced content is unchanged at the viewed commit.
	// Its location there may differ from the pin when lines moved or the file
	// was renamed.
	Current State = "current"
	// Stale means the referenced content changed, disappeared, or cannot be
	// verified. A stale reference accounts for nothing until it is re-authored.
	Stale State = "stale"
)

// Resolution is one reference viewed at one commit. Location is where the
// referenced content is at the viewed commit when current, and the pinned
// location when stale.
type Resolution struct {
	State    State            `json:"state"`
	Location coderef.Location `json:"location"`
	Moved    bool             `json:"moved,omitempty"`
	Reason   string           `json:"reason,omitempty"`
}

func (resolution Resolution) Current() bool { return resolution.State == Current }

// Resolver caches repository reads for its lifetime. It is safe for concurrent
// use; one Resolver serves one command or one review session.
type Resolver struct {
	repo string

	mu       sync.Mutex
	objects  *catFile
	blobs    map[string]blobResult
	commits  map[string]bool
	verified map[string]string
	changes  map[[2]string]changeSet
}

type blobResult struct {
	content []byte
	lines   [][]byte
	found   bool
}

type changeSet struct {
	byOldPath map[string]gitdiff.FileChange
	err       error
}

// New opens a resolver for the repository containing dir.
func New(ctx context.Context, dir string) (*Resolver, error) {
	output, err := gitexec.Output(ctx, "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("locate Git repository: %w", err)
	}
	return &Resolver{
		repo: strings.TrimSpace(string(output)), blobs: map[string]blobResult{},
		commits: map[string]bool{}, changes: map[[2]string]changeSet{}, verified: map[string]string{},
	}, nil
}

// Repository is the checkout root the resolver reads.
func (resolver *Resolver) Repository() string { return resolver.repo }

// Close stops the resolver's Git object reader.
func (resolver *Resolver) Close() {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.objects != nil {
		resolver.objects.close()
		resolver.objects = nil
	}
}

// Author builds a reference to location, computing its digest from the
// repository. Authoring fails when the location does not exist.
func (resolver *Resolver) Author(ctx context.Context, location coderef.Location, note string) (coderef.Reference, error) {
	if err := location.Validate(); err != nil {
		return coderef.Reference{}, err
	}
	content, found, err := resolver.Blob(ctx, location.Commit, location.Path)
	if err != nil {
		return coderef.Reference{}, err
	}
	if !found {
		return coderef.Reference{}, fmt.Errorf("%s does not exist at commit %s", location.Path, location.Commit)
	}
	digest, err := coderef.DigestRange(content, location.Start, location.End)
	if err != nil {
		return coderef.Reference{}, fmt.Errorf("%s: %w", location, err)
	}
	return coderef.Reference{Commit: location.Commit, Path: location.Path, Start: location.Start, End: location.End, Digest: digest, Note: note}, nil
}

// Resolve views reference at the commit view, which must be a full object name.
func (resolver *Resolver) Resolve(ctx context.Context, reference coderef.Reference, view string) Resolution {
	pinned := reference.Location()
	stale := func(format string, args ...any) Resolution {
		return Resolution{State: Stale, Location: pinned, Reason: fmt.Sprintf(format, args...)}
	}
	exists, err := resolver.CommitExists(ctx, reference.Commit)
	if err != nil {
		return stale("%v", err)
	}
	if !exists {
		if found, ok := resolver.findByDigest(ctx, reference, view); ok {
			found.Reason = fmt.Sprintf("pinned commit %s is not in this repository; found by content digest", short(reference.Commit))
			return found
		}
		return stale("pinned commit %s is not in this repository and the referenced content was not found at %s", short(reference.Commit), short(view))
	}
	if reason := resolver.verify(ctx, reference); reason != "" {
		return stale("%s", reason)
	}
	if view == reference.Commit {
		return Resolution{State: Current, Location: pinned}
	}
	changes, err := resolver.treeChanges(ctx, reference.Commit, view)
	if err != nil {
		return stale("%v", err)
	}
	change, changed := changes[reference.Path]
	if !changed {
		return Resolution{State: Current, Location: coderef.Location{Commit: view, Path: reference.Path, Start: reference.Start, End: reference.End}}
	}
	if change.Deleted {
		return stale("%s was deleted between %s and %s", reference.Path, short(reference.Commit), short(view))
	}
	moved := change.NewPath != reference.Path
	if reference.WholeFile() {
		if len(change.Hunks) > 0 || change.Binary || change.ModeChange {
			return stale("%s changed between %s and %s", reference.Path, short(reference.Commit), short(view))
		}
		return Resolution{State: Current, Location: coderef.Location{Commit: view, Path: change.NewPath}, Moved: moved}
	}
	if change.Binary {
		return stale("%s changed between %s and %s", reference.Path, short(reference.Commit), short(view))
	}
	start, end, ok := Remap(change.Hunks, reference.Start, reference.End)
	if !ok {
		return stale("lines %d-%d of %s changed between %s and %s", reference.Start, reference.End, reference.Path, short(reference.Commit), short(view))
	}
	return Resolution{
		State: Current, Location: coderef.Location{Commit: view, Path: change.NewPath, Start: start, End: end},
		Moved: moved || start != reference.Start,
	}
}

// Remap moves an inclusive line range across zero-context hunks. It fails
// when any hunk removes a line of the range or inserts lines inside it.
func Remap(hunks []gitdiff.Hunk, start, end int) (int, int, bool) {
	shift := 0
	for _, hunk := range hunks {
		if hunk.OldCount == 0 {
			// A pure insertion after line OldStart.
			switch {
			case hunk.OldStart < start:
				shift += hunk.NewCount
			case hunk.OldStart >= end:
			default:
				return 0, 0, false
			}
			continue
		}
		last := hunk.OldStart + hunk.OldCount - 1
		switch {
		case last < start:
			shift += hunk.NewCount - hunk.OldCount
		case hunk.OldStart > end:
		default:
			return 0, 0, false
		}
	}
	return start + shift, end + shift, true
}

// DiffSince is the patch of the referenced file from its pin to view, for
// showing why a reference went stale.
func (resolver *Resolver) DiffSince(ctx context.Context, reference coderef.Reference, view string) (string, error) {
	paths := []string{reference.Path}
	if changes, err := resolver.treeChanges(ctx, reference.Commit, view); err == nil {
		if change, ok := changes[reference.Path]; ok && change.NewPath != "" && change.NewPath != reference.Path {
			paths = append(paths, change.NewPath)
		}
	}
	return gitdiff.FileDiff(ctx, resolver.repo, reference.Commit, view, paths...)
}

// Find locates reference's content at commit by digest alone, ignoring the
// pinned commit. It is the fallback that finds the same lines in any commit;
// it succeeds only on exactly one match in the pinned path.
func (resolver *Resolver) Find(ctx context.Context, reference coderef.Reference, commit string) (Resolution, bool) {
	return resolver.findByDigest(ctx, reference, commit)
}

// verify checks the reference against its own pin and returns why it fails.
func (resolver *Resolver) verify(ctx context.Context, reference coderef.Reference) string {
	key := reference.Key()
	resolver.mu.Lock()
	reason, ok := resolver.verified[key]
	resolver.mu.Unlock()
	if ok {
		return reason
	}
	blob, err := resolver.blob(ctx, reference.Commit, reference.Path)
	switch {
	case err != nil:
		reason = err.Error()
	case !blob.found:
		reason = fmt.Sprintf("%s does not exist at pinned commit %s", reference.Path, short(reference.Commit))
	default:
		digest, digestErr := resolver.digest(blob, reference.Start, reference.End)
		if digestErr != nil || digest != reference.Digest {
			reason = fmt.Sprintf("content digest does not match %s", reference.Location())
		}
	}
	resolver.mu.Lock()
	resolver.verified[key] = reason
	resolver.mu.Unlock()
	return reason
}

func (resolver *Resolver) digest(blob blobResult, start, end int) (string, error) {
	if start == 0 && end == 0 {
		return coderef.DigestBytes(blob.content), nil
	}
	return coderef.DigestLines(blob.lines, start, end)
}

func (resolver *Resolver) findByDigest(ctx context.Context, reference coderef.Reference, view string) (Resolution, bool) {
	blob, err := resolver.blob(ctx, view, reference.Path)
	if err != nil || !blob.found {
		return Resolution{}, false
	}
	if reference.WholeFile() {
		if coderef.DigestBytes(blob.content) != reference.Digest {
			return Resolution{}, false
		}
		return Resolution{State: Current, Location: coderef.Location{Commit: view, Path: reference.Path}, Moved: view != reference.Commit}, true
	}
	width := reference.End - reference.Start + 1
	matches := coderef.FindDigestLines(blob.lines, width, reference.Digest)
	if len(matches) != 1 {
		return Resolution{}, false
	}
	location := coderef.Location{Commit: view, Path: reference.Path, Start: matches[0], End: matches[0] + width - 1}
	return Resolution{State: Current, Location: location, Moved: matches[0] != reference.Start || view != reference.Commit}, true
}

// CommitExists reports whether commit names a commit object in the repository.
func (resolver *Resolver) CommitExists(ctx context.Context, commit string) (bool, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if exists, ok := resolver.commits[commit]; ok {
		return exists, nil
	}
	objectType, _, err := resolver.readObject(ctx, commit)
	if err != nil {
		return false, err
	}
	resolver.commits[commit] = objectType == "commit"
	return objectType == "commit", nil
}

// Blob reads path at commit. found is false when the path is absent there.
func (resolver *Resolver) Blob(ctx context.Context, commit, path string) ([]byte, bool, error) {
	blob, err := resolver.blob(ctx, commit, path)
	return blob.content, blob.found, err
}

func (resolver *Resolver) blob(ctx context.Context, commit, path string) (blobResult, error) {
	if strings.ContainsAny(path, "\n\r") {
		return blobResult{}, fmt.Errorf("path %q cannot be read", path)
	}
	key := commit + ":" + path
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if cached, ok := resolver.blobs[key]; ok {
		return cached, nil
	}
	objectType, content, err := resolver.readObject(ctx, key)
	if err != nil {
		return blobResult{}, err
	}
	result := blobResult{content: content, found: objectType == "blob"}
	if result.found {
		result.lines = coderef.Lines(content)
	}
	resolver.blobs[key] = result
	return result, nil
}

func (resolver *Resolver) treeChanges(ctx context.Context, from, to string) (map[string]gitdiff.FileChange, error) {
	key := [2]string{from, to}
	resolver.mu.Lock()
	cached, ok := resolver.changes[key]
	resolver.mu.Unlock()
	if ok {
		return cached.byOldPath, cached.err
	}
	changes, err := gitdiff.TreeChanges(ctx, resolver.repo, from, to)
	result := changeSet{byOldPath: map[string]gitdiff.FileChange{}, err: err}
	for _, change := range changes {
		if change.OldPath != "" {
			result.byOldPath[change.OldPath] = change
		}
	}
	resolver.mu.Lock()
	resolver.changes[key] = result
	resolver.mu.Unlock()
	return result.byOldPath, result.err
}

// readObject must be called with mu held.
func (resolver *Resolver) readObject(ctx context.Context, name string) (string, []byte, error) {
	if objectType, content, ok := gitexec.ReadObject(ctx, resolver.repo, name); ok {
		return objectType, content, nil
	}
	if resolver.objects == nil {
		objects, err := startCatFile(resolver.repo)
		if err != nil {
			return "", nil, err
		}
		resolver.objects = objects
	}
	objectType, content, err := resolver.objects.read(name)
	if err != nil {
		resolver.objects.abort()
		resolver.objects = nil
	}
	return objectType, content, err
}

type catFile struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

func startCatFile(repo string) (*catFile, error) {
	cmd := exec.Command("git", "-C", repo, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start git cat-file: %w", err)
	}
	return &catFile{cmd: cmd, stdin: stdin, stdout: bufio.NewReaderSize(stdout, 64*1024)}, nil
}

// read returns the object type ("missing" when absent) and its content.
func (objects *catFile) read(name string) (string, []byte, error) {
	if _, err := io.WriteString(objects.stdin, name+"\n"); err != nil {
		return "", nil, fmt.Errorf("read Git object: %w", err)
	}
	header, err := objects.stdout.ReadString('\n')
	if err != nil {
		return "", nil, fmt.Errorf("read Git object: %w", err)
	}
	fields := strings.Fields(header)
	if len(fields) == 2 && (fields[1] == "missing" || fields[1] == "ambiguous") {
		return "missing", nil, nil
	}
	if len(fields) != 3 {
		return "", nil, fmt.Errorf("read Git object: unexpected header %q", strings.TrimSpace(header))
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", nil, fmt.Errorf("read Git object: unexpected header %q", strings.TrimSpace(header))
	}
	content := make([]byte, size+1)
	if _, err := io.ReadFull(objects.stdout, content); err != nil {
		return "", nil, fmt.Errorf("read Git object: %w", err)
	}
	return fields[1], content[:size], nil
}

func (objects *catFile) close() {
	_ = objects.stdin.Close()
	_ = objects.cmd.Wait()
}

// abort stops a reader that failed mid-answer. Git may still be writing an
// object nobody will read, and once the pipe fills (4 KiB on Windows) it
// blocks, so waiting without killing it would never return.
func (objects *catFile) abort() {
	_ = objects.cmd.Process.Kill()
	objects.close()
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// Pinned resolves without a repository: a reference is current exactly at its
// own commit and stale everywhere else. It suits callers whose comparisons are
// constructed rather than read from Git, such as tests.
type Pinned struct{}

func (Pinned) Resolve(_ context.Context, reference coderef.Reference, view string) Resolution {
	if reference.Commit == view {
		return Resolution{State: Current, Location: reference.Location()}
	}
	return Resolution{State: Stale, Location: reference.Location(), Reason: fmt.Sprintf("pinned at %s, viewed at %s", short(reference.Commit), short(view))}
}
