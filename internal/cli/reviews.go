package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

var reviewOperations = []string{"create", "follow", "list", "refresh-coverage", "approve", "request-changes", "withdraw", "comment"}

// Review is the pull request review family. A review is a pull request's
// slide deck; approval and comments exist only on its slides and Items.
func Review(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("review", reviewOperations, out)
	}
	var err error
	switch args[0] {
	case "create":
		err = reviewCreate(ctx, args[1:], out)
	case "follow":
		err = reviewFollow(ctx, args[1:], out)
	case "list":
		err = reviewList(ctx, args[1:], out)
	case "refresh-coverage":
		err = reviewRefreshCoverage(ctx, args[1:], out)
	case "approve":
		err = reviewDecide(ctx, "review approve", saga.ApprovalApproved, args[1:], out)
	case "request-changes":
		err = reviewDecide(ctx, "review request-changes", saga.ApprovalChangesRequested, args[1:], out)
	case "withdraw":
		err = reviewDecide(ctx, "review withdraw", saga.ApprovalNone, args[1:], out)
	case "comment":
		err = reviewComment(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["review"])
	}
	if err != nil && jsonFlagRequested(args) && args[0] != "list" {
		return reportLivingMutationFailure(out, "review "+args[0], err)
	}
	return err
}

func reviewCreate(ctx context.Context, args []string, out io.Writer) error {
	name := "review create"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable review id, for example pr-42; defaults to pr-N for a pull request, else the branch name")
	base := flags.String("base", "", "the revision the pull request merges into, for example main; defaults to the pull request's base, else origin's default branch")
	head := flags.String("head", "", "the ref the review follows as commits are pushed, usually the pull request's branch; defaults to the checkout's HEAD")
	number := flags.Int("pr", 0, "pull request number; a pull request has one review")
	url := flags.String("url", "", "pull request URL")
	title := flags.String("title", "", "review title; defaults to the pull request")
	objective := flags.String("objective", "", "what the review deck explains; defaults to the transition and why it was made")
	deckID := flags.String("deck", "", "review deck id; defaults to the review id")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	// What was not given is worked out from the checkout: the pull request
	// (through gh, when installed), its base or origin's default branch, and
	// an id from the pull request number or the branch.
	filled, err := fillReviewDefaults(ctx, firstNonEmpty(*repo, flags.Arg(0)), reviewCreateInputs{id: *id, base: *base, head: *head, url: *url, title: *title, number: *number})
	if err != nil {
		return err
	}
	*id, *base, *head, *url, *title, *number = filled.id, filled.base, filled.head, filled.url, filled.title, filled.number
	if *title == "" {
		*title = "Review " + *id
		if *number > 0 {
			*title = fmt.Sprintf("Pull request #%d", *number)
		}
	}
	if *objective == "" {
		*objective = "Explain what this change did and why: the transition and its reasoning."
	}
	if *deckID == "" {
		*deckID = *id
	}
	manifest := saga.ReviewManifest{ID: *id, Title: strings.TrimSpace(*title), Base: *base, Head: *head}
	if *number != 0 || *url != "" {
		manifest.PullRequest = &saga.PullRequest{Number: *number, URL: *url}
	}
	deck := saga.DeckManifest{ID: *deckID, Title: manifest.Title, Objective: strings.TrimSpace(*objective)}
	root := flags.Arg(0)
	if err := reviewstore.Create(root, manifest, deck); err != nil {
		return err
	}
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.ReviewTarget(sagaManifest.ID, *id)
	path := saga.ReviewsDir + "/" + *id + saga.ReviewSuffix
	created := []string{urn, saga.ReviewDeckTarget(sagaManifest.ID, *id, *deckID)}
	staleness := createdReviewStaleness(ctx, root, *repo, *id)
	if *jsonOutput {
		// The JSON says what the review is, including what was worked out.
		return writeJSON(out, reviewCreateOutput{
			livingMutationOutput: livingMutationOutput{OK: true, Operation: name, Resource: urn, Path: path, Created: created, EventIDs: []string{}},
			Review:               reviewCreated{ID: *id, Base: *base, Head: *head, PullRequest: manifest.PullRequest, Inferred: append([]string{}, filled.inferred...)},
			StaleByChange:        staleness,
		})
	}
	if err := writeLivingMutation(out, name, urn, path, created, nil, false, false); err != nil {
		return err
	}
	{
		if len(filled.inferred) > 0 {
			fmt.Fprintf(out, "Using %s; pass the flags to choose otherwise\n", strings.Join(filled.inferred, ", "))
		}
		fmt.Fprintf(out, "Next: change-saga add-slide --review %s --intent explain --layout diagram %s first-slide\n", *id, root)
		if staleness != nil && staleness.Count > 0 {
			fmt.Fprintf(out, "Then: your change made %d living documentation %s stale (%d with a proposed range); read each diff, then repair with change-saga reconcile --against %s %s\n", staleness.Count, plural(staleness.Count, "reference", "references"), staleness.Proposed, shortOID(staleness.BaseOID), root)
		}
	}
	return nil
}

