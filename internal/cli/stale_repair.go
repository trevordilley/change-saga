package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// staleRow is one stale reference with where its diff proposes it now is.
// Accept is the one-line command that takes the proposal, present only for
// coverage evidence with a proposal: claims and quality evidence are
// append-only, and a term is revised through term revise.
type staleRow struct {
	ownedReference
	Pinned   coderef.Location     `json:"pinned"`
	Reason   string               `json:"reason"`
	Proposal coderesolve.Proposal `json:"proposal"`
	Accept   *grammar.Invocation  `json:"accept,omitempty"`
}

// changeStaleness is what a change did to the Saga's references: those it
// made stale, each with a proposal, and a count of those already stale
// before it. A reference is made stale by the change when it is stale at the
// head and either current at the merge-base or pinned at a commit the
// merge-base does not contain (evidence written during the change).
type changeStaleness struct {
	BaseOID string `json:"base_oid"`
	HeadOID string `json:"head_oid"`
	// Count is how many references the change made stale.
	Count int `json:"count"`
	// Proposed is how many of them have a proposed range to accept with
	// repin --accept-proposed.
	Proposed int `json:"proposed"`
	// DeletedSide is how many stale references were written during the
	// change, pinned at the merge-base to lines the change removed: evidence
	// of what the change took out, current at the base, and not something to
	// repair. Living documentation that already described lines the change
	// deleted is the change's regression and is counted above instead.
	DeletedSide int `json:"deleted_side"`
	// PreExisting is how many other references were already stale. It is
	// absent where only the files the change touched were read.
	PreExisting *int       `json:"pre_existing,omitempty"`
	References  []staleRow `json:"references"`
}

// historicalOwners names quality evidence a later record superseded: its
// staleness is history, not debt.
func historicalOwners(document *saga.Saga) map[string][]string {
	historical := map[string][]string{}
	qualityDoc, err := quality.Load(document.Root)
	if err != nil {
		return historical
	}
	for _, test := range qualityDoc.TestCases {
		heads := map[string]bool{}
		for _, head := range test.EvidenceHeads {
			heads[head] = true
		}
		for _, evidence := range test.Evidence {
			owner, _ := qualityid.Evidence(document.Manifest.ID, test.Identity.ID, evidence.ID)
			if !heads[owner] {
				historical[owner] = []string{"evidence was superseded"}
			}
		}
	}
	return historical
}

// measureChangeStaleness classifies every stale reference at head against
// base. root and repo shape the accept commands. written reports evidence
// recorded during the change (writtenDuringChange); only such evidence of
// lines the change removed is deleted-side, and nil treats none as such. With
// changedOnly it reads only references into files base..head touches, the
// only ones the change can make stale, and leaves the pre-existing count out.
func measureChangeStaleness(ctx context.Context, resolver *coderesolve.Resolver, owned []ownedReference, historical map[string][]string, base, head, root, repo string, written func(ownedReference) bool, changedOnly ...bool) changeStaleness {
	result := changeStaleness{BaseOID: base, HeadOID: head, References: []staleRow{}}
	var changed map[string]bool
	if len(changedOnly) > 0 && changedOnly[0] {
		changed, _ = resolver.ChangedPaths(ctx, base, head)
		if changed == nil {
			changed = map[string]bool{}
		}
	}
	preExisting := 0
	if changed == nil {
		result.PreExisting = &preExisting
	}
	inBase := map[string]bool{}
	pinnedBefore := func(commit string) bool {
		known, ok := inBase[commit]
		if !ok {
			exists, _ := resolver.CommitExists(ctx, commit)
			known = !exists || commit == base || isAncestor(ctx, resolver.Repository(), commit, base)
			inBase[commit] = known
		}
		return known
	}
	for _, ref := range owned {
		if len(historical[ref.Owner]) > 0 || changed != nil && !changed[ref.Code.Path] {
			continue
		}
		resolution := resolver.Resolve(ctx, ref.Code, head)
		if resolution.Current() {
			continue
		}
		if ref.Code.Commit == base && base != head && resolver.Removed(ctx, base, head, ref.Code.Path, ref.Code.Start, ref.Code.End) && written != nil && written(ref) {
			result.DeletedSide++
			continue
		}
		byChange := base != "" && base != head && (!pinnedBefore(ref.Code.Commit) || resolver.Resolve(ctx, ref.Code, base).Current())
		if !byChange {
			preExisting++
			continue
		}
		row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: resolution.Reason, Proposal: resolver.Propose(ctx, ref.Code, head)}
		if row.Proposal.Proposed() {
			row.Accept = acceptInvocation(ref, head, root, repo)
		}
		if row.Accept != nil {
			result.Proposed++
		}
		result.Count++
		result.References = append(result.References, row)
	}
	return result
}

