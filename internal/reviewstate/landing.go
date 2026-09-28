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
// record, review.json, is in its base's tree and the branch it follows adds
// nothing the base lacks: its head is in the base, or merging it would change
// nothing, as after a squash or rebase. A branch deleted after it merged
// leaves only the record, which is then enough. Asking only whether the head
// is an ancestor of the base would miss a squash and would mistake a branch
// with no commits yet for a merged one; asking for the record too keeps a
// review committed to the base ahead of its change open. It only reads the
// Saga; the durable record is still repin --onto.
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
		commit := landings.resolve(ctx, ref)
		if landings.hasFile(ctx, commit, record) && landings.changeIn(ctx, review, commit) {
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

// changeIn reports whether the branch review follows adds nothing to base:
// its head is base or an ancestor of it, or merging it into base yields
// base's own tree. A review that follows HEAD, or whose branch no longer
// resolves, has only its record to go on.
func (landings *Landings) changeIn(ctx context.Context, review *saga.Review, base string) bool {
	if review.Head == "" || review.Head == "HEAD" {
		return true
	}
	head, _, err := ResolveHead(ctx, landings.checkout, review)
	if err != nil {
		return true
	}
	if head == base {
		return true
	}
	if _, err := gitOutput(ctx, landings.checkout, "merge-base", "--is-ancestor", head, base); err == nil {
		return true
	}
	// A squash or rebase leaves the head outside base. merge-tree (Git
	// 2.38) merges without touching the checkout; a conflict, or a Git too
	// old to ask, leaves the change reported open.
	merged, err := gitOutput(ctx, landings.checkout, "merge-tree", "--write-tree", base, head)
	if err != nil {
		return false
	}
	tree, err := gitOutput(ctx, landings.checkout, "rev-parse", base+"^{tree}")
	return err == nil && strings.SplitN(merged, "\n", 2)[0] == tree
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
