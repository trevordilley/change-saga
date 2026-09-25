package inventoryview

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// Source reads code at a commit and views references there.
type Source interface {
	Resolver
	Blob(ctx context.Context, commit, path string) ([]byte, bool, error)
}

const (
	MaxCoverageFiles = 20000
	MaxCoverageLines = 5_000_000
)

// CoverageInput names the measured scope: the files at Head the caller
// selected, and the path prefixes that selected them.
type CoverageInput struct {
	Head  string
	Paths []string
	Files []string
	// Kinds, when non-empty, restricts which technical owners are measured.
	Kinds []string
}

// CoverageOwner is one inventory reference accounting for code.
type CoverageOwner struct {
	Owner     string `json:"owner"` // target, or target#edge for owned edges
	Revision  string `json:"revision"`
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

type CoverageRange struct {
	Path   string          `json:"path"`
	Start  int             `json:"start"`
	End    int             `json:"end"`
	State  string          `json:"state"` // covered | uncovered
	Owners []CoverageOwner `json:"owners"`
}

type StaleEvidence struct {
	CoverageOwner
	Resolution coderesolve.Resolution `json:"resolution"`
}

type ExcludedOwner struct {
	Target  string   `json:"target"`
	Kind    string   `json:"kind"`
	Reasons []string `json:"reasons"`
}

type CoverageSummary struct {
	Files        int `json:"files"`
	BinaryFiles  int `json:"binary_files"`
	Lines        int `json:"lines"`
	Covered      int `json:"covered_lines"`
	Uncovered    int `json:"uncovered_lines"`
	Overlapping  int `json:"overlapping_lines"`
	References   int `json:"references"`
	Current      int `json:"current_references"`
	Stale        int `json:"stale_references"`
	OutsideScope int `json:"current_references_outside_scope"`
	Unresolved   int `json:"unresolved_owners"`
	Excluded     int `json:"excluded_owners"`
	// ProposedEdgeReferences are references on explicitly proposed
	// interactions or relationships; they account for nothing.
	ProposedEdgeReferences int `json:"proposed_edge_references"`
	CoveredRanges          int `json:"covered_ranges"`
	UncoveredRange         int `json:"uncovered_ranges"`
}

// CoverageReport is an omission check of which code in a named scope the
// inventory's current definitions account for. It is never a verdict.
type CoverageReport struct {
	Head       string          `json:"head_oid"`
	Paths      []string        `json:"paths"`
	Summary    CoverageSummary `json:"summary"`
	Ranges     []CoverageRange `json:"-"`
	Stale      []StaleEvidence `json:"-"`
	Unresolved []ExcludedOwner `json:"-"`
	Excluded   []ExcludedOwner `json:"-"`
}

// Exclusion reasons for owners that account for no code.
const (
	ExcludedRetired  = "retired"
	ExcludedProposed = "proposed_intent"
)

// Coverage measures inventory code coverage at Head. Only the unique current
// revision of an active record counts; conflicted records are unresolved and
// retired or proposed revisions are excluded, each with a reason, as is the
// evidence of explicitly proposed interactions and relationships. Stale
// references account for nothing. Every line counts once however many owners
// reference it; all owners are retained on the range.
func Coverage(ctx context.Context, inventory *requirements.Inventory, in CoverageInput, source Source) (CoverageReport, error) {
	report := CoverageReport{Head: in.Head, Paths: append([]string{}, in.Paths...), Ranges: []CoverageRange{}, Stale: []StaleEvidence{}, Unresolved: []ExcludedOwner{}, Excluded: []ExcludedOwner{}}
	if !coderef.ValidCommit(in.Head) {
		return report, fmt.Errorf("coverage requires a resolved head commit")
	}
	if len(in.Files) > MaxCoverageFiles {
		return report, fmt.Errorf("scope has %d files; the limit is %d: narrow it with path prefixes", len(in.Files), MaxCoverageFiles)
	}
	kinds := map[string]bool{}
	for _, k := range in.Kinds {
		kinds[k] = true
	}
	files := map[string][][]CoverageOwner{}
	order := append([]string{}, in.Files...)
	sort.Strings(order)
	for _, path := range order {
		content, found, err := source.Blob(ctx, in.Head, path)
		if err != nil {
			return report, err
		}
		if !found {
			continue
		}
		if bytes.IndexByte(content, 0) >= 0 {
			report.Summary.BinaryFiles++
			continue
		}
		n := len(coderef.Lines(content))
		report.Summary.Files++
		report.Summary.Lines += n
		if report.Summary.Lines > MaxCoverageLines {
			return report, fmt.Errorf("scope exceeds %d lines: narrow it with path prefixes", MaxCoverageLines)
		}
		files[path] = make([][]CoverageOwner, n)
	}
	if inventory != nil {
		for _, r := range inventory.Records {
			if len(kinds) > 0 && !kinds[r.Kind] {
				continue
			}
			if r.CurrentRevision == nil || r.CurrentLifecycle == nil {
				reasons := []string{}
				if r.CurrentRevision == nil {
					reasons = append(reasons, UnresolvedRevisionHeads)
				}
				if r.CurrentLifecycle == nil {
					reasons = append(reasons, UnresolvedLifecycleHeads)
				}
				report.Unresolved = append(report.Unresolved, ExcludedOwner{r.Target, r.Kind, reasons})
				continue
			}
			if r.CurrentLifecycle.State == "retired" {
				report.Excluded = append(report.Excluded, ExcludedOwner{r.Target, r.Kind, []string{ExcludedRetired}})
				continue
			}
			if RevisionIntent(r.CurrentRevision) == technicalpolicy.Proposed {
				report.Excluded = append(report.Excluded, ExcludedOwner{r.Target, r.Kind, []string{ExcludedProposed}})
				continue
			}
			revision := r.Target + ":revision:" + r.CurrentRevision.ID
			visit := func(owner string, ref coderef.Reference) {
				o := CoverageOwner{Owner: owner, Revision: revision, Kind: r.Kind, Reference: ref.Location().String()}
				report.Summary.References++
				resolution := source.Resolve(ctx, ref, in.Head)
				if !resolution.Current() {
					report.Summary.Stale++
					report.Stale = append(report.Stale, StaleEvidence{o, resolution})
					return
				}
				report.Summary.Current++
				lines, ok := files[resolution.Location.Path]
				if !ok {
					report.Summary.OutsideScope++
					return
				}
				start, end := resolution.Location.Start, resolution.Location.End
				if resolution.Location.WholeFile() {
					start, end = 1, len(lines)
				}
				for line := max(start, 1); line <= min(end, len(lines)); line++ {
					lines[line-1] = append(lines[line-1], o)
				}
			}
			for _, owned := range Evidence(r.CurrentRevision) {
				if owned.Intent == technicalpolicy.Proposed {
					// A proposed edge's evidence is not implemented coverage.
					report.Summary.ProposedEdgeReferences++
					continue
				}
				visit(r.Target+owned.Suffix(), owned.Evidence.Reference)
			}
		}
	}
	report.Summary.Unresolved, report.Summary.Excluded = len(report.Unresolved), len(report.Excluded)
	for _, path := range order {
		lines, ok := files[path]
		if !ok {
			continue
		}
		var current *CoverageRange
		currentKey := ""
		for i, owners := range lines {
			key := ownersKey(owners)
			if len(owners) > 0 {
				report.Summary.Covered++
				if len(owners) > 1 {
					report.Summary.Overlapping++
				}
			} else {
				report.Summary.Uncovered++
			}
			if current != nil && key == currentKey {
				current.End = i + 1
				continue
			}
			if current != nil {
				report.Ranges = append(report.Ranges, *current)
			}
			state := "uncovered"
			if len(owners) > 0 {
				state = "covered"
			}
			sorted := append([]CoverageOwner{}, owners...)
			sort.Slice(sorted, func(a, b int) bool { return ownerKey(sorted[a]) < ownerKey(sorted[b]) })
			current, currentKey = &CoverageRange{Path: path, Start: i + 1, End: i + 1, State: state, Owners: sorted}, key
		}
		if current != nil {
			report.Ranges = append(report.Ranges, *current)
		}
	}
	for _, r := range report.Ranges {
		if r.State == "covered" {
			report.Summary.CoveredRanges++
		} else {
			report.Summary.UncoveredRange++
		}
	}
	return report, nil
}

func ownerKey(o CoverageOwner) string { return o.Owner + "\x00" + o.Reference }

func ownersKey(owners []CoverageOwner) string {
	keys := make([]string, len(owners))
	for i, o := range owners {
		keys[i] = ownerKey(o)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x01")
}
