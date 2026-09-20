package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/areas"
)

// CheckSchema names the machine-readable `check --json` contract.
const CheckSchema = "change-saga.check/v1"

// checkExitUncovered is check's exit code when a named area has a gap. It
// differs from 1, which means the report could not be produced or trusted.
const checkExitUncovered = 3

// checkDocument is the `check --json` contract: the scope, whether every
// named area is fully covered, and those areas alone, in the order asked.
type checkDocument struct {
	Schema  string       `json:"check_schema"`
	Scope   areas.Scope  `json:"scope"`
	Covered bool         `json:"covered"`
	Areas   []areas.Area `json:"areas"`
}

// Check answers one question: are the named areas fully covered in scope?
// It exits 0 when they are, and 3 with only those areas' gaps when they are
// not. Nothing is required unless someone asks, so an area not named is
// never consulted. It exits 1, like status, when the report cannot be
// trusted.
func Check(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("check", commandUsage["check"], out)
	covers := flags.String("covers", "", "comma-separated coverage areas: implementation, stories, personas, design, quality, health")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	maxItems := flags.Int("max", 100, "maximum gaps per area in text mode; 0 means all")
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	opening := registerOpenFlags(flags)
	allowRepositoryMismatch := flags.Bool("allow-repository-mismatch", false, "use a checkout whose origin differs from the declared repository")
	feature := flags.String("feature", "", "narrow the question to one feature (id or URN)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["check"])
	}
	if *covers == "" {
		return fmt.Errorf("--covers is required: name the areas to ask about, for example --covers implementation,stories")
	}
	names, err := areas.Parse(*covers)
	if err != nil {
		return err
	}
	status, err := buildStatus(ctx, flags.Arg(0), *repoDir, opening.rng(), *allowRepositoryMismatch, *feature)
	if err != nil {
		return err
	}
	if err := status.trustworthy(flags.Arg(0)); err != nil {
		return err
	}
	document := checkDocument{Schema: CheckSchema, Scope: status.Coverage.Scope, Covered: true, Areas: []areas.Area{}}
	for _, name := range names {
		area := status.Coverage.Get(name)
		document.Areas = append(document.Areas, area)
		document.Covered = document.Covered && area.Complete
	}
	if *jsonOutput {
		if err := writeJSON(out, document); err != nil {
			return err
		}
	} else {
		printCheck(out, document, *maxItems)
	}
	if !document.Covered {
		return &StatusError{Code: checkExitUncovered}
	}
	return nil
}

func printCheck(out io.Writer, document checkDocument, maxItems int) {
	verdict := "not fully covered"
	if document.Covered {
		verdict = "fully covered"
	}
	fmt.Fprintf(out, "Asked of %s: %s\n", describeScope(document.Scope), verdict)
	for _, area := range document.Areas {
		printAreaLine(out, area)
		if !area.Complete {
			printGaps(out, area, maxItems)
		}
	}
}
