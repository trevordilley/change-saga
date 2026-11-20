package testfixture

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// Fixtures that exercise historical pins commit documentation, just as an
// author does. There are no fabricated Saga revision IDs or fixture archives.
func commitFixtureDocumentation(ctx context.Context, root, label string) error {
	run := func(args ...string) error {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test"}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %v: %w: %s", args, err, out)
		}
		return nil
	}
	if exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-toplevel").Run() != nil {
		if out, err := exec.CommandContext(ctx, "git", "-C", filepath.Dir(root), "init", "--quiet").CombinedOutput(); err != nil {
			return fmt.Errorf("initialize fixture documentation repository: %w: %s", err, out)
		}
	}
	if err := run("add", "."); err != nil {
		return err
	}
	return run("commit", "--quiet", "--allow-empty", "-m", "Document "+label)
}
