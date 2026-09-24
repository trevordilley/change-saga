package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

// termsQuery is the data of "query terms": the project's vocabulary, each
// term with its stories, records, and code health at the head.
type termsQuery struct {
	Head  string                 `json:"head_oid"`
	Ref   string                 `json:"ref,omitempty"`
	Terms []livingapp.TermStatus `json:"terms"`
}

type termsCursorToken struct {
	Version  int    `json:"v"`
	Key      string `json:"key"`
	Snapshot string `json:"snapshot"`
	Offset   int    `json:"offset"`
	Checksum string `json:"checksum"`
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
	cursor := flags.String("cursor", "", "pagination cursor; use with --limit for bounded enumeration")
	var limit optionalInt
	flags.Var(&limit, "limit", "page size; omitted preserves the legacy complete response")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeQuerySuccess(out, "", queryHelpFor("terms"), nil)
		}
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *sagaRoot == "" || flags.NArg() != 0 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "usage: " + queryUsage["terms"]})
	}
	if limit.set && (limit.value < 1 || limit.value > maxQueryPageSize) {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: fmt.Sprintf("--limit must be between 1 and %d", maxQueryPageSize)})
	}
	if flags.Lookup("cursor") != nil && *cursor == "" {
		cursorSet := false
		flags.Visit(func(value *flag.Flag) { cursorSet = cursorSet || value.Name == "cursor" })
		if cursorSet {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--cursor cannot be empty"})
		}
	}
	manifest, err := saga.ReadManifest(*sagaRoot)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	document, err := requirements.Load(*sagaRoot, manifest.ID)
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "invalid_saga", Message: err.Error()})
	}
	checkout := firstNonEmpty(*sourceDir, *sagaRoot)
	changes, err := gitdiff.ReadRange(ctx, checkout, manifest.Source.Repository, opening.rng(), gitdiff.ReadOptions{})
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	defer resolver.Close()

	selected := document.Terms
	if *term != "" {
		id, selectorErr := exactTermID(manifest.ID, *term)
		if selectorErr != nil {
			return writeQueryFailure(out, selectorErr)
		}
		selected = nil
		if found := document.FindTerm(id); found != nil {
			selected = []requirements.Term{*found}
		}
		if selected == nil {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "term " + *term + " does not exist"})
		}
	}
	if *story != "" {
		urn, selectorErr := exactStoryURN(manifest.ID, *story)
		if selectorErr != nil {
			return writeQueryFailure(out, selectorErr)
		}
		storyRef, _ := livingid.Parse(urn)
		if document.FindStory(storyRef.ID) == nil {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "story " + *story + " does not exist", Details: map[string]any{"kind": "story", "selector": *story}})
		}
		selected = filterTerms(selected, func(revision *requirements.TermRevision) bool { return slices.Contains(revision.Stories, urn) })
	}
	result := termsQuery{Head: changes.HeadOID}
	if *ref != "" {
		location, err := resolveLocation(ctx, checkout, *ref)
		if err != nil {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--ref: " + err.Error()})
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
	snapshot, err := reviewapp.Snapshot(ctx, *sagaRoot, changes)
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "internal", Message: err.Error()})
	}
	keyBytes, _ := json.Marshal(struct {
		Term  string `json:"term,omitempty"`
		Story string `json:"story,omitempty"`
		Ref   string `json:"ref,omitempty"`
	}{Term: *term, Story: *story, Ref: result.Ref})
	start, end := 0, len(result.Terms)
	page := queryPageEnvelope{Total: len(result.Terms), Returned: len(result.Terms)}
	bounded := limit.set || *cursor != ""
	if bounded {
		pageLimit := limit.value
		if pageLimit == 0 {
			pageLimit = livingapp.DefaultPageLimit
		}
		var cursorErr *queryError
		start, cursorErr = decodeTermsCursor(*cursor, string(keyBytes), snapshot, len(result.Terms))
		if cursorErr != nil {
			return writeQueryFailure(out, cursorErr)
		}
		end = start + pageLimit
		if end > len(result.Terms) {
			end = len(result.Terms)
		}
		page.Returned, page.HasMore = end-start, end < len(result.Terms)
		if page.HasMore {
			next := encodeTermsCursor(string(keyBytes), snapshot, end)
			page.NextCursor = &next
		}
	}
	result.Terms = result.Terms[start:end]
	if !bounded {
		return writeQuerySuccess(out, snapshot, result, nil)
	}
	return writeQuerySuccess(out, snapshot, result, &page)
}

func exactTermID(sagaID, selector string) (string, *queryError) {
	if livingid.ValidID(selector) {
		return selector, nil
	}
	id, err := requirements.ParseTermURN(sagaID, selector)
	if err != nil {
		return "", &queryError{Code: "invalid_argument", Message: "--term must be a stable term ID or canonical URN in this Saga", Details: map[string]any{"selector": selector, "expected": "ID or urn:change-saga:" + sagaID + ":term:ID"}}
	}
	return id, nil
}

func exactStoryURN(sagaID, selector string) (string, *queryError) {
	if livingid.ValidID(selector) {
		urn, _ := livingid.Story(sagaID, selector)
		return urn, nil
	}
	ref, err := livingid.Parse(selector)
	if err != nil || ref.Kind != livingid.KindStory || ref.SagaID != sagaID {
		return "", &queryError{Code: "invalid_argument", Message: "--story must be a stable story ID or canonical URN in this Saga", Details: map[string]any{"selector": selector, "expected": "ID or urn:change-saga:" + sagaID + ":story:ID"}}
	}
	return selector, nil
}

func encodeTermsCursor(key, snapshot string, offset int) string {
	token := termsCursorToken{Version: 1, Key: key, Snapshot: snapshot, Offset: offset}
	token.Checksum = termsCursorChecksum(token)
	data, _ := json.Marshal(token)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeTermsCursor(cursor, key, snapshot string, total int) (int, *queryError) {
	if cursor == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, invalidTermsCursor()
	}
	var token termsCursorToken
	decodeErr := json.Unmarshal(data, &token)
	canonical, marshalErr := json.Marshal(token)
	if decodeErr != nil || marshalErr != nil || !bytes.Equal(data, canonical) || token.Version != 1 || token.Key != key || token.Offset < 0 || len(token.Checksum) != sha256.Size*2 ||
		subtle.ConstantTimeCompare([]byte(token.Checksum), []byte(termsCursorChecksum(token))) != 1 {
		return 0, invalidTermsCursor()
	}
	if token.Snapshot != snapshot {
		return 0, &queryError{Code: "stale_snapshot", Message: "the cursor belongs to a different snapshot", Retryable: true, Details: map[string]any{"expected": token.Snapshot, "actual": snapshot}}
	}
	if token.Offset > total {
		return 0, invalidTermsCursor()
	}
	return token.Offset, nil
}

func termsCursorChecksum(token termsCursorToken) string {
	token.Checksum = ""
	data, _ := json.Marshal(token)
	digest := sha256.Sum256(append([]byte("change-saga-terms-cursor-v1\x00"), data...))
	return hex.EncodeToString(digest[:])
}

func invalidTermsCursor() *queryError {
	return &queryError{Code: "invalid_argument", Message: "cursor does not apply to this query"}
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

// resolveLocation parses a code location whose commit may be any revision:
// every command that accepts a location resolves HEAD, a branch, or an
// abbreviated commit to the full commit it pins.
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
