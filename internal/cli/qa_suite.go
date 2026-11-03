package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

var e2eSuiteOperations = []string{"set", "show", "clear"}

// E2ESuite dispatches the commands that say where end-to-end tests live.
func E2ESuite(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("e2e-suite", e2eSuiteOperations, out)
	}
	operation := "e2e-suite " + args[0]
	var err error
	switch args[0] {
	case "set":
		err = e2eSuiteSet(args[1:], out)
	case "show":
		err = e2eSuiteShow(ctx, args[1:], out)
	case "clear":
		err = e2eSuiteClear(args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["e2e-suite"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func e2eSuiteSet(args []string, out io.Writer) error {
	name := "e2e-suite set"
	flags := commandFlags(name, commandUsage[name], out)
	var paths stringList
	flags.Var(&paths, "path", "repository path holding end-to-end tests; replaces the current paths")
	command := flags.String("command", "", "command that runs the tests named by "+saga.QASuiteLocations+"; empty removes it")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	commandGiven := false
	flags.Visit(func(given *flag.Flag) { commandGiven = commandGiven || given.Name == "command" })
	if len(paths) == 0 && !commandGiven {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	root := flags.Arg(0)
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, validation, err := saga.Load(root)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("cannot write into a structurally invalid Saga; run change-saga validate")
		}
		suite := saga.QASuite{Schema: saga.QASuiteSchema, Version: saga.SagaVersion}
		if document.QASuite != nil {
			suite = *document.QASuite
		}
		if len(paths) > 0 {
			suite.Paths = paths
		}
		if commandGiven {
			suite.RunCommand = *command
		}
		if err := saga.ValidateQASuite(suite); err != nil {
			return err
		}
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(document.Root, saga.QADir))
		if err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(dir, saga.QASuiteName), suite, false)
	})
	if err != nil {
		return err
	}
	return writeE2ESuiteChange(out, name, *jsonOutput)
}

func e2eSuiteClear(args []string, out io.Writer) error {
	name := "e2e-suite clear"
	flags := commandFlags(name, commandUsage[name], out)
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	root := flags.Arg(0)
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		dir := filepath.Join(root, saga.QADir)
		if err := os.Remove(filepath.Join(dir, saga.QASuiteName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return writeE2ESuiteChange(out, name, *jsonOutput)
}

func writeE2ESuiteChange(out io.Writer, operation string, jsonOutput bool) error {
	if jsonOutput {
		return writeLivingMutation(out, operation, "", saga.QASuitePath(), nil, nil, false, true)
	}
	fmt.Fprintf(out, "Updated %s\n", saga.QASuitePath())
	return nil
}

// e2eSuiteReport is what e2e-suite show prints. Declared is false for a Saga that
// has not said, which is fine: some apps have no end-to-end tests.
type e2eSuiteReport struct {
	Declared   bool     `json:"declared"`
	Paths      []string `json:"paths"`
	RunCommand string   `json:"run_command,omitempty"`
	// MissingPaths are declared paths the checkout does not hold. They may simply
	// not have been written yet, so this fails nothing.
	MissingPaths []string `json:"missing_paths"`
}

func e2eSuiteShow(ctx context.Context, args []string, out io.Writer) error {
	name := "e2e-suite show"
	flags := commandFlags(name, commandUsage[name], out)
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	root := flags.Arg(0)
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	report := e2eSuiteReport{Paths: []string{}, MissingPaths: []string{}}
	if suite := document.QASuite; suite != nil {
		report.Declared, report.Paths, report.RunCommand = true, suite.Paths, suite.RunCommand
		top, err := gitexec.TopLevel(ctx, firstNonEmpty(*repo, root))
		if err != nil {
			return err
		}
		for _, path := range suite.Paths {
			if _, err := os.Stat(filepath.Join(top, filepath.FromSlash(path))); errors.Is(err, os.ErrNotExist) {
				report.MissingPaths = append(report.MissingPaths, path)
			}
		}
	}
	if *jsonOutput {
		return writeJSON(out, report)
	}
	if !report.Declared {
		fmt.Fprintln(out, "No end-to-end test locations are recorded, which is fine for an app without them.")
		fmt.Fprintln(out, "To record them: change-saga e2e-suite set --path PATH --command TEMPLATE")
		return nil
	}
	for _, path := range report.Paths {
		fmt.Fprintf(out, "path: %s\n", path)
	}
	if report.RunCommand != "" {
		fmt.Fprintf(out, "command: %s\n", report.RunCommand)
	}
	for _, path := range report.MissingPaths {
		fmt.Fprintf(out, "not found in this checkout: %s\n", path)
	}
	return nil
}