// reviewCreateOutput is review create --json: the mutation, and the review
// as created, with what was worked out rather than given.
type reviewCreateOutput struct {
	livingMutationOutput
	Review reviewCreated `json:"review"`
	// StaleByChange is what the review's change did to the living
	// documentation's references, when the Saga records any.
	StaleByChange *changeStaleness `json:"stale_by_change,omitempty"`
}

// createdReviewStaleness measures a new review's change against the living
// documentation over the review's own range. It never fails the creation.
func createdReviewStaleness(ctx context.Context, root, repo, id string) *changeStaleness {
	document, _, err := saga.Load(root)
	if err != nil {
		return nil
	}
	review := document.FindReview(id)
	if review == nil {
		return nil
	}
	checkout := firstNonEmpty(repo, document.Root)
	rng, err := reviewstate.ResolveRange(ctx, checkout, review)
	if err != nil {
		return nil
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return nil
	}
	defer resolver.Close()
	return livingStaleness(ctx, document, resolver, rng.BaseOID, rng.HeadOID, root, repo, true)
}

type reviewCreated struct {
	ID          string            `json:"id"`
	Base        string            `json:"base"`
	Head        string            `json:"head,omitempty"`
	PullRequest *saga.PullRequest `json:"pull_request,omitempty"`
	Inferred    []string          `json:"inferred"`
}

// reviewFollow sets the ref an open review follows. A review created without
// one follows HEAD, which after its change lands would take the next branch's
// change for its own; naming its branch pins it to its pull request.
func reviewFollow(ctx context.Context, args []string, out io.Writer) error {
	name := "review follow"
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "review id")
	head := flags.String("head", "", "the ref the review follows, usually the pull request's branch")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reviewID, *head); err != nil {
		return err
	}
	root := flags.Arg(0)
	if err := reviewstore.Follow(root, *reviewID, strings.TrimSpace(*head)); err != nil {
		return err
	}
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.ReviewTarget(sagaManifest.ID, *reviewID)
	path := saga.ReviewsDir + "/" + *reviewID + saga.ReviewSuffix
	if *jsonOutput {
		return writeLivingMutation(out, name, urn, path, []string{}, nil, false, true)
	}
	fmt.Fprintf(out, "Review %s now follows %s\nPath: %s\n", *reviewID, *head, path)
	return nil
}

// reviewerFlags are the reviewer seat every decision and comment declares.
type reviewerFlags struct{ kind, name, agent, model *string }

func registerReviewerFlags(flags *flag.FlagSet) reviewerFlags {
	return reviewerFlags{
		kind:  flags.String("reviewer-kind", "", "human for your own decision, ai for an AI reviewer's"),
		name:  flags.String("reviewer-name", "", "a distinct AI reviewer seat, for example Claude 1 (AI only)"),
		agent: flags.String("agent", "", "AI agent kind, for example claude-code (AI only)"),
		model: flags.String("model", "", "exact AI model name (AI only)"),
	}
}

