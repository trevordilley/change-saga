package cli

import (
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// DefaultSagaName is the directory init creates when it is given no path. The
// recommended idiom is one Saga per repository, named after the tool rather
// than an app so that it fits a monorepo of several apps as well as one.
const DefaultSagaName = "change.saga"

// repositoryName is the name a repository is known by: the last path segment
// of its recorded repository URI without a .git suffix, so both
// https://host/owner/name.git and git@host:owner/name.git give "name". When
// that yields no usable identifier, the checkout's top-level directory names
// it instead.
func repositoryName(repositoryURI, topLevel string) string {
	if parsed, err := url.Parse(repositoryURI); err == nil {
		name := strings.TrimSuffix(path.Base(strings.TrimRight(parsed.Path, "/")), ".git")
		if name != "" && name != "." && name != "/" && saga.ValidID(store.Slug(name)) {
			return name
		}
	}
	return filepath.Base(topLevel)
}

// printExistingSagaNote tells whoever runs init that the repository already
// had a Saga. It is guidance only: init has already succeeded.
func printExistingSagaNote(out io.Writer, existing []string) {
	fmt.Fprintln(out, "\nNote: this repository already contains a Saga:")
	for _, path := range existing {
		fmt.Fprintf(out, "  - %s\n", path)
	}
	fmt.Fprintf(out, `The recommended idiom is one Saga per repository (by default %s at its
root), with each app or product domain documented as a feature of it. The new
Saga was still created; remove it if you meant to extend the existing one.

`, DefaultSagaName)
}
