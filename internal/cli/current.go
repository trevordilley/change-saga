package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func init() {
	commandUsage["normalize"] = "change-saga normalize [--json] <saga>"
	commandDescription["normalize"] = "Replace unambiguous legacy content graphs with editable current records; preserve evidence pins and let Git own history."
	for _, command := range grammar.Commands() {
		commandUsage[command.Name] = command.Usage
		if command.Mutates {
			for _, family := range []string{"story", "criterion", "persona", "flag", "term", "component", "system", "data-entity", "erd", "erd-overlay", "prototype", "deprecation", "whiteboard", "quality test-case"} {
				if strings.HasPrefix(command.Name, family+" ") {
					commandDescription[command.Name] = command.Summary
					break
				}
			}
		}
	}
}

// Normalize is optional: normal authoring also normalizes encountered legacy
// data under the Saga lock. Ordinary reads never persist the conversion.
func Normalize(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("normalize", commandUsage["normalize"], out)
	jsonOutput := flags.Bool("json", false, "machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["normalize"])
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		_, validation, err := saga.Load(root)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("legacy Saga is invalid; correct it before normalization: %+v", validation.Issues)
		}
		if _, err := requirements.Load(root, manifest.ID); err != nil {
			return err
		}
		if _, err := requirements.LoadInventory(root, manifest.ID); err != nil {
			return err
		}
		if _, err := quality.Load(root); err != nil {
			return err
		}
		if _, err := prototypes.Load(root, manifest.ID); err != nil {
			return err
		}
		return currentrecord.Normalize(root)
	}); err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, map[string]any{"ok": true, "operation": "normalize", "history": "git"})
	}
	_, err = fmt.Fprintln(out, "Current documentation normalized. Git owns its history.")
	return err
}
