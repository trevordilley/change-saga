package changeview

import (
	"context"
	"fmt"
	"os"
)

// ReadSnapshot visits exactly the Saga side selected by Open. Committed sides
// are disposable copies; the callback must not retain their paths or mutate
// the working side. An absent side has no document to visit.
func ReadSnapshot(ctx context.Context, root string, side SagaSide, visit func(string) error) error {
	if side.Source == SideWorking {
		return visit(root)
	}
	if side.Source != SideGit {
		return fmt.Errorf("cannot read Saga side %q", side.Source)
	}
	location, err := Locate(ctx, root)
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "change-saga-snapshot-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	snapshot, err := extract(ctx, location, side.Commit, temp)
	if err != nil {
		return err
	}
	return visit(snapshot)
}