// withoutDiffs keeps the rows' proposals but drops their diffs, for reports
// that carry the signal alongside much else; references --stale and
// reconcile show the diffs.
func (staleness changeStaleness) withoutDiffs() changeStaleness {
	rows := make([]staleRow, len(staleness.References))
	for index, row := range staleness.References {
		row.Proposal.Diff = nil
		rows[index] = row
	}
	staleness.References = rows
	return staleness
}

// acceptInvocation is the one-line accept for one evidence reference.
func acceptInvocation(ref ownedReference, head, root, repo string) *grammar.Invocation {
	if ref.Kind != "evidence" || ref.EvidenceFile == "" {
		return nil
	}
	values := []grammar.Value{grammar.V("accept-proposed", "true"), grammar.V("record", ref.EvidenceFile), grammar.V("reference", strconv.Itoa(ref.Index))}
	if head != "" {
		values = append(values, grammar.V("head", head))
	}
	if repo != "" {
		values = append(values, grammar.V("repo", repo))
	}
	invocation := grammar.MustInvoke("repin", root, values...)
	return &invocation
}

// shortLocation spells a location without its commit: path#Lstart-Lend.
func shortLocation(location coderef.Location) string {
	spelled := location.String()
	if _, rest, ok := strings.Cut(spelled, ":"); ok {
		return rest
	}
	return spelled
}

// describeProposal is "old -> proposed" or why nothing is proposed.
func describeProposal(pinned coderef.Location, proposal coderesolve.Proposal) string {
	if !proposal.Proposed() {
		return shortLocation(pinned) + " -> nothing proposed: " + proposal.Reason
	}
	text := shortLocation(pinned) + " -> " + shortLocation(*proposal.Location)
	if proposal.Widened {
		text += " (widened)"
	}
	return text
}

// printChangeStaleness leads a report with what the change made stale.
func printChangeStaleness(out io.Writer, staleness changeStaleness, preExistingHint string) {
	switch staleness.Count {
	case 0:
		fmt.Fprintln(out, "Your change made no references stale.")
	case 1:
		fmt.Fprintln(out, "Your change made 1 reference stale:")
	default:
		fmt.Fprintf(out, "Your change made %d references stale:\n", staleness.Count)
	}
	for _, row := range staleness.References {
		fmt.Fprintf(out, "  %s  %s", row.Owner, describeProposal(row.Pinned, row.Proposal))
		switch row.Kind {
		case "inventory":
			fmt.Fprint(out, " (revise the definition with focused current references)")
		case "term":
			fmt.Fprint(out, " (revise the term with change-saga term revise)")
		case "claim", "quality_evidence":
			fmt.Fprint(out, " (append-only: record a new one)")
		}
		fmt.Fprintln(out)
	}
	if staleness.Proposed > 0 {
		fmt.Fprintf(out, "  %d with a proposed range: read its diff, then accept with change-saga repin --accept-proposed --record FILE [--reference N]\n", staleness.Proposed)
	}
	if staleness.DeletedSide > 0 {
		fmt.Fprintf(out, "Deleted-side evidence: %d %s document lines this change removed (current at the merge-base; nothing to repair)\n", staleness.DeletedSide, plural(staleness.DeletedSide, "reference", "references"))
	}
	if staleness.PreExisting != nil && *staleness.PreExisting > 0 {
		fmt.Fprintf(out, "Pre-existing stale references: %d (stale before this change%s)\n", *staleness.PreExisting, preExistingHint)
	}
}

// reviewReferences lists an open review's Item evidence like sagaReferences.
func reviewReferences(review *saga.Review) []ownedReference {
	var result []ownedReference
	if review.Deck == nil {
		return result
	}
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for _, file := range item.Code {
				for index, reference := range file.References {
					result = append(result, ownedReference{Kind: "evidence", Owner: item.Target, EvidenceFile: filepath.ToSlash(file.Path), Index: index + 1, Code: reference})
				}
			}
		}
	}
	return result
}

// acceptScope is what repin --accept-proposed repairs.
type acceptScope struct {
	record, target, review string
	reference              int
	all                    bool
}

