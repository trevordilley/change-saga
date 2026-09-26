package changeview

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagalineage"
)

// Reason is one commit in the comparison, attached to a record because the
// commit touched the record's files or code the record references. A
// squash-merged commit carries the branch commits its ___merges record kept.
type Reason struct {
	Commit    string              `json:"commit"`
	Author    string              `json:"author"`
	Date      string              `json:"date"`
	Subject   string              `json:"subject"`
	Body      string              `json:"body,omitempty"`
	Touched   []string            `json:"touched"`
	Collapsed []saga.MergedCommit `json:"collapsed,omitempty"`
}

// Touch kinds.
const (
	TouchedRecord = "record"
	TouchedCode   = "code"
)

type commitInfo struct {
	Reason
	files []string
}

// readCommits lists the commits in from..to, oldest first, with the files
// each touched. paths, when set, limits the log to them.
func readCommits(ctx context.Context, repo, from, to string, paths ...string) ([]commitInfo, error) {
	revision := to
	if from != "" {
		revision = from + ".." + to
	}
	args := []string{"-C", repo, "log", "--reverse", "--no-renames", "--name-only", "--format=%x1e%H%x00%an <%ae>%x00%aI%x00%s%x00%b%x00", revision, "--"}
	args = append(args, paths...)
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return nil, err
	}
	commits := []commitInfo{}
	for _, record := range strings.Split(string(output), "\x1e") {
		fields := strings.SplitN(record, "\x00", 6)
		if len(fields) != 6 {
			continue
		}
		info := commitInfo{Reason: Reason{Commit: fields[0], Author: fields[1], Date: fields[2], Subject: fields[3], Body: strings.TrimSpace(fields[4])}}
		for _, line := range strings.Split(fields[5], "\n") {
			if line = strings.TrimSpace(line); line != "" {
				info.files = append(info.files, line)
			}
		}
		commits = append(commits, info)
	}
	return commits, nil
}

// blame attributes each line of path at head to the commit in from..head
// that introduced it. Lines older than the range are omitted.
func blame(ctx context.Context, repo, from, head, path string) map[int]string {
	lines := map[int]string{}
	output, err := exec.CommandContext(ctx, "git", "-C", repo, "blame", "--porcelain", from+".."+head, "--", path).Output()
	if err != nil {
		return lines
	}
	boundary := map[string]bool{}
	var commit string
	var final int
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "\t"):
			if !boundary[commit] {
				lines[final] = commit
			}
		case line == "boundary":
			boundary[commit] = true
		default:
			fields := strings.Fields(line)
			if len(fields) >= 3 && len(fields[0]) == 40 {
				commit = fields[0]
				final, _ = strconv.Atoi(fields[2])
			}
		}
	}
	return lines
}

// reasonSources says where commit reasons come from. Code commits come from
// the code repository; record commits from the Saga repository, which is the
// same repository unless the Saga is a companion.
type reasonSources struct {
	codeRepo, sagaRepo, sagaPath string
	codeFrom, codeTo             string
	sagaFrom, sagaTo             string
	companion                    bool
}