func (value reviewerFlags) identity() (saga.ReviewerIdentity, error) {
	if strings.TrimSpace(*value.kind) == "" {
		return saga.ReviewerIdentity{}, fmt.Errorf("--reviewer-kind human or ai is required")
	}
	reviewer := saga.ReviewerIdentity{Kind: strings.TrimSpace(*value.kind), Name: strings.TrimSpace(*value.name), Agent: strings.TrimSpace(*value.agent), Model: strings.TrimSpace(*value.model)}
	return reviewer, saga.ValidateReviewerIdentity(&reviewer)
}

// reviewHead is the pull request head a decision or comment is given at.
func reviewHead(ctx context.Context, root, repo, reviewID string) (string, *saga.Review, error) {
	document, _, err := saga.Load(root)
	if err != nil {
		return "", nil, err
	}
	review := document.FindReview(reviewID)
	if review == nil {
		return "", nil, fmt.Errorf("review %q does not exist%s", reviewID, knownReviews(document))
	}
	rng, err := reviewstate.ResolveRange(ctx, firstNonEmpty(repo, document.Root), review)
	if err != nil {
		return "", review, err
	}
	return rng.HeadOID, review, nil
}

func knownReviews(document *saga.Saga) string {
	if len(document.Reviews) == 0 {
		return "; the Saga has no reviews yet (change-saga review create)"
	}
	ids := make([]string, 0, len(document.Reviews))
	for _, review := range document.Reviews {
		ids = append(ids, review.ID)
	}
	return "; known reviews: " + strings.Join(ids, ", ")
}

func reviewDecide(ctx context.Context, name, state string, args []string, out io.Writer) error {
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "review id")
	slide := flags.String("slide", "", "review slide id or URN")
	body := flags.String("body", "", "note explaining the decision")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	reviewer := registerReviewerFlags(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reviewID, *slide); err != nil {
		return err
	}
	if state == saga.ApprovalChangesRequested && strings.TrimSpace(*body) == "" {
		return fmt.Errorf("--body is required: say what should change")
	}
	identity, err := reviewer.identity()
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	head, _, err := reviewHead(ctx, root, *repo, *reviewID)
	if err != nil {
		return err
	}
	approval, err := reviewstore.Decide(root, reviewstore.Decision{Review: *reviewID, Slide: *slide, State: state, Reviewer: identity, Commit: head, Body: *body})
	if err != nil {
		return err
	}
	relative := relativeToSaga(root, approval.Path)
	if *jsonOutput {
		return writeLivingMutation(out, name, approval.ID, relative, []string{approval.ID}, []string{approval.ID}, false, true)
	}
	verb := map[string]string{saga.ApprovalApproved: "Approved", saga.ApprovalChangesRequested: "Requested changes on", saga.ApprovalNone: "Withdrew the decision on"}[state]
	fmt.Fprintf(out, "%s slide %s of review %s at %s\nRecord: %s\n", verb, approval.Slide, *reviewID, shortOID(head), relative)
	return nil
}