func (scope acceptScope) includes(ref ownedReference, review string) bool {
	switch {
	case scope.record != "":
		return ref.EvidenceFile == filepath.ToSlash(filepath.Clean(scope.record)) && (scope.reference == 0 || ref.Index == scope.reference)
	case scope.target != "":
		return ref.Owner == scope.target || strings.HasPrefix(ref.Owner, scope.target+":")
	case scope.review != "":
		return review == scope.review
	}
	return scope.all
}

type acceptChange struct {
	Kind         string           `json:"kind"`
	Owner        string           `json:"owner"`
	EvidenceFile string           `json:"evidence_file"`
	Reference    int              `json:"reference"`
	From         coderef.Location `json:"from"`
	To           coderef.Location `json:"to"`
	Widened      bool             `json:"widened,omitempty"`
	Diff         []string         `json:"diff,omitempty"`
	replacement  coderef.Reference
	previous     coderef.Reference
}

type acceptRefusal struct {
	Kind         string           `json:"kind"`
	Owner        string           `json:"owner"`
	EvidenceFile string           `json:"evidence_file,omitempty"`
	Reference    int              `json:"reference"`
	Pinned       coderef.Location `json:"pinned"`
	Reason       string           `json:"reason"`
}

type acceptSkip struct {
	Review string `json:"review"`
	Reason string `json:"reason"`
}

type acceptOutput struct {
	OK       bool            `json:"ok"`
	DryRun   bool            `json:"dry_run"`
	Accepted []acceptChange  `json:"accepted"`
	Refused  []acceptRefusal `json:"refused"`
	// Current counts references in scope that needed nothing.
	Current int `json:"current"`
	// Skipped are open reviews left alone because their range cannot be
	// read; their Items were not repaired.
	Skipped []acceptSkip `json:"skipped,omitempty"`
	// Slides are the complete-slide transactions that carried accepted
	// evidence of slides apply-slide manages.
	Slides []SlideTransactionResult `json:"slide_transactions"`
}

