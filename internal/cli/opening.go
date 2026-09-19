package cli

import (
	"flag"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// Help text for the flags that choose how a Saga is opened. The comparison
// is never stored in the Saga: every command that reads one takes it here.
const (
	againstUsage = "compare: view what --head changes since its merge-base with this revision, the way a pull request does; omit to observe --head"
	headUsage    = "the commit to observe, or the head of the comparison"
)

// openFlags are --against and --head.
type openFlags struct {
	against, head *string
}

func registerOpenFlags(flags *flag.FlagSet) *openFlags {
	return &openFlags{
		against: flags.String("against", "", againstUsage),
		head:    flags.String("head", "HEAD", headUsage),
	}
}

func (value *openFlags) rng() gitdiff.Range {
	return gitdiff.Range{Against: *value.against, Head: *value.head}
}
