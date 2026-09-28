package reviewstate

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
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
	// the Saga lives outside the code checkout, the base does not resolve,
	// or the history fits both a merged change and one not yet begun.
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

// Landings detects which reviews have landed in their base. It only reads;
// the durable record is still repin --onto.
//
// A review is committed with the change it explains, so the first question is
// whether its record, review.json, is in the base at all. When it is, the
// commit on the base's first-parent line that brought the record in (its
// landing commit) and the head the review follows say how:
//
//   - The head is in the base and was merged in, not a commit of the base's
//     own line: a merge commit merged the branch.
//   - The landing commit merged a branch and names the review's pull request
//     or branch: the review's change as it stood then was merged, even when
//     the branch was reused and has moved on since.
//   - The landing commit is a squash of the review's pull request, its
//     subject ending "(#N)".
//   - The head is outside the base but merging it would change nothing: a
//     squash or rebase with the branch still present.
//
// A head still outside the base that adds something is open. What fits both
// a merged change and one not yet begun is unknown, never merged: a record
// committed to the base ahead of its change, whose branch has no commits of
// its own yet, does not resolve here, or is HEAD, looks exactly like a
// fast-forward merge.
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
	unknown := State{State: StateOpen, Source: StateUnknown}
	if landings == nil {
		return unknown
	}
	record, ok := landings.recordPath(ctx, review)
	if !ok {
		return unknown
	}
	refs := landings.baseRefs(ctx, review.Base)
	if len(refs) == 0 {
		return unknown
	}
	result := State{State: StateOpen, Source: StateDetected}
	for _, ref := range refs {
		commit := landings.resolve(ctx, ref)
		if !landings.hasFile(ctx, commit, record) {
			continue
		}
		switch landings.verdict(ctx, review, commit, record) {
		case StateMerged:
			return State{State: StateMerged, Source: StateDetected, LandedIn: ref}
		case StateUnknown:
			result = unknown
		}
	}
	return result
}

// verdict says whether review, whose record base holds, has landed in base:
// StateMerged, StateOpen, or StateUnknown.
func (landings *Landings) verdict(ctx context.Context, review *saga.Review, base, record string) string {
	landing := landings.landingCommit(ctx, base, record)
	if landing.commit == "" {
		return StateUnknown
	}
	merge := len(landing.parents) >= 2
	names := namesReview(landing.subject, review)
	head := ""
	if review.Head != "" && review.Head != "HEAD" {
		if resolved, _, err := ResolveHead(ctx, landings.checkout, review); err == nil {
			head = resolved
		}
	}
	if head == "" {
		// Only the landing commit is left to go on.
		if (merge && names) || squashOf(landing.subject, review) {
			return StateMerged
		}
		return StateUnknown
	}
	if landings.isAncestor(ctx, head, base) {
		if !landings.onFirstParentLine(ctx, head, base) || (merge && names) || squashOf(landing.subject, review) {
			return StateMerged
		}
		return StateUnknown
	}
	if landings.addsNothing(ctx, head, base) || squashOf(landing.subject, review) {
		return StateMerged
	}
	if merge && names {
		for _, parent := range landing.parents[1:] {
			if landings.isAncestor(ctx, parent, head) {
				return StateMerged
			}
		}
	}
	return StateOpen
}

// landing is the commit on a base's first-parent line that added a record.
type landing struct {
	commit  string
	parents []string
	subject string
}

// landingCommit finds the commit on base's first-parent line that added
// record, which a merge, a squash, or a commit of the base's own adds.
func (landings *Landings) landingCommit(ctx context.Context, base, record string) landing {
	value := remember(ctx, landings.checkout, "landing", base+":"+record, func() any {
		output, err := gitOutput(ctx, landings.top, "log", "--first-parent", "--diff-filter=A", "-1", "--format=%H%x1f%P%x1f%s", base, "--", record)
		fields := strings.SplitN(output, "\x1f", 3)
		if err != nil || len(fields) != 3 {
			return landing{}
		}
		return landing{commit: fields[0], parents: strings.Fields(fields[1]), subject: fields[2]}
	})
	return value.(landing)
}