// acceptProposed is repin --accept-proposed.
func acceptProposed(ctx context.Context, root, repoDir, headRev string, scope acceptScope, dryRun, jsonOutput bool, out io.Writer) error {
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	checkout := firstNonEmpty(repoDir, document.Root)
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return err
	}
	defer resolver.Close()
	head, err := resolveCommit(ctx, checkout, firstNonEmpty(headRev, "HEAD"))
	if err != nil {
		return err
	}
	type candidate struct {
		ref        ownedReference
		review     string
		base, head string
	}
	var candidates []candidate
	skipped := []acceptSkip{}
	if scope.review == "" {
		owned, err := sagaReferences(document)
		if err != nil {
			return err
		}
		for _, ref := range owned {
			candidates = append(candidates, candidate{ref: ref, head: head})
		}
	}
	if scope.review != "" && document.FindReview(scope.review) == nil {
		return fmt.Errorf("review %q does not exist%s", scope.review, knownReviews(document))
	}
	for _, review := range document.Reviews {
		if scope.review != "" && review.ID != scope.review {
			continue
		}
		refs := reviewReferences(review)
		if len(refs) == 0 || (scope.review == "" && !scope.all && !anyIncluded(scope, refs, review.ID)) {
			continue
		}
		if review.Merged != nil {
			if scope.review != "" || scope.record != "" {
				return fmt.Errorf("review %q is history: its evidence cannot be repaired after landing", review.ID)
			}
			continue
		}
		rng, err := reviewstate.ResolveRange(ctx, checkout, review)
		if err != nil {
			if scope.review != "" {
				return fmt.Errorf("read review %s's range: %w", review.ID, err)
			}
			// One review whose range cannot be read (its branch gone, its
			// base unknown) does not stop the repair of the others.
			skipped = append(skipped, acceptSkip{Review: review.ID, Reason: err.Error()})
			continue
		}
		for _, ref := range refs {
			candidates = append(candidates, candidate{ref: ref, review: review.ID, base: rng.BaseOID, head: rng.HeadOID})
		}
	}
	result := acceptOutput{OK: true, DryRun: dryRun, Accepted: []acceptChange{}, Refused: []acceptRefusal{}, Skipped: skipped, Slides: []SlideTransactionResult{}}
	matched := 0
	for _, value := range candidates {
		if !scope.includes(value.ref, value.review) {
			continue
		}
		matched++
		view := value.head
		if value.review != "" {
			view = reviewView(ctx, resolver, value.ref.Code, value.base, value.head)
		}
		if resolver.Resolve(ctx, value.ref.Code, view).Current() {
			result.Current++
			continue
		}
		refuse := func(reason string) {
			result.Refused = append(result.Refused, acceptRefusal{Kind: value.ref.Kind, Owner: value.ref.Owner, EvidenceFile: value.ref.EvidenceFile, Reference: value.ref.Index, Pinned: value.ref.Code.Location(), Reason: reason})
		}
		switch value.ref.Kind {
		case "evidence":
		case "term":
			refuse("a term's code is revised with change-saga term revise; its proposal is " + describeProposal(value.ref.Code.Location(), resolver.Propose(ctx, value.ref.Code, view)))
			continue
		default:
			refuse(value.ref.Kind + " records are append-only; record a new one rather than repinning")
			continue
		}
		proposal := resolver.Propose(ctx, value.ref.Code, view)
		if !proposal.Proposed() {
			refuse("nothing proposed: " + proposal.Reason)
			continue
		}
		replacement, err := resolver.Author(ctx, *proposal.Location, value.ref.Code.Note)
		if err != nil {
			refuse(err.Error())
			continue
		}
		result.Accepted = append(result.Accepted, acceptChange{
			Kind: value.ref.Kind, Owner: value.ref.Owner, EvidenceFile: value.ref.EvidenceFile, Reference: value.ref.Index,
			From: value.ref.Code.Location(), To: replacement.Location(), Widened: proposal.Widened, Diff: proposal.Diff,
			replacement: replacement, previous: value.ref.Code,
		})
	}
	if matched == 0 {
		if len(skipped) > 0 {
			return fmt.Errorf("no evidence reference matches the scope among the reviews whose range reads; skipped %s: %s", skipped[0].Review, skipped[0].Reason)
		}
		return fmt.Errorf("no evidence reference matches the scope; list them with change-saga references --stale")
	}
	if scope.record != "" && scope.reference != 0 && len(result.Refused) > 0 {
		refused := result.Refused[0]
		return fmt.Errorf("%s #%d has no proposal to accept: %s", refused.EvidenceFile, refused.Reference, refused.Reason)
	}
	if len(result.Accepted) > 0 {
		slides, err := writeAcceptedEvidence(ctx, root, repoDir, result.Accepted, dryRun)
		if err != nil {
			return err
		}
		result.Slides = slides
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Accepted"
	if dryRun {
		verb = "Would accept"
	}
	fmt.Fprintf(out, "%s %d proposed %s (%d in scope already current, %d refused)\n", verb, len(result.Accepted), plural(len(result.Accepted), "range", "ranges"), result.Current, len(result.Refused))
	for _, change := range result.Accepted {
		widened := ""
		if change.Widened {
			widened = " (widened)"
		}
		fmt.Fprintf(out, "  %s #%d %s -> %s%s\n", change.EvidenceFile, change.Reference, shortLocation(change.From), shortLocation(change.To), widened)
		for _, line := range change.Diff {
			fmt.Fprintf(out, "      %s\n", line)
		}
	}
	for _, refused := range result.Refused {
		fmt.Fprintf(out, "  refused %s #%d %s: %s\n", firstNonEmpty(refused.EvidenceFile, refused.Owner), refused.Reference, shortLocation(refused.Pinned), refused.Reason)
	}
	for _, skip := range result.Skipped {
		fmt.Fprintf(out, "  skipped review %s: its range cannot be read (%s)\n", skip.Review, skip.Reason)
	}
	for _, slide := range result.Slides {
		fmt.Fprintf(out, "  slide %s: snapshot %s\n", slide.Target, slide.Snapshot)
	}
	return nil
}

func anyIncluded(scope acceptScope, refs []ownedReference, review string) bool {
	for _, ref := range refs {
		if scope.includes(ref, review) {
			return true
		}
	}
	return false
}

// managedEvidence parses a complete-slide evidence_file: the slide record,
// the Item, and the 0-based evidence index.
func managedEvidence(file string) (string, string, int, bool) {
	record, rest, ok := strings.Cut(file, "#items/")
	if !ok {
		return "", "", 0, false
	}
	item, index, ok := strings.Cut(rest, "/evidence/")
	if !ok {
		return "", "", 0, false
	}
	number, err := strconv.Atoi(index)
	if err != nil || number < 0 {
		return "", "", 0, false
	}
	return record, item, number, true
}

