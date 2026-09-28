// Package reviewstate reports a pull request review slide by slide: every
// reviewer's current decision on each review slide, whether that decision is
// out of date for the pull request's current head, and the slide's open
// discussion. It reports; it never declares a review approved. What a team
// requires is the team's rule, written over this report.
package reviewstate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitattribution"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Currency of a decision against the pull request's current head.
const (
	Current   = "current"
	OutOfDate = "out_of_date"
	// Unknown means the decision's commit is not in this checkout, so what
	// changed since cannot be read.
	Unknown = "unknown"
)

// Range is the comparison a review is viewed as: the merge-base of its base
// and head, and the head, like a pull request.
type Range struct {
	BaseOID string `json:"base_oid"`
	HeadOID string `json:"head_oid"`
	// Following is the ref whose commits the review follows; empty once the
	// review is frozen after merge.
	Following string `json:"following,omitempty"`
	Frozen    bool   `json:"frozen"`
	Note      string `json:"note,omitempty"`
}

// ResolveRange reads a review's current range in checkout. A frozen review
// uses the commits recorded at merge; when its head commit is gone it falls
// back to the landed commit and its parent.
func ResolveRange(ctx context.Context, checkout string, review *saga.Review) (Range, error) {
	if merged := review.Merged; merged != nil {
		if commitExists(ctx, checkout, merged.Head) && commitExists(ctx, checkout, merged.Base) {
			return Range{BaseOID: merged.Base, HeadOID: merged.Head, Frozen: true}, nil
		}
		if commitExists(ctx, checkout, merged.Landed) {
			parent, err := revParse(ctx, checkout, merged.Landed+"^1")
			if err == nil {
				return Range{BaseOID: parent, HeadOID: merged.Landed, Frozen: true, Note: "the review's head commit is no longer in this repository; showing the commit it landed as"}, nil
			}
		}
		return Range{}, fmt.Errorf("review %s is frozen at %s..%s, and neither those commits nor the landed commit %s are in %s", review.ID, short(merged.Base), short(merged.Head), short(merged.Landed), checkout)
	}
	head, following, err := ResolveHead(ctx, checkout, review)
	if err != nil {
		return Range{}, err
	}
	// A CI checkout often has origin's branches but no local ones, so a base
	// named as a branch falls back to origin's.
	base, err := gitOutput(ctx, checkout, "merge-base", review.Base, head)
	if err != nil && !strings.HasPrefix(review.Base, "origin/") {
		base, err = gitOutput(ctx, checkout, "merge-base", "origin/"+review.Base, head)
	}
	if err != nil {
		return Range{}, fmt.Errorf("review %s: no merge-base between %s and %s", review.ID, review.Base, short(head))
	}
	return Range{BaseOID: base, HeadOID: head, Following: following}, nil
}

// ResolveHead resolves the head an open review follows: its --head ref (or
// origin's branch of that name, as a CI checkout has), or HEAD when it names
// none. It returns the commit and the ref that resolved.
func ResolveHead(ctx context.Context, checkout string, review *saga.Review) (string, string, error) {
	following := review.Head
	if following == "" {
		following = "HEAD"
	}
	candidates := []string{following}
	if following != "HEAD" && !strings.HasPrefix(following, "origin/") {
		candidates = append(candidates, "origin/"+following)
	}
	for _, candidate := range candidates {
		if head, err := revParse(ctx, checkout, candidate+"^{commit}"); err == nil {
			return head, candidate, nil
		}
	}
	return "", "", fmt.Errorf("review %s follows %s, which does not resolve in %s", review.ID, following, checkout)
}

// Report is one review, slide by slide.
type Report struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Target      string            `json:"target"`
	Path        string            `json:"path"`
	PullRequest *saga.PullRequest `json:"pull_request,omitempty"`
	Base        string            `json:"base"`
	Head        string            `json:"head,omitempty"`
	Range       *Range            `json:"range,omitempty"`
	Merged      *saga.ReviewMerge `json:"merged,omitempty"`
	// State is "open" or "merged", and StateSource how that is known:
	// "recorded" by repin --onto, "detected" from Git at read time, or
	// "unknown" when Git cannot tell. LandedIn names the ref a detected
	// review was found in.
	State       string        `json:"state"`
	StateSource string        `json:"state_source"`
	LandedIn    string        `json:"landed_in,omitempty"`
	Slides      []SlideReport `json:"slides"`
	// Coverage is how completely the deck accounts for the review's range;
	// it is absent, with a diagnostic, when the range cannot be read.
	Coverage *Coverage `json:"coverage,omitempty"`
	// LineThreads is every thread on code lines of the review's diff, each
	// placed in its current range, and OpenLineThreads how many are open.
	LineThreads     []LineThread `json:"line_threads,omitempty"`
	OpenLineThreads int          `json:"open_line_threads"`
	// Diagnostics say why part of the report could not be read, such as a
	// head that does not resolve in this checkout.
	Diagnostics []string `json:"diagnostics"`
}