// namesReview reports whether a merge's subject names review: its pull
// request's number, as "#12" in "Merge pull request #12 from ...", or the
// branch it follows. A subject naming another pull request is that pull
// request's merge, even from the same branch, so the branch then counts for
// nothing.
func namesReview(subject string, review *saga.Review) bool {
	number := 0
	if review.PullRequest != nil {
		number = review.PullRequest.Number
	}
	if numbers := pullRequestNumbers.FindAllStringSubmatch(subject, -1); len(numbers) > 0 {
		for _, match := range numbers {
			if match[1] == strconv.Itoa(number) {
				return true
			}
		}
		if number > 0 {
			return false
		}
	}
	branch := strings.TrimPrefix(review.Head, "origin/")
	if branch == "" || branch == "HEAD" || gitexec.NamesObjects(branch) {
		return false
	}
	quoted := regexp.QuoteMeta(branch)
	// Git's and GitHub's merge subjects, and this repository's own
	// "Merge BRANCH: why": the branch, never a longer one ending in it.
	return regexp.MustCompile(`^Merge pull request #\d+ from [^/\s]+/` + quoted + `(\s|$)` +
		`|^Merge (remote-tracking )?branch '(origin/)?` + quoted + `'` +
		`|^Merge ` + quoted + `(:|\s|$)`).MatchString(subject)
}

// pullRequestNumbers finds the "#N" a merge subject names.
var pullRequestNumbers = regexp.MustCompile(`#(\d+)\b`)

// squashOf reports whether subject is a squash of review's pull request,
// which GitHub ends with "(#N)".
func squashOf(subject string, review *saga.Review) bool {
	return review.PullRequest != nil && review.PullRequest.Number > 0 &&
		strings.HasSuffix(strings.TrimSpace(subject), "(#"+strconv.Itoa(review.PullRequest.Number)+")")
}

// isAncestor reports whether commit is base or one of its ancestors.
func (landings *Landings) isAncestor(ctx context.Context, commit, base string) bool {
	if commit == base {
		return true
	}
	return remember(ctx, landings.checkout, "ancestor", commit+" "+base, func() any {
		_, err := gitOutput(ctx, landings.checkout, "merge-base", "--is-ancestor", commit, base)
		return err == nil
	}).(bool)
}

// onFirstParentLine reports whether head, an ancestor of base, is a commit
// of base's own first-parent line rather than one a merge brought in.
func (landings *Landings) onFirstParentLine(ctx context.Context, head, base string) bool {
	if head == base {
		return true
	}
	return remember(ctx, landings.checkout, "line", head+" "+base, func() any {
		// base's first-parent line down to where it meets head's history:
		// head itself when head is on the line, or the fork point a merge
		// brought head in from.
		output, err := gitOutput(ctx, landings.checkout, "rev-list", "--first-parent", head+".."+base)
		lines := strings.Fields(output)
		if err != nil || len(lines) == 0 {
			return err == nil
		}
		parent, err := revParse(ctx, landings.checkout, lines[len(lines)-1]+"^1")
		return err != nil || parent == head
	}).(bool)
}

// addsNothing reports whether merging head into base would change nothing:
// base already holds head's change, as after a squash or rebase. merge-tree
// (Git 2.38) merges without touching the checkout; a conflict, or a Git too
// old to ask, reports that it adds something.
func (landings *Landings) addsNothing(ctx context.Context, head, base string) bool {
	return remember(ctx, landings.checkout, "within", head+" "+base, func() any {
		merged, err := gitOutput(ctx, landings.checkout, "merge-tree", "--write-tree", base, head)
		if err != nil {
			return false
		}
		tree, err := revParse(ctx, landings.checkout, base+"^{tree}")
		return err == nil && strings.SplitN(merged, "\n", 2)[0] == tree
	}).(bool)
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

// hasFile reports whether commit's tree holds path as a file, asking the
// session's cat-file process for its type rather than reading it.
func (landings *Landings) hasFile(ctx context.Context, commit, path string) bool {
	if commit == "" {
		return false
	}
	name := commit + ":" + path
	return remember(ctx, landings.checkout, "file", name, func() any {
		if kind, ok := gitexec.ObjectType(ctx, landings.checkout, name); ok {
			return kind == "blob"
		}
		kind, err := gitOutput(ctx, landings.checkout, "cat-file", "-t", name)
		return err == nil && kind == "blob"
	}).(bool)
}

// settled remembers answers about commits, which never change: whether a
// commit holds a file, which commit brought a record into a base, and how a
// head relates to a base. A page that lists every review asks them on each
// render, and after the first only the refs are looked up again.
var settled struct {
	sync.Mutex
	answers map[string]any
}

// settledLimit bounds settled; it starts over rather than evicting.
const settledLimit = 20000

func remember(ctx context.Context, checkout, kind, key string, ask func() any) any {
	key = checkout + "\x00" + kind + "\x00" + key
	settled.Lock()
	answer, ok := settled.answers[key]
	settled.Unlock()
	if ok {
		return answer
	}
	answer = ask()
	// A canceled read may have answered for want of time.
	if ctx.Err() != nil {
		return answer
	}
	settled.Lock()
	if settled.answers == nil || len(settled.answers) >= settledLimit {
		settled.answers = map[string]any{}
	}
	settled.answers[key] = answer
	settled.Unlock()
	return answer
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