// evidenceEdit is one change to a recorded evidence file: Reference (1-based)
// is replaced when previous still matches, or, when 0, replacement is
// appended to the file.
type evidenceEdit struct {
	EvidenceFile string
	Reference    int
	previous     coderef.Reference
	replacement  coderef.Reference
}

// writeAcceptedEvidence writes accepted references.
func writeAcceptedEvidence(ctx context.Context, root, repo string, accepted []acceptChange, dryRun bool) ([]SlideTransactionResult, error) {
	edits := make([]evidenceEdit, 0, len(accepted))
	for _, change := range accepted {
		edits = append(edits, evidenceEdit{EvidenceFile: change.EvidenceFile, Reference: change.Reference, previous: change.previous, replacement: change.replacement})
	}
	return writeEvidenceEdits(ctx, root, repo, edits, "accept-proposed", dryRun)
}

// writeEvidenceEdits writes edits. A plain evidence file is rewritten in
// place, keeping its path (so its owner) and every other reference; a slide
// apply-slide manages gets one complete-slide update built from its current
// revision with only the edited evidence changed. Every edit is resolved and
// validated before any is written, so a failure leaves the Saga untouched. A
// dry run writes nothing and still validates each slide update.
func writeEvidenceEdits(ctx context.Context, root, repo string, edits []evidenceEdit, requestPrefix string, dryRun bool) ([]SlideTransactionResult, error) {
	prepared, results, err := prepareEvidenceEdits(ctx, root, repo, edits, requestPrefix)
	if err != nil || dryRun {
		return results, err
	}
	if len(prepared.plain) > 0 {
		if err := authorMutation(root, prepared.writePlain); err != nil {
			return nil, err
		}
	}
	return prepared.writeManaged(ctx)
}

// preparedEvidence is a set of evidence edits every one of which has been
// resolved and validated: plain files by applying their edits in memory,
// managed slides by a dry run of their complete-slide update.
type preparedEvidence struct {
	root, repo string
	plain      map[string][]evidenceEdit
	managed    []preparedSlide
}

type preparedSlide struct {
	target  string
	request SlideTransactionRequest
}

// applyEvidenceEdits applies fileEdits to references, refusing when a
// replaced reference is no longer the one the edit was made from.
func applyEvidenceEdits(label string, references []coderef.Reference, fileEdits []evidenceEdit) ([]coderef.Reference, error) {
	for _, edit := range fileEdits {
		if edit.Reference == 0 {
			references = append(references, edit.replacement)
			continue
		}
		if edit.Reference > len(references) || references[edit.Reference-1].Key() != edit.previous.Key() {
			return nil, fmt.Errorf("%s changed while its evidence was being repaired; run the command again", label)
		}
		references[edit.Reference-1] = edit.replacement
	}
	return references, nil
}

// prepareEvidenceEdits resolves and validates edits without writing: each
// plain file is read and edited in memory, and each managed slide's update is
// built and checked with a dry run, whose results it returns.
func prepareEvidenceEdits(ctx context.Context, root, repo string, edits []evidenceEdit, requestPrefix string) (*preparedEvidence, []SlideTransactionResult, error) {
	prepared := &preparedEvidence{root: root, repo: repo, plain: map[string][]evidenceEdit{}}
	managed := map[string][]evidenceEdit{}
	for _, edit := range edits {
		if record, _, _, ok := managedEvidence(edit.EvidenceFile); ok {
			managed[record] = append(managed[record], edit)
		} else {
			prepared.plain[edit.EvidenceFile] = append(prepared.plain[edit.EvidenceFile], edit)
		}
	}
	if _, err := prepared.editPlain(root); err != nil {
		return nil, nil, err
	}
	results := []SlideTransactionResult{}
	if len(managed) == 0 {
		return prepared, results, nil
	}
	document, _, err := saga.Load(root)
	if err != nil {
		return nil, nil, err
	}
	base, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	for _, record := range sortedKeys(managed) {
		slide, err := findManagedSlide(document, record)
		if err != nil {
			return nil, nil, err
		}
		if slide == nil {
			return nil, nil, fmt.Errorf("no slide is recorded at %s", record)
		}
		request, err := currentSlideRequest(document, slide)
		if err != nil {
			return nil, nil, err
		}
		fingerprint := sha256.New()
		fmt.Fprint(fingerprint, request.ExpectedSnapshot)
		byEvidence := map[string][]evidenceEdit{}
		for _, edit := range managed[record] {
			byEvidence[edit.EvidenceFile] = append(byEvidence[edit.EvidenceFile], edit)
			fmt.Fprint(fingerprint, edit.EvidenceFile, edit.Reference, edit.replacement.Key())
		}
		for _, file := range sortedKeys(byEvidence) {
			_, itemID, evidenceIndex, _ := managedEvidence(file)
			found := false
			for itemIndex := range request.Items {
				item := &request.Items[itemIndex]
				if item.ID != itemID || evidenceIndex >= len(item.Evidence) {
					continue
				}
				references, err := applyEvidenceEdits(file, item.Evidence[evidenceIndex].References, byEvidence[file])
				if err != nil {
					return nil, nil, err
				}
				item.Evidence[evidenceIndex].References = references
				found = true
			}
			if !found {
				return nil, nil, fmt.Errorf("%s changed while its evidence was being repaired; run the command again", file)
			}
		}
		request.RequestID = requestPrefix + "-" + hex.EncodeToString(fingerprint.Sum(nil))[:16]
		result, err := ApplySlideTransaction(ctx, root, base, repo, request, true)
		if err != nil {
			return nil, nil, fmt.Errorf("update slide %s: %w", slide.Target, err)
		}
		results = append(results, result)
		prepared.managed = append(prepared.managed, preparedSlide{target: slide.Target, request: request})
	}
	return prepared, results, nil
}