func reviewComment(ctx context.Context, args []string, out io.Writer) error {
	name := "review comment"
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "review id")
	target := flags.String("target", "", "review slide or Item: its id, <slide>/<item>, or URN")
	replyTo := flags.String("reply-to", "", "comment id this replies to")
	body := flags.String("body", "", "Markdown comment")
	resolve := flags.Bool("resolve", false, "resolve the thread")
	reopen := flags.Bool("reopen", false, "reopen the thread")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	reviewer := registerReviewerFlags(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reviewID, *body); err != nil {
		return err
	}
	if (*target == "") == (*replyTo == "") {
		return fmt.Errorf("provide exactly one of --target or --reply-to")
	}
	if *resolve && *reopen {
		return fmt.Errorf("--resolve and --reopen cannot be combined")
	}
	state := ""
	if *resolve {
		state = saga.CommentResolved
	} else if *reopen {
		state = saga.CommentOpen
	}
	identity, err := reviewer.identity()
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	// A comment records the head it was made at when the head resolves; a
	// comment is still allowed on a review whose head is not checked out.
	head, review, err := reviewHead(ctx, root, *repo, *reviewID)
	if review == nil && err != nil {
		return err
	}
	comment, err := reviewstore.Comment(root, reviewstore.Remark{Review: *reviewID, Target: *target, ReplyTo: *replyTo, Body: *body, State: state, Reviewer: identity, Commit: head})
	if err != nil {
		return err
	}
	relative := relativeToSaga(root, comment.Path)
	if *jsonOutput {
		return writeLivingMutation(out, name, comment.ID, relative, []string{comment.ID}, []string{comment.ID}, false, true)
	}
	fmt.Fprintf(out, "Commented on %s\nComment: %s\nRecord: %s\n", comment.Target, comment.ID, relative)
	return nil
}

// reviewListOutput is review list --json.
type reviewListOutput struct {
	Reviews []reviewstate.Report `json:"reviews"`
	// Repair is, per open review, what its change did to the living
	// documentation's references and its stale Item references, each with a
	// proposed range to accept after reading its diff.
	Repair []reviewRepair `json:"repair,omitempty"`
}

// reviewRepair is one open review's repair signal over its own range.
type reviewRepair struct {
	Review string `json:"review"`
	// StaleByChange is what the review's change did to the living
	// documentation; absent when the Saga records no living references.
	StaleByChange *changeStaleness `json:"stale_by_change,omitempty"`
	// Items are the review's stale Item references with proposals.
	Items []staleRow `json:"items"`
}

// buildReviewRepairs measures each open review whose range reads.
func buildReviewRepairs(ctx context.Context, document *saga.Saga, checkout, root, repo string, reports []reviewstate.Report) []reviewRepair {
	repairs := []reviewRepair{}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return repairs
	}
	defer resolver.Close()
	for _, report := range reports {
		review := document.FindReview(report.ID)
		if review == nil || review.Merged != nil || report.Range == nil || report.Range.Frozen {
			continue
		}
		base, head := report.Range.BaseOID, report.Range.HeadOID
		repairs = append(repairs, reviewRepair{
			Review: report.ID, StaleByChange: livingStaleness(ctx, document, resolver, base, head, root, repo, true),
			Items: reviewItemRows(ctx, resolver, review, base, head, root, repo),
		})
	}
	return repairs
}

// repairFor finds one review's repair signal.
func repairFor(repairs []reviewRepair, id string) *reviewRepair {
	for index := range repairs {
		if repairs[index].Review == id {
			return &repairs[index]
		}
	}
	return nil
}

