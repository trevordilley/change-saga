package cli

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
)

// queryLayers answers "query layers": the Changed, Affected, and Code layers
// of one comparison. Layers exist only in compare mode, so --against is
// required.
func queryLayers(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query layers", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sagaRoot := flags.String("saga", "", "saga root")
	sourceDir := flags.String("repo", "", "source repository checkout")
	opening := registerOpenFlags(flags)
	layer := flags.String("layer", "", "changed, affected, or code; all three when omitted")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeQuerySuccess(out, "", queryHelpFor("layers"), nil)
		}
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	switch {
	case *sagaRoot == "" || flags.NArg() != 0:
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "invalid_argument", Message: "usage: " + queryUsage["layers"]})
	case *opening.against == "":
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "invalid_argument", Message: "layers belong to a comparison: pass --against REV"})
	case *layer != "" && *layer != "changed" && *layer != "affected" && *layer != "code":
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "invalid_argument", Message: "--layer must be changed, affected, or code"})
	}
	value, err := readComparison(ctx, *sagaRoot, *sourceDir, opening.rng(), false)
	if err != nil {
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	resolver, err := coderesolve.New(ctx, value.checkout)
	if err != nil {
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true})
	}
	defer resolver.Close()
	layers, _, err := changeview.Open(ctx, changeview.OpenOptions{
		SagaRoot: *sagaRoot, Document: value.document, Checkout: value.checkout,
		Changes: value.changes, Report: value.report, Resolver: resolver,
	})
	if err != nil {
		return writeQueryOperationFailure(out, "layers", &queryError{Code: "internal", Message: err.Error()})
	}
	empty := changeview.CodeLayer{Groups: []changeview.CodeGroup{}, Unreferenced: []changeview.Hunk{}}
	switch *layer {
	case "changed":
		layers.Affected, layers.Code = []changeview.Affected{}, empty
	case "affected":
		layers.Changed, layers.Code = []changeview.Change{}, empty
	case "code":
		layers.Changed, layers.Affected = []changeview.Change{}, []changeview.Affected{}
	}
	return writeQuerySuccess(out, "", layers, nil)
}

// queryHistory answers "query history": when a record was introduced, what
// it replaced, and every commit that changed it, each with the command that
// opens the comparison where it happened. It reads Git's log of the record's
// files, so it needs no comparison.
func queryHistory(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sagaRoot := flags.String("saga", "", "saga root")
	node := flags.String("node", "", "record URN")
	if err := flags.Parse(args); err != nil || *sagaRoot == "" || *node == "" || flags.NArg() != 0 {
		return writeQueryOperationFailure(out, "history", &queryError{Code: "invalid_argument", Message: "usage: " + queryUsage["history"]})
	}
	history, err := changeview.NodeHistory(ctx, *sagaRoot, *node)
	if err != nil {
		return writeQueryOperationFailure(out, "history", &queryError{Code: "not_found", Message: err.Error()})
	}
	return writeQuerySuccess(out, "", history, nil)
}