// editPlain reads every plain evidence file under sagaRoot and applies its
// edits in memory, writing nothing.
func (prepared *preparedEvidence) editPlain(sagaRoot string) (map[string]saga.CodeFile, error) {
	files := map[string]saga.CodeFile{}
	for _, relative := range sortedKeys(prepared.plain) {
		var file saga.CodeFile
		if err := readStrictJSONFile(filepath.Join(sagaRoot, filepath.FromSlash(relative)), &file); err != nil {
			return nil, fmt.Errorf("read %s: %w", relative, err)
		}
		references, err := applyEvidenceEdits(relative, file.References, prepared.plain[relative])
		if err != nil {
			return nil, err
		}
		file.References = references
		files[relative] = file
	}
	return files, nil
}

// writePlain writes the plain evidence files under the Saga lock. It edits
// every file again from what is on disk before writing the first one, so a
// file changed since preparation refuses the whole write.
func (prepared *preparedEvidence) writePlain(locked *saga.Saga) error {
	files, err := prepared.editPlain(locked.Root)
	if err != nil {
		return err
	}
	for _, relative := range sortedKeys(files) {
		if err := store.WriteJSON(filepath.Join(locked.Root, filepath.FromSlash(relative)), files[relative], false); err != nil {
			return err
		}
	}
	return nil
}

// writeManaged publishes the prepared complete-slide updates.
func (prepared *preparedEvidence) writeManaged(ctx context.Context) ([]SlideTransactionResult, error) {
	results := []SlideTransactionResult{}
	if len(prepared.managed) == 0 {
		return results, nil
	}
	base, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, slide := range prepared.managed {
		result, err := ApplySlideTransaction(ctx, prepared.root, base, prepared.repo, slide.request, false)
		if err != nil {
			return nil, fmt.Errorf("update slide %s: %w", slide.target, err)
		}
		results = append(results, result)
	}
	return results, nil
}

// findManagedSlide finds the slide recorded at a path, or named by its
// target or ID, in any deck, a review's included; nil when none is. A bare
// ID that slides of several decks share is ambiguous: the error names each
// one's target to use instead.
func findManagedSlide(document *saga.Saga, value string) (*saga.Slide, error) {
	decks := allDecks(document)
	for _, review := range document.Reviews {
		if review.Deck != nil {
			decks = append(decks, review.Deck)
		}
	}
	var byID []*saga.Slide
	for _, deck := range decks {
		for _, slide := range deck.Slides {
			if slide.Target == value || filepath.Clean(slide.Path) == filepath.Clean(value) {
				return slide, nil
			}
			if slide.ID == value {
				byID = append(byID, slide)
			}
		}
	}
	if len(byID) <= 1 {
		if len(byID) == 1 {
			return byID[0], nil
		}
		return nil, nil
	}
	targets := make([]string, 0, len(byID))
	for _, slide := range byID {
		targets = append(targets, slide.Target)
	}
	return nil, fmt.Errorf("slide id %q names %d slides in different decks; name one by its target: %s", value, len(byID), strings.Join(targets, ", "))
}

