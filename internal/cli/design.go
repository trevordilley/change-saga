package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

// authoringScope selects a physical hierarchy root while leaving chapter,
// section, fragment, landmark, and target behavior in the shared machinery.
// Report content lives in an epic (its report root, or its ___design root for
// design authoring) or in one of the app-level report roots.
type authoringScope struct {
	design bool
	// epic and app are the parsed --epic and --app values.
	epic string
	app  string
}

var narrativeAuthoring = authoringScope{}
var designAuthoring = authoringScope{design: true}

// appReportRoots maps --app values to the app-level report roots.
var appReportRoots = map[string]string{
	"overview":     applayout.OverviewDir,
	"designsystem": applayout.DesignSystemDir,
}

const appFlagHelp = "author into an app-level report root instead of an epic: overview or designsystem"

// placeFlags registers the flags that choose where report content goes.
func (scope authoringScope) placeFlags(flags *flag.FlagSet) (*string, *string) {
	epic := epicFlag(flags)
	app := new(string)
	if !scope.design {
		app = flags.String("app", "", appFlagHelp)
	}
	return epic, app
}

func (scope authoringScope) placed(epic, app *string) (authoringScope, error) {
	scope.epic, scope.app = strings.TrimSpace(*epic), strings.TrimSpace(*app)
	if scope.app != "" {
		if _, ok := appReportRoots[scope.app]; !ok {
			return scope, fmt.Errorf("--app must be overview or designsystem")
		}
		if scope.epic != "" {
			return scope, fmt.Errorf("--app and --epic cannot be combined; app-level report content belongs to no epic")
		}
	}
	return scope, nil
}

func (scope authoringScope) command(operation string) string {
	if scope.design {
		return "design-" + operation
	}
	return operation
}

func (scope authoringScope) commandText(operation string) string {
	if scope.design {
		return "change-saga design " + operation
	}
	return "change-saga " + operation
}

// placeArguments renders the --epic or --app arguments a follow-up command
// needs to author into the same place.
func (scope authoringScope) placeArguments() string {
	if scope.app != "" {
		return "--app " + scope.app + " "
	}
	if scope.epic != "" {
		return "--epic " + scope.epic + " "
	}
	return ""
}

// hierarchyRoot is the directory new top-level report content is written to.
func (scope authoringScope) hierarchyRoot(document *saga.Saga) (string, error) {
	var dir string
	if scope.app != "" {
		dir = filepath.Join(document.Root, appReportRoots[scope.app])
	} else {
		epic, err := applayout.Require(document.Root, document.Manifest.ID, scope.epic)
		if err != nil {
			if scope.design {
				return "", err
			}
			return "", fmt.Errorf("%w; app-level report content uses --app overview|designsystem", err)
		}
		dir = epic.Dir
		if scope.design {
			dir = filepath.Join(epic.Dir, applayout.DesignDir)
		}
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return dir, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s must be a real directory", filepath.Base(dir))
	}
	return dir, nil
}

// resolveTarget resolves an existing chapter, section, or fragment. "." names
// the hierarchy root itself. A stable ID or URN resolves app-wide and then
// decides the place: --epic, when given, must agree with it. A relative path
// is relative to the hierarchy root.
func (scope authoringScope) resolveTarget(document *saga.Saga, value string, allowFragment bool) (string, string, error) {
	if value == "." || value == "" {
		root, err := scope.hierarchyRoot(document)
		return root, "", err
	}
	var dir, target string
	var err error
	if root, rootErr := scope.hierarchyRoot(document); rootErr == nil && !filepath.IsAbs(value) && !strings.HasPrefix(value, "urn:") {
		rel, _ := filepath.Rel(document.Root, root)
		dir, target, err = resolveTarget(document, filepath.Join(rel, value), allowFragment)
	}
	if target == "" {
		dir, target, err = resolveTarget(document, value, allowFragment)
	}
	if err != nil {
		return "", "", err
	}
	rel, _ := filepath.Rel(document.Root, dir)
	rel = filepath.ToSlash(rel)
	switch {
	case scope.app != "":
		if !pathWithin(filepath.Join(document.Root, appReportRoots[scope.app]), dir) {
			return "", "", fmt.Errorf("target %q is outside the app's %s root", value, scope.app)
		}
	case saga.EpicOf(rel) == "":
		return "", "", fmt.Errorf("target %q belongs to the app, not an epic; use --app overview|designsystem", value)
	case scope.epic != "":
		if err := assertEpic(document.Root, scope.epic, target, saga.EpicOf(rel)); err != nil {
			return "", "", err
		}
	}
	if scope.design && !saga.IsDesignPath(rel) {
		return "", "", fmt.Errorf("target %q is outside the technical design root", value)
	}
	if !scope.design && scope.app == "" && saga.IsDesignPath(rel) {
		return "", "", fmt.Errorf("target %q is technical design; use change-saga design", value)
	}
	return dir, target, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	if len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return false
	}
	return true
}

var designOperations = []string{"add-chapter", "add-section", "add-fragment", "set-fragment-content"}

// Design dispatches the technical-design authoring family. Each operation
// calls the same implementation used by root narrative authoring with a
// different physical hierarchy root.
func Design(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s", commandUsage["design"])
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printDesignHelp(out)
		return flag.ErrHelp
	}
	switch args[0] {
	case "add-chapter":
		return addChapter(ctx, args[1:], out, designAuthoring)
	case "add-section":
		return addSection(ctx, args[1:], out, designAuthoring)
	case "add-fragment":
		return addFragment(ctx, args[1:], out, designAuthoring)
	case "set-fragment-content":
		return setFragmentContentCommand(ctx, args[1:], out, os.Stdin, designAuthoring)
	default:
		return fmt.Errorf("unknown design operation %q; expected add-chapter, add-section, add-fragment, or set-fragment-content", args[0])
	}
}

func printDesignHelp(out io.Writer) {
	fmt.Fprintln(out, "Technical design authoring")
	if description := commandDescription["design"]; description != "" {
		fmt.Fprintf(out, "\n%s\n", description)
	}
	fmt.Fprintln(out, "\nUsage:")
	for _, operation := range designOperations {
		fmt.Fprintf(out, "  %s\n", commandUsage["design-"+operation])
	}
}