// attachReasons attaches every commit in the comparison to the records it
// touched: their files, or the code they reference.
func attachReasons(ctx context.Context, layers *Layers, sources reasonSources, base, head *Inventory, merges []saga.Merge) {
	codeCommits, err := readCommits(ctx, sources.codeRepo, sources.codeFrom, sources.codeTo)
	if err != nil {
		layers.Diagnostics = append(layers.Diagnostics, Diagnostic{Code: "commit_reasons_unavailable", Message: err.Error()})
		return
	}
	recordCommits := codeCommits
	if sources.companion {
		recordCommits = nil
		if sources.sagaTo != "" {
			recordCommits, _ = readCommits(ctx, sources.sagaRepo, sources.sagaFrom, sources.sagaTo, sources.sagaPath)
		}
	}
	collapsed := map[string]saga.Merge{}
	for _, merge := range merges {
		collapsed[merge.Commit] = merge
	}
	inRange := map[string]bool{}
	for _, commit := range codeCommits {
		inRange[commit.Commit] = true
	}

	// Which records own which Saga files, at either side.
	prefix := strings.TrimSuffix(sources.sagaPath, "/") + "/"
	if sources.sagaPath == "" || sources.sagaPath == "." {
		prefix = ""
	}
	// A moved Saga owned the same files at each path it had, and a commit
	// that only moved a file changed nothing its record says.
	prefixes := []string{prefix}
	unchanged := map[string]map[string]bool{}
	if prefix != "" {
		lineage := sagalineage.Of(ctx, sources.sagaRepo, sources.sagaPath)
		for _, move := range lineage.Moves {
			prefixes = append(prefixes, move.From+"/")
			carried := sagalineage.CarriedBy(ctx, sources.sagaRepo, move)
			moved := map[string]bool{}
			for relative, blob := range carried.Before {
				if carried.After[relative] == blob {
					moved[path.Join(move.From, relative)] = true
					moved[path.Join(move.To, relative)] = true
				}
			}
			unchanged[move.Commit] = moved
		}
	}
	owners := map[string]string{}
	for _, inventory := range []*Inventory{base, head} {
		for _, node := range inventory.Nodes {
			for _, file := range node.Files {
				for _, at := range prefixes {
					owners[at+file] = node.URN
				}
			}
		}
	}

	attached := map[string]map[string]*Reason{}
	attach := func(urn string, commit commitInfo, touched string) {
		if attached[urn] == nil {
			attached[urn] = map[string]*Reason{}
		}
		reason := attached[urn][commit.Commit]
		if reason == nil {
			value := commit.Reason
			value.Touched = []string{}
			if merge, ok := collapsed[commit.Commit]; ok {
				for _, branch := range merge.Commits {
					if !inRange[branch.Commit] {
						value.Collapsed = append(value.Collapsed, branch)
					}
				}
			}
			reason = &value
			attached[urn][commit.Commit] = reason
		}
		for _, existing := range reason.Touched {
			if existing == touched {
				return
			}
		}
		reason.Touched = append(reason.Touched, touched)
	}
	used := map[string]bool{}
	for _, commit := range recordCommits {
		for _, file := range commit.files {
			if unchanged[commit.Commit][file] {
				continue
			}
			if urn, ok := owners[file]; ok {
				attach(urn, commit, TouchedRecord)
				used[commit.Commit] = true
			}
		}
	}

	// Code: added lines by the commit that introduced them; deleted lines and
	// file events by every commit that touched the file.
	byCommit := map[string]commitInfo{}
	touchedPath := map[string][]commitInfo{}
	for _, commit := range codeCommits {
		byCommit[commit.Commit] = commit
		for _, file := range commit.files {
			touchedPath[file] = append(touchedPath[file], commit)
		}
	}
	blamed := map[string]map[int]string{}
	codeReasons := func(urn string, hunks []Hunk) {
		for _, hunk := range hunks {
			if hunk.Event != "" {
				for _, commit := range touchedPath[hunk.Path] {
					attach(urn, commit, TouchedCode)
					used[commit.Commit] = true
				}
				continue
			}
			for _, line := range hunk.Lines {
				if line.Side == "old" {
					for _, commit := range touchedPath[hunk.Path] {
						attach(urn, commit, TouchedCode)
						used[commit.Commit] = true
					}
					continue
				}
				if blamed[hunk.Path] == nil {
					blamed[hunk.Path] = blame(ctx, sources.codeRepo, sources.codeFrom, sources.codeTo, hunk.Path)
				}
				if commit, ok := byCommit[blamed[hunk.Path][line.Line]]; ok {
					attach(urn, commit, TouchedCode)
					used[commit.Commit] = true
				}
			}
		}
	}
	for _, group := range layers.Code.Groups {
		codeReasons(group.URN, group.Hunks)
	}

	list := func(urn string) []Reason {
		reasons := []Reason{}
		for _, reason := range attached[urn] {
			sort.Strings(reason.Touched)
			reasons = append(reasons, *reason)
		}
		sort.Slice(reasons, func(i, j int) bool {
			if reasons[i].Date != reasons[j].Date {
				return reasons[i].Date < reasons[j].Date
			}
			return reasons[i].Commit < reasons[j].Commit
		})
		return reasons
	}
	for index := range layers.Changed {
		layers.Changed[index].Reasons = list(layers.Changed[index].URN)
	}
	for index := range layers.Affected {
		layers.Affected[index].Reasons = list(layers.Affected[index].URN)
	}
	for index := range layers.Code.Groups {
		layers.Code.Groups[index].Reasons = list(layers.Code.Groups[index].URN)
	}
	layers.Unattached = []Reason{}
	for _, commit := range append(append([]commitInfo{}, codeCommits...), companionOnly(recordCommits, sources.companion)...) {
		if !used[commit.Commit] {
			value := commit.Reason
			value.Touched = []string{}
			layers.Unattached = append(layers.Unattached, value)
		}
	}
}

func companionOnly(commits []commitInfo, companion bool) []commitInfo {
	if companion {
		return commits
	}
	return nil
}