// currentSlideRequest rebuilds the complete apply-slide request that
// republishes a managed slide's current revision unchanged: its diagram
// source, or its asset inline as base64, every Item with its evidence,
// criterion links, and documentation links. It is an update expecting the
// current snapshot; the caller supplies a fresh request_id.
func currentSlideRequest(document *saga.Saga, slide *saga.Slide) (SlideTransactionRequest, error) {
	if !transactionManagedSlide(slide) {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s is not managed by apply-slide", slide.Target)
	}
	if slide.AuthoringConflict || len(slide.AuthoringHeads) > 1 {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s has divergent heads %s; reconcile them with apply-slide first", slide.Target, strings.Join(slide.AuthoringHeads, ", "))
	}
	var deck *saga.Deck
	for _, candidate := range allDecks(document) {
		for _, owned := range candidate.Slides {
			if owned == slide {
				deck = candidate
			}
		}
	}
	for _, review := range document.Reviews {
		if review.Deck != nil {
			for _, owned := range review.Deck.Slides {
				if owned == slide {
					deck = review.Deck
				}
			}
		}
	}
	if deck == nil {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s belongs to no deck", slide.Target)
	}
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(document.Root, filepath.FromSlash(slide.Path)), &record); err != nil {
		return SlideTransactionRequest{}, err
	}
	revision, err := record.CurrentRevision()
	if err != nil {
		return SlideTransactionRequest{}, err
	}
	value := revision.Slide
	request := SlideTransactionRequest{
		Version: saga.SlideTransactionVersion, Operation: "update", Deck: deck.ID, ExpectedSnapshot: revision.Snapshot,
		Slide: SlideTransactionSlide{
			ID: value.ID, Title: value.Title, Rank: value.Rank, Section: value.Section, Intent: value.Intent, Layout: value.Layout,
			MediaType: value.MediaType, Takeaway: value.Takeaway, ReadingOrder: append([]string{}, value.ReadingOrder...), ExceptionRationale: value.ExceptionRationale,
		},
		Items: []SlideTransactionItemRequest{},
	}
	// A review deck's slide is republished into its review, which names the
	// deck; its Items keep their record links.
	for _, review := range document.Reviews {
		if review.Deck == deck {
			request.Deck, request.Review = "", review.ID
		}
	}
	if revision.Diagram != nil {
		data, err := os.ReadFile(filepath.Join(deck.Directory, revision.Diagram.Source))
		if err != nil {
			return SlideTransactionRequest{}, err
		}
		source, err := diagram.Decode(data)
		if err != nil {
			return SlideTransactionRequest{}, fmt.Errorf("diagram source %s: %w", revision.Diagram.Source, err)
		}
		request.Diagram = &source
	} else {
		data, err := os.ReadFile(filepath.Join(deck.Directory, revision.Asset))
		if err != nil {
			return SlideTransactionRequest{}, err
		}
		request.Asset = SlideTransactionAsset{ContentBase64: base64.StdEncoding.EncodeToString(data)}
	}
	for _, item := range revision.Items {
		manifest := item.Item
		evidence := make([]saga.CodeFile, 0, len(item.Evidence))
		for _, file := range item.Evidence {
			evidence = append(evidence, saga.CodeFile{Version: file.Version, References: append([]coderef.Reference{}, file.References...)})
		}
		request.Items = append(request.Items, SlideTransactionItemRequest{
			Documentation: manifest.Documentation, DocumentationView: manifest.DocumentationView, Selections: append([]saga.ItemSelection(nil), manifest.Selections...),
			ID: manifest.ID, Rank: manifest.Rank, Kind: manifest.Kind, Label: manifest.Label, Description: manifest.Description,
			Selector: manifest.Selector, Hotspot: manifest.Hotspot, About: manifest.About, Body: manifest.Body, Placement: manifest.Placement, Leader: manifest.Leader,
			Record: manifest.Record, Evidence: evidence, CriterionLinks: append([]saga.CriterionLink{}, item.CriterionLinks...),
		})
	}
	return request, nil
}

