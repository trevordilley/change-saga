package reviewstate

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A review's state: whether its change is still open or has landed.
const (
	StateOpen   = "open"
	StateMerged = "merged"
)

// How a review's state is known.
const (
	// StateRecorded is a review frozen by repin --onto: review.json says so.
	StateRecorded = "recorded"
	// StateDetected is read from Git at read time and never written.
	StateDetected = "detected"
	// StateUnknown means Git could not tell, so the review is reported open:
	// the Saga lives outside the code checkout, or the base does not resolve.
	StateUnknown = "unknown"
)

// State is what a review's report says about its landing.
type State struct {
	State  string
	Source string
	// LandedIn is the ref the review was found in, when detected.
	LandedIn string
}

// Merged reports whether the review's change has landed, recorded or
// detected.
func (state State) Merged() bool { return state.State == StateMerged }

// Landings detects which reviews have landed in their base. A review is
// committed with the change it explains, so a review has landed when its own
// record, review.json, is in its base's tree. That holds however the change
// merged — fast-forward, merge commit, squash, or rebase — and after its
// branch is deleted, where asking whether the head is an ancestor of the base
// would miss a squash and would mistake a branch with no commits yet for a
// merged one. It only reads; the durable record is still repin --onto.
//
// One Landings serves every review of one read and is safe to share between
// the workers that build their reports.
type Landings struct {
	checkout string
	once     sync.Once
	top      string
	mu       sync.Mutex
	refs     map[string]string
	fallback []string
}

// NewLandings detects landings in checkout, the code repository.
func NewLandings(checkout string) *Landings {
	return &Landings{checkout: checkout, refs: map[string]string{}}
}

// Detect reports review's state. A recorded merge always wins.
func (landings *Landings) Detect(ctx context.Context, review *saga.Review) State {
	if review.Merged != nil {
		return State{State: StateMerged, Source: StateRecorded}
	}
	if landings == nil {
		return State{State: StateOpen, Source: StateUnknown}
	}
	record, ok := landings.recordPath(ctx, review)
	if !ok {
		return State{State: StateOpen, Source: StateUnknown}
	}
	refs := landings.baseRefs(ctx, review.Base)
	if len(refs) == 0 {
		return State{State: StateOpen, Source: StateUnknown}
	}
	for _, ref := range refs {
		if landings.hasFile(ctx, landings.resolve(ctx, ref), record) {
			return State{State: StateMerged, Source: StateDetected, LandedIn: ref}
		}
	}
	return State{State: StateOpen, Source: StateDetected}
}

// recordPath is review.json's path in the checkout's repository, or false
// when the Saga does not live in it.
func (landings *Landings) recordPath(ctx context.Context, review *saga.Review) (string, bool) {
	landings.once.Do(func() {
		top, err := gitexec.TopLevel(ctx, landings.checkout)
		if err != nil {
			return
		}
		if resolved, err := filepath.EvalSymlinks(top); err == nil {
			top = resolved
		}
		landings.top = top
	})
	if landings.top == "" || review.Directory == "" {
		return "", false
	}
	directory := review.Directory
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		directory = resolved
	}
	rel, err := filepath.Rel(landings.top, filepath.Join(directory, saga.ReviewManifestName))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// baseRefs are the refs a review lands in: its base, as named and as
// origin's. A base pinned to a commit never moves, and a base branch can be
// deleted once it merged, so either reads origin's default branch instead.
func (landings *Landings) baseRefs(ctx context.Context, base string) []string {
	var refs []string
	if base != "" && !looksLikeCommit(base) {
		candidates := []string{base}
		if !strings.HasPrefix(base, "origin/") {
			candidates = append(candidates, "origin/"+base)
		}
		for _, candidate := range candidates {
			if landings.resolve(ctx, candidate) != "" {
				refs = append(refs, candidate)
			}
		}
	}
	if len(refs) > 0 {
		return refs
	}
	return landings.defaultBranch(ctx)
}

// defaultBranch is origin's default branch, else a main or master.
func (landings *Landings) defaultBranch(ctx context.Context) []string {
	landings.mu.Lock()
	if landings.fallback != nil {
		defer landings.mu.Unlock()
		return landings.fallback
	}
	landings.mu.Unlock()
	candidates := []string{}
	if output, err := gitOutput(ctx, landings.checkout, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD"); err == nil && output != "" {
		candidates = append(candidates, output)
	}
	candidates = append(candidates, "origin/main", "main", "origin/master", "master")
	found := []string{}
	for _, candidate := range candidates {
		if landings.resolve(ctx, candidate) != "" {
			found = append(found, candidate)
			break
		}
	}
	landings.mu.Lock()
	defer landings.mu.Unlock()
	landings.fallback = found
	return found
}

// resolve names ref's commit, remembered for the read.
func (landings *Landings) resolve(ctx context.Context, ref string) string {
	landings.mu.Lock()
	commit, ok := landings.refs[ref]
	landings.mu.Unlock()
	if ok {
		return commit
	}
	commit, _ = revParse(ctx, landings.checkout, ref+"^{commit}")
	landings.mu.Lock()
	landings.refs[ref] = commit
	landings.mu.Unlock()
	return commit
}

// hasFile reports whether commit's tree holds path, through the session's
// cat-file process when there is one.
func (landings *Landings) hasFile(ctx context.Context, commit, path string) bool {
	if commit == "" {
		return false
	}
	name := commit + ":" + path
	if kind, _, ok := gitexec.ReadObject(ctx, landings.checkout, name); ok {
		return kind == "blob"
	}
	kind, err := gitOutput(ctx, landings.checkout, "cat-file", "-t", name)
	return err == nil && kind == "blob"
}

// looksLikeCommit reports whether base is an abbreviated or full commit ID
// rather than a branch name.
func looksLikeCommit(base string) bool {
	if len(base) < 7 || len(base) > 64 {
		return false
	}
	for _, r := range base {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}
