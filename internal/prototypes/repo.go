package prototypes

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitexec"
)

// A repo source keeps a prototype's files in the source repository instead of
// copying them into the Saga. It pins them the way code evidence pins lines:
// at a commit, by the Git object of the package at that commit. The package is
// Root, the directory served beside the entrypoint, or the entrypoint alone
// for a single-file prototype, so a prototype at the repository root never
// pins the whole repository.

// RepoPackage is the repository path whose Git object the source pins.
func (s Source) RepoPackage() string {
	if s.Root != "" {
		return s.Root
	}
	return s.Entrypoint
}

// RepoEntry is the entrypoint relative to the served package.
func (s Source) RepoEntry() string {
	if s.Root == "" {
		return path.Base(s.Entrypoint)
	}
	return strings.TrimPrefix(s.Entrypoint, s.Root+"/")
}

// RepoFile maps a path relative to the served package to its repository
// path. A single-file prototype serves its entrypoint and nothing else.
func (s Source) RepoFile(rel string) (string, bool) {
	rel = path.Clean("/" + rel)[1:]
	if rel == "" {
		return "", false
	}
	if s.Root == "" {
		return s.Entrypoint, rel == path.Base(s.Entrypoint)
	}
	return s.Root + "/" + rel, true
}

// ResolveRepoSource pins entrypoint (and root, when the prototype has assets
// beside it) at rev in repo. The package must be committed at rev: a pin
// names content Git can always return, never a working-tree file.
func ResolveRepoSource(ctx context.Context, repo, entrypoint, root, rev string) (Source, error) {
	if rev == "" {
		rev = "HEAD"
	}
	source := Source{Kind: SourceRepo, Entrypoint: entrypoint, Root: root}
	commit, err := gitexec.Output(ctx, "-C", repo, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return Source{}, fmt.Errorf("%s does not name a commit in %s", rev, repo)
	}
	source.Commit = strings.TrimSpace(string(commit))
	if kind, _ := gitexec.Output(ctx, "-C", repo, "cat-file", "-t", source.Commit+":"+entrypoint); strings.TrimSpace(string(kind)) != "blob" {
		return Source{}, fmt.Errorf("%s is not committed at %s; commit the prototype first", entrypoint, short(source.Commit))
	}
	object, err := gitexec.Output(ctx, "-C", repo, "rev-parse", "--verify", "--quiet", source.Commit+":"+source.RepoPackage())
	if err != nil {
		return Source{}, fmt.Errorf("%s is not committed at %s; commit the prototype first", source.RepoPackage(), short(source.Commit))
	}
	source.Object = strings.TrimSpace(string(object))
	var problems validationErrors
	validateSource(&problems, source)
	return source, problems.err()
}

// RepoStaleness says why a repo source no longer matches the package at
// head, or "" while it still does.
func RepoStaleness(ctx context.Context, repo string, source Source, head string) string {
	if source.Kind != SourceRepo {
		return ""
	}
	if head == "" {
		head = "HEAD"
	}
	current, err := gitexec.Output(ctx, "-C", repo, "rev-parse", "--verify", "--quiet", head+":"+source.RepoPackage())
	if err != nil {
		return fmt.Sprintf("%s no longer exists at %s; this revision is shown as pinned at %s.", source.RepoPackage(), head, short(source.Commit))
	}
	if strings.TrimSpace(string(current)) != source.Object {
		return fmt.Sprintf("%s changed since %s; this revision is shown as pinned. Record the change with change-saga prototype update.", source.RepoPackage(), short(source.Commit))
	}
	return ""
}

// ReadRepoFile reads one file of a repo source's package as pinned.
func ReadRepoFile(ctx context.Context, repo string, source Source, rel string) ([]byte, bool) {
	file, ok := source.RepoFile(rel)
	if !ok {
		return nil, false
	}
	name := source.Commit + ":" + file
	if kind, data, answered := gitexec.ReadObject(ctx, repo, name); answered {
		return data, kind == "blob"
	}
	if kind, err := gitexec.Output(ctx, "-C", repo, "cat-file", "-t", name); err != nil || strings.TrimSpace(string(kind)) != "blob" {
		return nil, false
	}
	data, err := gitexec.Output(ctx, "-C", repo, "cat-file", "blob", name)
	return data, err == nil
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