// inventoryReferences lists the code references of every active technical
// definition's current revision, except evidence of proposed intent, which
// asserts no implementation. A stale one is repaired by revising the
// definition, so it has no one-line accept.
func inventoryReferences(document *saga.Saga) []ownedReference {
	inventory, err := requirements.LoadInventory(document.Root, document.Manifest.ID)
	if err != nil {
		return nil
	}
	var result []ownedReference
	for _, record := range inventory.Records {
		if record.CurrentRevision == nil || record.CurrentLifecycle == nil || record.CurrentLifecycle.State == "retired" {
			continue
		}
		for index, owned := range inventoryview.Evidence(record.CurrentRevision) {
			if owned.Intent == technicalpolicy.Proposed {
				continue
			}
			result = append(result, ownedReference{Kind: "inventory", Owner: record.Target + owned.Suffix(), Index: index + 1, Code: owned.Evidence.Reference})
		}
	}
	return result
}

// documentationReferences is every living reference the headline counts:
// sagaReferences plus technical definitions' evidence.
func documentationReferences(document *saga.Saga) ([]ownedReference, error) {
	owned, err := sagaReferences(document)
	if err != nil {
		return nil, err
	}
	return append(owned, inventoryReferences(document)...), nil
}

// livingStaleness measures what base..head did to the living documentation's
// references, or nil when the Saga has none.
func livingStaleness(ctx context.Context, document *saga.Saga, resolver *coderesolve.Resolver, base, head, root, repo string, changedOnly ...bool) *changeStaleness {
	owned, err := documentationReferences(document)
	if err != nil || len(owned) == 0 || resolver == nil {
		return nil
	}
	written := writtenDuringChange(ctx, document.Root, resolver.Repository(), base, head)
	staleness := measureChangeStaleness(ctx, resolver, owned, historicalOwners(document), base, head, root, repo, written, changedOnly...).withoutDiffs()
	return &staleness
}

// writtenDuringChange reports whether a living reference was recorded during
// the change base..head: the Saga that documented the merge-base (the base
// commit's own Saga, or a companion Saga's through its sync cursor) holds no
// identical reference for the same owner. It reads that Saga once, on first
// use. When no snapshot documents the base it answers no, so a deletion is
// listed for repair rather than excused; when the Saga did not exist yet,
// every reference was written during the change.
func writtenDuringChange(ctx context.Context, root, checkout, base, head string) func(ownedReference) bool {
	var once sync.Once
	var recorded map[string]bool
	absent := false
	key := func(ref ownedReference) string {
		return ref.Owner + "\x00" + ref.EvidenceFile + "\x00" + ref.Code.Key()
	}
	return func(ref ownedReference) bool {
		once.Do(func() {
			side, _ := changeview.BaseSide(ctx, root, checkout, gitdiff.ChangeSet{BaseOID: base, HeadOID: head})
			if side.Source != changeview.SideGit {
				return
			}
			err := changeview.ReadSnapshot(ctx, root, side, func(snapshot string) error {
				document, _, err := saga.Load(snapshot)
				if err != nil {
					return err
				}
				owned, err := documentationReferences(document)
				if err != nil {
					return err
				}
				recorded = map[string]bool{}
				for _, prior := range owned {
					recorded[key(prior)] = true
				}
				return nil
			})
			absent = changeview.IsSagaAbsent(err)
		})
		return absent || recorded != nil && !recorded[key(ref)]
	}
}

// reviewView is the commit a review Item's reference is read at: the
// merge-base for deleted-side evidence, which is pinned at or before it, and
// the head for everything else. A reference keeps its side. New-side
// evidence stale at the head needs judgment even when the merge-base still
// has its lines; reading it there would quietly turn it into deleted-side
// evidence.
func reviewView(ctx context.Context, resolver *coderesolve.Resolver, code coderef.Reference, base, head string) string {
	if code.Commit == base || isAncestor(ctx, resolver.Repository(), code.Commit, base) {
		return base
	}
	return head
}

// reviewItemRows lists a review's stale Item references with proposals at
// the side each lives on (reviewView).
func reviewItemRows(ctx context.Context, resolver *coderesolve.Resolver, review *saga.Review, base, head, root, repo string) []staleRow {
	rows := []staleRow{}
	for _, ref := range reviewReferences(review) {
		view := reviewView(ctx, resolver, ref.Code, base, head)
		atView := resolver.Resolve(ctx, ref.Code, view)
		if atView.Current() {
			continue
		}
		row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: atView.Reason, Proposal: resolver.Propose(ctx, ref.Code, view)}
		if row.Proposal.Proposed() {
			row.Accept = acceptInvocation(ref, "", root, repo)
		}
		rows = append(rows, row)
	}
	return rows
}
