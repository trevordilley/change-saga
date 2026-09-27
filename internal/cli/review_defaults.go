package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// pullRequestInfo is what gh reports about the current branch's pull request.
type pullRequestInfo struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	BaseRefName string `json:"baseRefName"`
}

// ghTimeout bounds asking gh about the pull request; gh is a convenience and
// never worth waiting long for.
const ghTimeout = 10 * time.Second

// detectPullRequest asks gh, when it is installed and origin is on GitHub,
// for the pull request of the checkout's branch. Anything that goes wrong
// means there is nothing to detect: gh is never required.
// CHANGE_SAGA_NO_GH=1 turns detection off.
func detectPullRequest(ctx context.Context, repo, branch string) (pullRequestInfo, bool) {
	if os.Getenv("CHANGE_SAGA_NO_GH") != "" {
		return pullRequestInfo{}, false
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return pullRequestInfo{}, false
	}
	origin, err := gitexec.ConfigOutput(ctx, repo, "remote", "get-url", "origin")
	if err != nil || !strings.Contains(string(origin), "github.com") {
		return pullRequestInfo{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	// Asked about the branch the review follows, not whatever is checked out.
	args := []string{"pr", "view", "--json", "number,url,title,baseRefName"}
	if branch != "" {
		args = append(args, strings.TrimPrefix(branch, "origin/"))
	}
	command := exec.CommandContext(ctx, "gh", args...)
	command.Dir = repo
	// gh may run child processes that keep its output open; the timeout must
	// still bound the whole call.
	command.WaitDelay = time.Second
	output, err := command.Output()
	if err != nil {
		return pullRequestInfo{}, false
	}
	var value pullRequestInfo
	if json.Unmarshal(output, &value) != nil || value.Number <= 0 {
		return pullRequestInfo{}, false
	}
	return value, true
}

// localOrOriginRef names branch as every checkout knows it: origin's
// remote-tracking branch when there is one, since a CI checkout has origin's
// branches but rarely local ones (and a local branch may be stale), else the
// local branch.
func localOrOriginRef(ctx context.Context, repo, branch string) (string, bool) {
	for _, candidate := range []string{"origin/" + branch, branch} {
		if _, err := gitexec.RepoOutput(ctx, repo, "rev-parse", "--verify", "-q", candidate+"^{commit}"); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// defaultBase is the branch a pull request most likely merges into: origin's
// default branch, else a local main or master.
func defaultBase(ctx context.Context, repo string) (string, string, error) {
	if output, err := gitexec.RepoOutput(ctx, repo, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD"); err == nil {
		if branch := strings.TrimPrefix(strings.TrimSpace(string(output)), "origin/"); branch != "" {
			if ref, ok := localOrOriginRef(ctx, repo, branch); ok {
				return ref, "origin's default branch", nil
			}
		}
	}
	for _, branch := range []string{"main", "master"} {
		if ref, ok := localOrOriginRef(ctx, repo, branch); ok {
			return ref, "the repository's " + branch + " branch", nil
		}
	}
	return "", "", fmt.Errorf("cannot work out what the pull request merges into; pass --base (for example main)")
}

// currentBranch is the branch the checkout has checked out, or empty.
func currentBranch(ctx context.Context, repo string) string {
	output, err := gitexec.RepoOutput(ctx, repo, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// reviewCreateInputs are review create's flags after defaults are filled.
type reviewCreateInputs struct {
	id, base, head, url, title string
	number                     int
	// inferred says, for the author, what was worked out and from where.
	inferred []string
}

// fillReviewDefaults works out what review create was not told: the pull
// request from gh, the base from the pull request or origin's default branch,
// and the id from the pull request number or the branch name. Explicit flags
// always win.
func fillReviewDefaults(ctx context.Context, repo string, in reviewCreateInputs) (reviewCreateInputs, error) {
	// A review follows its branch, not whatever HEAD later points at, so a
	// review created on a branch records it: once the change lands, the next
	// branch's comparisons are not taken for this review's.
	branch := currentBranch(ctx, repo)
	if in.head == "" && branch != "" {
		in.head = branch
		in.inferred = append(in.inferred, "head "+branch)
	}
	if in.id == "" || in.base == "" || (in.number == 0 && in.url == "") {
		if pr, ok := detectPullRequest(ctx, repo, in.head); ok && (in.number == 0 || in.number == pr.Number) {
			if in.number == 0 && in.url == "" {
				in.number, in.url = pr.Number, pr.URL
				in.inferred = append(in.inferred, fmt.Sprintf("pull request #%d (gh)", pr.Number))
			}
			if in.title == "" {
				in.title = pr.Title
			}
			if in.base == "" && pr.BaseRefName != "" {
				if ref, ok := localOrOriginRef(ctx, repo, pr.BaseRefName); ok {
					in.base = ref
					in.inferred = append(in.inferred, "base "+ref+" (the pull request's base)")
				}
			}
		}
	}
	if in.base == "" {
		base, source, err := defaultBase(ctx, repo)
		if err != nil {
			return in, err
		}
		in.base = base
		in.inferred = append(in.inferred, "base "+base+" ("+source+")")
	}
	if in.id == "" {
		switch {
		case in.number > 0:
			in.id = fmt.Sprintf("pr-%d", in.number)
		case in.head != "" && strings.TrimPrefix(in.head, "origin/") != strings.TrimPrefix(in.base, "origin/"):
			in.id = store.Slug(strings.TrimPrefix(in.head, "origin/"))
		default:
			return in, fmt.Errorf("cannot name the review: the checkout is not on a pull request's branch; pass --id (for example pr-42) or --pr N")
		}
		if !saga.ValidID(in.id) {
			return in, fmt.Errorf("cannot name the review from %q; pass --id", in.id)
		}
		in.inferred = append(in.inferred, "id "+in.id)
	}
	return in, nil
}
