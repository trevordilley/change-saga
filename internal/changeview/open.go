package changeview

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// OpenOptions is a Saga opened to compare one change.
type OpenOptions struct {
	SagaRoot string
	Document *saga.Saga
	// Checkout is the code repository checkout the comparison was read from.
	Checkout string
	Changes  gitdiff.ChangeSet
	Report   coverage.Report
	Resolver coverage.Resolver
}

// Open derives the layers of a comparison. It decides which Saga snapshots to
// compare: when the Saga lives in the code repository, the Saga at the
// merge-base and the Saga at head (the working tree when head is the commit
// checked out, so uncommitted authoring shows). A companion Saga is read at
// the Saga commits whose sync cursor matched each side.
func Open(ctx context.Context, options OpenOptions) (Layers, *Inventory, error) {
	location, locateErr := Locate(ctx, options.SagaRoot)
	compute := Options{
		SagaRoot: options.SagaRoot, Document: options.Document, Changes: options.Changes,
		Report: options.Report, Resolver: options.Resolver, Location: location,
	}
	var diagnostics []Diagnostic
	switch {
	case locateErr != nil:
		diagnostics = append(diagnostics, Diagnostic{Code: "saga_history_unavailable", Message: locateErr.Error() + "; every record is treated as added"})
	case sameRepository(ctx, location.Repo, options.Checkout):
		compute.Base = options.Changes.BaseOID
		if checkedOut, _ := revParse(ctx, location.Repo, "HEAD"); checkedOut != options.Changes.HeadOID {
			compute.Head = options.Changes.HeadOID
		}
	default:
		sides, err := companionSides(ctx, location, options.Checkout, options.Changes)
		if err != nil {
			return Layers{}, nil, err
		}
		compute.Base, compute.Head, diagnostics = sides.base, sides.head, sides.diagnostics
	}
	layers, inventory, err := Compute(ctx, compute)
	if err != nil {
		return Layers{}, nil, err
	}
	layers.Diagnostics = append(diagnostics, layers.Diagnostics...)
	return layers, inventory, nil
}

func sameRepository(ctx context.Context, sagaRepo, checkout string) bool {
	output, err := exec.CommandContext(ctx, "git", "-C", checkout, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return false
	}
	codeRepo := strings.TrimSpace(string(output))
	if resolved, err := filepath.EvalSymlinks(codeRepo); err == nil {
		codeRepo = resolved
	}
	return filepath.Clean(codeRepo) == filepath.Clean(sagaRepo)
}

func revParse(ctx context.Context, repo, revision string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", revision+"^{commit}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