// SlideReport is one review slide: every reviewer's current decision and the
// slide's discussion. There is deliberately no slide-level verdict.
type SlideReport struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Target      string           `json:"target"`
	Decisions   []DecisionReport `json:"decisions"`
	Comments    int              `json:"comments"`
	OpenThreads int              `json:"open_threads"`
	// OpenLineThreads is how many of the open threads are on code lines.
	OpenLineThreads int `json:"open_line_threads"`
	// Callouts are the slide's callout Items: the surprises its author
	// called out, each saying what a reviewer would expect and what the
	// change does instead.
	Callouts []CalloutReport `json:"callouts,omitempty"`
}

// CalloutReport is one callout Item on a review slide. About names the
// Item on the same slide that the callout explains, when it names one.
type CalloutReport struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Body  string `json:"body"`
	About string `json:"about,omitempty"`
}

// DecisionReport is one reviewer's latest decision on a slide.
type DecisionReport struct {
	ID       string                `json:"id"`
	State    string                `json:"state"`
	Reviewer saga.ReviewerIdentity `json:"reviewer"`
	// Author is who recorded the decision, read from Git: the committer of
	// its record, or the local Git user before it is committed.
	Author    string    `json:"author"`
	Commit    string    `json:"commit"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Currency  string    `json:"currency"`
	// Reasons say what changed between the decision's commit and the head.
	Reasons []string `json:"reasons,omitempty"`
}

// Options carries what a report reads besides the review.
type Options struct {
	// SkipCoverage projects fresh decisions and discussion without generating
	// coverage. The resolver still checks every decision's exact currency.
	SkipCoverage bool
	// Checkout is the code repository the review's commits live in.
	Checkout string
	// SagaRoot attributes decision records through Git.
	SagaRoot string
	Resolver *coderesolve.Resolver
	// Repository is the Saga's declared source repository.
	Repository string
	// Landings detects whether the review has landed; one is shared by every
	// review of a read. Build makes its own when it is nil.
	Landings *Landings
}

// Build reports review. It never fails on a range that cannot be read: the
// decisions are still reported, with unknown currency and a diagnostic.
func Build(ctx context.Context, review *saga.Review, options Options) Report {
	report := newReport(review)
	landings := options.Landings
	if landings == nil {
		landings = NewLandings(options.Checkout)
	}
	state := landings.Detect(ctx, review)
	report.State, report.StateSource, report.LandedIn = state.State, state.Source, state.LandedIn
	var rng *Range
	if value, err := ResolveRange(ctx, options.Checkout, review); err != nil {
		report.Diagnostics = append(report.Diagnostics, err.Error())
	} else {
		rng = &value
		report.Range = rng
		if value.Note != "" {
			report.Diagnostics = append(report.Diagnostics, value.Note)
		}
		if options.Resolver != nil && !options.SkipCoverage {
			if covered, err := ReadCoverage(ctx, review, value, options.Checkout, options.Repository, options.Resolver); err != nil {
				report.Diagnostics = append(report.Diagnostics, "the review's coverage could not be read: "+err.Error())
			} else {
				report.Coverage = covered
			}
		}
	}
	if review.Deck == nil {
		return report
	}
	report.LineThreads = LineThreads(ctx, review, rng, options.Resolver)
	for _, thread := range report.LineThreads {
		if thread.State == saga.CommentOpen {
			report.OpenLineThreads++
		}
	}
	if len(report.LineThreads) == 0 {
		report.LineThreads = nil
	}
	authors := attributions(ctx, options.SagaRoot, review)
	latest := LatestDecisions(review.Approvals, authors)
	threads := Threads(review.Comments)
	for _, slide := range review.Deck.Slides {
		slideReport := SlideReport{ID: slide.ID, Title: slide.Title, Target: slide.Target, Decisions: []DecisionReport{}}
		for _, item := range slide.Items {
			if item.Kind == "callout" {
				slideReport.Callouts = append(slideReport.Callouts, CalloutReport{ID: item.ID, Label: item.Label, Body: item.Body, About: item.About})
			}
		}
		digest, digestErr := saga.SlideDigest(slide)
		for _, approval := range latest[slide.ID] {
			if approval.State == saga.ApprovalNone {
				continue
			}
			decision := DecisionReport{
				ID: approval.ID, State: approval.State, Reviewer: approval.Reviewer, Author: authors[approval.ID],
				Commit: approval.Commit, Body: approval.Body, CreatedAt: approval.CreatedAt,
			}
			decision.Currency, decision.Reasons = currency(ctx, options.Resolver, rng, slide, approval, digest, digestErr)
			slideReport.Decisions = append(slideReport.Decisions, decision)
		}
		for _, thread := range threads {
			if thread.Root.Target == slide.Target || strings.HasPrefix(thread.Root.Target, slide.Target+":item:") {
				slideReport.Comments += 1 + len(thread.Replies)
				if thread.State == saga.CommentOpen {
					slideReport.OpenThreads++
					if thread.Root.CodeLine != nil {
						slideReport.OpenLineThreads++
					}
				}
			}
		}
		report.Slides = append(report.Slides, slideReport)
	}
	return report
}

// newReport is review's report before anything is read from Git.
func newReport(review *saga.Review) Report {
	return Report{
		ID: review.ID, Title: review.Title, Target: review.Target, Path: review.Path,
		PullRequest: review.PullRequest, Base: review.Base, Head: review.Head, Merged: review.Merged,
		Slides: []SlideReport{}, Diagnostics: []string{},
	}
}

// Outline is review's report without its range, decisions, or coverage:
// what it is, its slides, and state, which a list shows at once while the
// rest is read.
func Outline(review *saga.Review, state State) Report {
	report := newReport(review)
	report.State, report.StateSource, report.LandedIn = state.State, state.Source, state.LandedIn
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			report.Slides = append(report.Slides, SlideReport{ID: slide.ID, Title: slide.Title, Target: slide.Target, Decisions: []DecisionReport{}})
		}
	}
	return report
}

// LatestDecisions keeps each reviewer's latest decision per slide, in the
// order they were made. A reviewer is the Git author together with the
// declared reviewer seat, so one person's direct decision and the decisions
// they record through distinct AI reviewers stay separate.
func LatestDecisions(approvals []saga.ReviewApproval, authors map[string]string) map[string][]saga.ReviewApproval {
	latest := map[string]map[string]saga.ReviewApproval{}
	for _, approval := range approvals {
		key := strings.ToLower(strings.Join([]string{authors[approval.ID], approval.Reviewer.Kind, approval.Reviewer.Name, approval.Reviewer.Agent, approval.Reviewer.Model}, "\x00"))
		if latest[approval.Slide] == nil {
			latest[approval.Slide] = map[string]saga.ReviewApproval{}
		}
		latest[approval.Slide][key] = approval
	}
	result := map[string][]saga.ReviewApproval{}
	for slide, byReviewer := range latest {
		for _, approval := range byReviewer {
			result[slide] = append(result[slide], approval)
		}
		sort.Slice(result[slide], func(i, j int) bool {
			left, right := result[slide][i], result[slide][j]
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
			return left.ID < right.ID
		})
	}
	return result
}

// Thread is a root comment and its replies. Its state is the last state any
// of them set; a thread starts open.
type Thread struct {
	Root    saga.ReviewComment   `json:"root"`
	Replies []saga.ReviewComment `json:"replies"`
	State   string               `json:"state"`
}

// Threads groups comments, which are already in time order.
func Threads(comments []saga.ReviewComment) []*Thread {
	byID := map[string]*Thread{}
	parent := map[string]string{}
	var threads []*Thread
	for _, comment := range comments {
		if comment.ReplyTo == "" {
			thread := &Thread{Root: comment, Replies: []saga.ReviewComment{}, State: saga.CommentOpen}
			if comment.State != "" {
				thread.State = comment.State
			}
			byID[comment.ID] = thread
			threads = append(threads, thread)
			continue
		}
		parent[comment.ID] = comment.ReplyTo
	}
	rootOf := func(id string) string {
		for seen := 0; seen < len(comments); seen++ {
			next, ok := parent[id]
			if !ok {
				return id
			}
			id = next
		}
		return id
	}
	for _, comment := range comments {
		if comment.ReplyTo == "" {
			continue
		}
		thread := byID[rootOf(comment.ID)]
		if thread == nil {
			continue
		}
		thread.Replies = append(thread.Replies, comment)
		if comment.State != "" {
			thread.State = comment.State
		}
	}
	return threads
}

// currency decides whether approval is out of date at the range's head: the
// slide changed since it was decided, or the code the slide references
// changed between the decision's commit and the head.
func currency(ctx context.Context, resolver *coderesolve.Resolver, rng *Range, slide *saga.Slide, approval saga.ReviewApproval, digest string, digestErr error) (string, []string) {
	var reasons []string
	if digestErr != nil {
		reasons = append(reasons, "the slide could not be read: "+digestErr.Error())
	} else if digest != approval.SlideDigest {
		reasons = append(reasons, "the slide changed since this decision")
	}
	if rng == nil || resolver == nil {
		if len(reasons) > 0 {
			return OutOfDate, reasons
		}
		return Unknown, []string{"the review's head could not be read"}
	}
	if approval.Commit == rng.HeadOID {
		if len(reasons) > 0 {
			return OutOfDate, reasons
		}
		return Current, nil
	}
	if exists, err := resolver.CommitExists(ctx, approval.Commit); err != nil || !exists {
		if len(reasons) > 0 {
			return OutOfDate, reasons
		}
		return Unknown, []string{"the decision's commit " + short(approval.Commit) + " is not in this repository"}
	}
	seen := map[string]bool{}
	for _, item := range slide.Items {
		for _, file := range item.Code {
			for _, reference := range file.References {
				if reason := codeChanged(ctx, resolver, reference, approval.Commit, rng.HeadOID); reason != "" && !seen[reason] {
					seen[reason] = true
					reasons = append(reasons, reason)
				}
			}
		}
	}
	if len(reasons) > 0 {
		return OutOfDate, reasons
	}
	return Current, nil
}

// codeChanged reports how the code a reference names changed between the
// decision's commit and the head. The referenced lines are located at the
// decision's commit and read forward to the head; a reference those lines no
// longer match at the decision's commit (it was written later) is read at the
// head and back instead, and one that matches neither is compared as its
// whole file.
func codeChanged(ctx context.Context, resolver *coderesolve.Resolver, reference coderef.Reference, decided, head string) string {
	describe := func(location coderef.Location) string {
		if location.WholeFile() {
			return location.Path + " changed since this decision"
		}
		return fmt.Sprintf("%s lines %d-%d changed since this decision", location.Path, location.Start, location.End)
	}
	if atDecision := resolver.Resolve(ctx, reference, decided); atDecision.Current() {
		pinned, err := resolver.Author(ctx, atDecision.Location, "")
		if err == nil {
			if forward := resolver.Resolve(ctx, pinned, head); !forward.Current() {
				return describe(atDecision.Location)
			}
			return ""
		}
	}
	location := coderef.Location{Commit: head, Path: reference.Path}
	if atHead := resolver.Resolve(ctx, reference, head); atHead.Current() {
		location = atHead.Location
	}
	atHead, err := resolver.Author(ctx, location, "")
	if err != nil {
		if _, found, _ := resolver.Blob(ctx, decided, reference.Path); found {
			return reference.Path + " was removed since this decision"
		}
		return ""
	}
	if back := resolver.Resolve(ctx, atHead, decided); !back.Current() {
		return describe(location)
	}
	return ""
}

// attributions names who recorded each decision: the committer of its
// record, or the local Git user while it is uncommitted.
func attributions(ctx context.Context, sagaRoot string, review *saga.Review) map[string]string {
	result := map[string]string{}
	if len(review.Approvals) == 0 || sagaRoot == "" {
		return result
	}
	resolver := gitattribution.New(ctx, sagaRoot)
	paths := make([]string, len(review.Approvals))
	for index, approval := range review.Approvals {
		paths[index] = approval.Path
	}
	local := ""
	for index, value := range resolver.ResolveAll(ctx, paths) {
		author := value.State
		switch value.State {
		case gitattribution.Committed:
			author = strings.TrimSpace(value.Name + " <" + value.Email + ">")
		case gitattribution.Uncommitted:
			if local == "" {
				name, _ := gitexec.ConfigOutput(ctx, sagaRoot, "config", "user.name")
				email, _ := gitexec.ConfigOutput(ctx, sagaRoot, "config", "user.email")
				local = strings.TrimSpace(strings.TrimSpace(string(name)) + " <" + strings.TrimSpace(string(email)) + ">")
			}
			author = local
		}
		result[review.Approvals[index].ID] = author
	}
	return result
}

func commitExists(ctx context.Context, dir, commit string) bool {
	_, err := revParse(ctx, dir, commit+"^{commit}")
	return err == nil
}

// revParse resolves revision, which every caller names as a commit.
func revParse(ctx context.Context, dir, revision string) (string, error) {
	if commit, ok := gitexec.ResolveCommit(ctx, dir, revision); ok {
		return commit, nil
	}
	return gitOutput(ctx, dir, "rev-parse", "--verify", "--quiet", revision)
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	output, err := gitexec.Output(ctx, append([]string{"-C", dir}, args...)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