func reviewList(ctx context.Context, args []string, out io.Writer) error {
	name := "review list"
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "report one review")
	uncovered := flags.Bool("uncovered", false, "list only reviews whose deck leaves changes of their range uncovered, or whose coverage cannot be read, and only those gaps")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	document, validation, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	if !validation.Valid {
		return fmt.Errorf("the Saga is invalid; run change-saga validate")
	}
	reviews := document.Reviews
	if *reviewID != "" {
		review := document.FindReview(*reviewID)
		if review == nil {
			return fmt.Errorf("review %q does not exist%s", *reviewID, knownReviews(document))
		}
		reviews = []*saga.Review{review}
	}
	reports, err := buildReviewReports(ctx, document, firstNonEmpty(*repo, document.Root), reviews)
	if err != nil {
		return err
	}
	if *uncovered {
		gaps := []reviewstate.Report{}
		for _, report := range reports {
			if report.Coverage == nil || report.Coverage.Summary.Uncovered > 0 || report.Coverage.Summary.Stale > 0 {
				gaps = append(gaps, report)
			}
		}
		reports = gaps
	}
	repairs := buildReviewRepairs(ctx, document, firstNonEmpty(*repo, document.Root), flags.Arg(0), *repo, reports)
	if *jsonOutput {
		return writeJSON(out, reviewListOutput{Reviews: reports, Repair: repairs})
	}
	if *uncovered {
		if len(reports) == 0 {
			fmt.Fprintln(out, "No uncovered changes: every listed review's deck explains its whole range.")
		}
		for _, report := range reports {
			fmt.Fprintf(out, "Review %s: %s\n", report.ID, report.Title)
			for _, diagnostic := range report.Diagnostics {
				fmt.Fprintf(out, "  note: %s\n", diagnostic)
			}
			printReviewCoverage(out, report.Coverage, repairFor(repairs, report.ID))
		}
		return nil
	}
	if len(reports) == 0 {
		fmt.Fprintln(out, "No reviews. Create one for a pull request with change-saga review create.")
		return nil
	}
	printReviewReports(out, reports, repairs)
	return nil
}

func buildReviewReports(ctx context.Context, document *saga.Saga, checkout string, reviews []*saga.Review) ([]reviewstate.Report, error) {
	reports := []reviewstate.Report{}
	if len(reviews) == 0 {
		return reports, nil
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		resolver = nil
	} else {
		defer resolver.Close()
	}
	for _, review := range reviews {
		reports = append(reports, reviewstate.Build(ctx, review, reviewstate.Options{Checkout: checkout, SagaRoot: document.Root, Resolver: resolver, Repository: document.Manifest.Source.Repository}))
	}
	return reports, nil
}

// printReviewReports states each slide's decisions and their currency. It
// never sums them into a verdict: the team decides what it requires.
func printReviewReports(out io.Writer, reports []reviewstate.Report, repairs []reviewRepair) {
	for _, report := range reports {
		fmt.Fprintf(out, "Review %s: %s", report.ID, report.Title)
		if report.PullRequest != nil {
			if report.PullRequest.Number > 0 {
				fmt.Fprintf(out, " (#%d)", report.PullRequest.Number)
			}
			if report.PullRequest.URL != "" {
				fmt.Fprintf(out, " %s", report.PullRequest.URL)
			}
		}
		fmt.Fprintln(out)
		switch {
		case report.Range != nil && report.Range.Frozen:
			fmt.Fprintf(out, "  merged: frozen at %s..%s", shortOID(report.Range.BaseOID), shortOID(report.Range.HeadOID))
			if report.Merged != nil {
				fmt.Fprintf(out, ", landed as %s", shortOID(report.Merged.Landed))
			}
			fmt.Fprintln(out)
		case report.Range != nil:
			fmt.Fprintf(out, "  %s..%s (head follows %s, base %s)\n", shortOID(report.Range.BaseOID), shortOID(report.Range.HeadOID), report.Range.Following, report.Base)
		}
		for _, diagnostic := range report.Diagnostics {
			fmt.Fprintf(out, "  note: %s\n", diagnostic)
		}
		printReviewCoverage(out, report.Coverage, repairFor(repairs, report.ID))
		if len(report.Slides) == 0 {
			fmt.Fprintln(out, "  no slides yet")
		}
		for _, slide := range report.Slides {
			fmt.Fprintf(out, "  slide %s: %s\n", slide.ID, slide.Title)
			if len(slide.Decisions) == 0 {
				fmt.Fprintln(out, "    no decision")
			}
			for _, decision := range slide.Decisions {
				fmt.Fprintf(out, "    %s by %s at %s: %s", strings.ReplaceAll(decision.State, "_", " "), reviewerLabel(decision), shortOID(decision.Commit), strings.ReplaceAll(decision.Currency, "_", " "))
				if len(decision.Reasons) > 0 {
					fmt.Fprintf(out, " (%s)", strings.Join(decision.Reasons, "; "))
				}
				fmt.Fprintln(out)
			}
			if slide.Comments > 0 {
				fmt.Fprintf(out, "    %d comments, %d open threads\n", slide.Comments, slide.OpenThreads)
			}
		}
	}
}

