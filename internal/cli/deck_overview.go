package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// Deck dispatches deck-level authoring commands.
func Deck(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return livingFamilyHelp("deck", []string{"overview"}, out)
	}
	if args[0] != "overview" {
		return fmt.Errorf("usage: %s", commandUsage["deck"])
	}
	err := DeckOverview(ctx, args[1:], out)
	var status *StatusError
	if err != nil && !errors.As(err, &status) && jsonFlagRequested(args) {
		return reportJSONMutationFailure(out, err)
	}
	return err
}

// DeckOverview replaces only the optional authored report on a deck record.
func DeckOverview(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("deck overview", commandUsage["deck overview"], out)
	target := flags.String("deck", "", "existing deck ID, path, or stable URN")
	reviewID := flags.String("review", "", "author the overview of this review's deck instead of --deck")
	file := flags.String("file", "", "JSON object with body and optional annotations")
	check := flags.Bool("check", false, "check authored every-slide overview coverage without writing; exit 3 for gaps")
	dryRun := flags.Bool("dry-run", false, "validate without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result including coverage warnings")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || (*file == "") == !*check || (*check && *dryRun) || (*target == "") == (*reviewID == "") {
		return fmt.Errorf("usage: %s", commandUsage["deck overview"])
	}
	var overview saga.DeckOverview
	if *file != "" {
		if err := readStrictJSONPath(*file, &overview); err != nil {
			return fmt.Errorf("read deck overview: %w", err)
		}
	}
	var checked saga.DeckOverviewReport
	var result struct {
		OK         bool               `json:"ok"`
		DryRun     bool               `json:"dry_run"`
		Target     string             `json:"target"`
		Overview   *saga.DeckOverview `json:"overview"`
		Validation saga.Validation    `json:"validation"`
	}
	err := authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		deck := findDeck(document, *target)
		if *reviewID != "" {
			review := document.FindReview(*reviewID)
			var err error
			if !*check {
				review, err = findReviewDeck(document, *reviewID)
			}
			if err == nil && (review == nil || review.Deck == nil) {
				err = fmt.Errorf("review %q has no deck", *reviewID)
			}
			if err != nil {
				return err
			}
			deck = review.Deck
		} else if deck == nil {
			// Full review deck URNs are accepted without ambiguous review IDs.
			for _, review := range document.Reviews {
				if review.Deck != nil && review.Deck.Target == *target {
					if _, err := findReviewDeck(document, review.ID); !*check && err != nil {
						return err
					}
					deck = review.Deck
					break
				}
			}
		}
		if deck == nil {
			return fmt.Errorf("--deck must identify an existing deck")
		}
		if *check {
			checked = deck.OverviewReport()
			result.Target = deck.Target
			return nil
		}
		candidate := *deck
		candidate.Overview = &overview
		validation := saga.ValidateDeckOverview(&candidate)
		if !validation.Valid {
			var errors []string
			for _, issue := range validation.Issues {
				if issue.Severity == "error" {
					errors = append(errors, issue.Message)
				}
			}
			return fmt.Errorf("invalid deck overview: %s", strings.Join(errors, "; "))
		}
		result.OK, result.DryRun, result.Target, result.Overview, result.Validation = true, *dryRun, deck.Target, &overview, validation
		if *dryRun {
			return nil
		}
		return store.WriteJSON(filepath.Join(document.Root, filepath.FromSlash(deck.Path)), candidate.DeckManifest, false)
	})
	if err != nil {
		return err
	}
	if *check {
		if *jsonOutput {
			if err := writeJSON(out, struct {
				Target string `json:"target"`
				saga.DeckOverviewReport
			}{result.Target, checked}); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "Overview %s: %d covered, %d uncovered slides\n", result.Target, len(checked.CoveredSlides), len(checked.UncoveredSlides))
			for _, issue := range checked.Validation.Issues {
				fmt.Fprintf(out, "%s: %s\n", issue.Severity, issue.Message)
			}
		}
		if !checked.Validation.Valid {
			return &StatusError{Code: 1}
		}
		if !checked.Complete {
			return &StatusError{Code: 3}
		}
		return nil
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Updated"
	if *dryRun {
		verb = "Would update"
	}
	fmt.Fprintf(out, "%s overview for %s\n", verb, result.Target)
	for _, issue := range result.Validation.Issues {
		fmt.Fprintf(out, "%s: %s\n", issue.Severity, issue.Message)
	}
	return nil
}
