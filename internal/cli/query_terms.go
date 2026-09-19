package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// termsQuery is the data of "query terms": the project's vocabulary, each
// term with its stories, records, and code health at the head.
type termsQuery struct {
	Head  string                 `json:"head_oid"`
	Ref   string                 `json:"ref,omitempty"`
	Terms []livingapp.TermStatus `json:"terms"`
}

// queryTerms answers "query terms". Its filters run the links both ways:
// --term names one term, --story finds the terms a story belongs to, and
// --ref finds the terms whose code contains a code location at any commit,
// so a line of code reaches the vocabulary it defines.
func queryTerms(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query terms", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sagaRoot := flags.String("saga", "", "saga root")
	sourceDir := flags.String("repo", "", "source repository checkout")
	opening := registerOpenFlags(flags)
	term := flags.String("term", "", "term ID or URN")
	story := flags.String("story", "", "story ID or URN")
	ref := flags.String("ref", "", "code location <commit>:<path>[#L<start>[-L<end>]]; the commit may be any revision")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeQuerySuccess(out, "", queryHelpFor("terms"), nil)
		}
		return writeQueryOperationFailure(out, "terms", &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *sagaRoot == "" || flags.NArg() != 0 {
		return writeQueryOperationFailure(out, "terms", &queryError{Code: "invalid_argument", Message: "usage: " + queryUsage["terms"]})
	}
	manifest, err := saga.ReadManifest(*sagaRoot)
	if err != nil {
		return writeQueryOperationFailure(out, "terms", normalizeQueryError(err))
	}
	document, err := requirements.Load(*sagaRoot, manifest.ID)
	if err != nil {
		return writeQueryOperationFailure(out, "terms", &queryError{Code: "invalid_saga", Message: err.Error()})
	}
	checkout := firstNonEmpty(*sourceDir, *sagaRoot)
	changes, err := gitdiff.ReadRange(ctx, checkout, manifest.Source.Repository, opening.rng(), gitdiff.ReadOptions{})
	if err != nil {
		return writeQueryOperationFailure(out, "terms", &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return writeQueryOperationFailure(out, "terms", &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	defer resolver.Close()

	selected := document.Terms
	if *term != "" {
		id := strings.TrimPrefix(*term, "urn:change-saga:"+manifest.ID+":term:")
		selected = nil
		if found := document.FindTerm(id); found != nil {
			selected = []requirements.Term{*found}
		}
		if selected == nil {
			return writeQueryOperationFailure(out, "terms", &queryError{Code: "not_found", Message: "term " + *term + " does not exist"})
		}
	}
	if *story != "" {
		urn := *story
		if livingid.ValidID(urn) {
			urn, _ = livingid.Story(manifest.ID, urn)
		}
		selected = filterTerms(selected, func(revision *requirements.TermRevision) bool { return slices.Contains(revision.Stories, urn) })
	}
	result := termsQuery{Head: changes.HeadOID}
	if *ref != "" {
		location, err := resolveLocation(ctx, checkout, *ref)
		if err != nil {
			return writeQueryOperationFailure(out, "terms", &queryError{Code: "invalid_argument", Message: "--ref: " + err.Error()})
		}
		result.Ref = location.String()
		selected = filterTerms(selected, func(revision *requirements.TermRevision) bool {
			for _, reference := range revision.Code {
				at := resolver.Resolve(ctx, reference, location.Commit)
				if at.Current() && at.Location.Path == location.Path && overlaps(at.Location, location) {
					return true
				}
			}
			return false
		})
	}
	result.Terms = livingapp.TermStatuses(ctx, manifest.ID, selected, changes, resolver)
	return writeQuerySuccess(out, "", result, nil)
}

func filterTerms(terms []requirements.Term, keep func(*requirements.TermRevision) bool) []requirements.Term {
	result := []requirements.Term{}
	for _, term := range terms {
		if term.CurrentRevision != nil && keep(term.CurrentRevision) {
			result = append(result, term)
		}
	}
	return result
}

// overlaps reports whether two locations in one file share a line; a whole
// file overlaps everything in it.
func overlaps(left, right coderef.Location) bool {
	if left.WholeFile() || right.WholeFile() {
		return true
	}
	return left.Start <= right.End && right.Start <= left.End
}

// resolveLocation parses a code location whose commit may be any revision.
func resolveLocation(ctx context.Context, checkout, value string) (coderef.Location, error) {
	if location, err := coderef.ParseLocation(value); err == nil {
		return location, nil
	}
	revision, rest, found := strings.Cut(value, ":")
	if !found || revision == "" {
		return coderef.Location{}, errors.New("a code location is <commit>:<path>[#L<start>[-L<end>]]")
	}
	commit, err := resolveCommit(ctx, checkout, revision)
	if err != nil {
		return coderef.Location{}, err
	}
	return coderef.ParseLocation(commit + ":" + rest)
}