// printReviewCoverage states how completely the deck accounts for the
// review's range, and every change it does not. It is reported, never a
// verdict.
func printReviewCoverage(out io.Writer, covered *reviewstate.Coverage, repair *reviewRepair) {
	if repair != nil && repair.StaleByChange != nil {
		staleness := *repair.StaleByChange
		fmt.Fprintf(out, "  living documentation: your change made %d %s stale", staleness.Count, plural(staleness.Count, "reference", "references"))
		if staleness.Count > 0 {
			fmt.Fprintf(out, " (%d with a proposed range; change-saga reconcile --against %s lists them)", staleness.Proposed, shortOID(staleness.BaseOID))
		}
		fmt.Fprintln(out)
	}
	if covered == nil {
		return
	}
	proposals := map[string]staleRow{}
	if repair != nil {
		for _, row := range repair.Items {
			proposals[row.EvidenceFile+"#"+strconv.Itoa(row.Index)] = row
		}
	}
	summary := covered.Summary
	fmt.Fprintf(out, "  coverage: %d of %d changed lines and file events explained by the deck", summary.Covered, summary.Total)
	fmt.Fprintf(out, " (%d uncovered, %d stale %s, %d overlapping)\n", summary.Uncovered, summary.Stale, plural(summary.Stale, "reference", "references"), summary.Overlapping)
	for _, file := range covered.UncoveredFiles {
		fmt.Fprintf(out, "    uncovered %s: %s\n", file.Path, strings.Join(file.Locations, " "))
	}
	for _, stale := range covered.StaleReferences {
		fmt.Fprintf(out, "    stale %s (%s): %s\n", stale.Reference.Location(), stale.Assignment.Target, stale.Reason)
		// The record path is what replace-coverage and remove-coverage take.
		fmt.Fprintf(out, "      record %s\n", stale.Assignment.EvidenceFile)
		if row, ok := proposals[stale.Assignment.EvidenceFile+"#"+strconv.Itoa(stale.Assignment.Reference)]; ok {
			fmt.Fprintf(out, "      proposed: %s\n", describeProposal(row.Pinned, row.Proposal))
			if row.Accept != nil {
				fmt.Fprintf(out, "      accept after reading its diff: %s\n", shellJoin(row.Accept.Argv))
			}
		}
	}
}

func reviewerLabel(decision reviewstate.DecisionReport) string {
	if decision.Reviewer.Kind == "ai" {
		return fmt.Sprintf("AI %s (%s, %s) for %s", decision.Reviewer.Name, decision.Reviewer.Agent, decision.Reviewer.Model, decision.Author)
	}
	return "human " + decision.Author
}

// findReviewDeck resolves the --review selector of the deck authoring
// commands.
func findReviewDeck(document *saga.Saga, reviewID string) (*saga.Review, error) {
	review := document.FindReview(reviewID)
	if review == nil {
		return nil, fmt.Errorf("review %q does not exist%s", reviewID, knownReviews(document))
	}
	if review.Deck == nil {
		return nil, fmt.Errorf("review %q has no deck record", reviewID)
	}
	if review.Merged != nil {
		return nil, fmt.Errorf("review %q is history: its change landed as %s", reviewID, shortOID(review.Merged.Landed))
	}
	return review, nil
}

// findReviewSlide finds a slide of a review by id, URN, or record path.
func findReviewSlide(review *saga.Review, value string) *saga.Slide {
	for _, slide := range review.Deck.Slides {
		if value == slide.ID || value == slide.Target || strings.TrimSuffix(value, "/") == slide.Path {
			return slide
		}
	}
	return nil
}

func relativeToSaga(root, path string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return relativePathForOutput(root, path)
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}
