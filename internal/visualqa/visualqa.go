// Package visualqa runs repeatable browser rendering and mechanical visual
// checks against authored slide assets and the real Change Saga reviewer.
package visualqa

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
)

//go:embed runner.mjs
var runnerSource []byte

var StandardViewports = []Viewport{{Width: 1280, Height: 720}, {Width: 1024, Height: 576}}

type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Options struct {
	SagaRoot      string
	SourceDir     string
	OutputDir     string
	Feature       string
	Deck          string
	Slide         string
	PlaywrightDir string
}

type Artifact struct {
	Surface  string `json:"surface"`
	Viewport string `json:"viewport"`
	Path     string `json:"path"`
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Surface  string `json:"surface"`
	Viewport string `json:"viewport,omitempty"`
	Deck     string `json:"deck"`
	Slide    string `json:"slide"`
	Item     string `json:"item,omitempty"`
	Other    string `json:"other,omitempty"`
	Message  string `json:"message"`
}

type SlideReport struct {
	Target    string     `json:"target"`
	Feature   string     `json:"feature,omitempty"`
	Deck      string     `json:"deck"`
	Slide     string     `json:"slide"`
	Title     string     `json:"title"`
	Artifacts []Artifact `json:"artifacts"`
}

type Report struct {
	Version        int           `json:"version"`
	Saga           string        `json:"saga"`
	Selection      Selection     `json:"selection"`
	Viewports      []Viewport    `json:"viewports"`
	Slides         []SlideReport `json:"slides"`
	ContactSheet   string        `json:"contact_sheet"`
	Findings       []Finding     `json:"findings"`
	Passed         bool          `json:"passed"`
	SemanticArrows string        `json:"semantic_arrows"`
}

type Selection struct {
	Feature string `json:"feature,omitempty"`
	Deck    string `json:"deck,omitempty"`
	Slide   string `json:"slide,omitempty"`
}

type runnerInput struct {
	BaseURL        string        `json:"base_url"`
	OutputDir      string        `json:"output_dir"`
	PlaywrightDir  string        `json:"playwright_dir"`
	Saga           string        `json:"saga"`
	Selection      Selection     `json:"selection"`
	Viewports      []Viewport    `json:"viewports"`
	Slides         []runnerSlide `json:"slides"`
	SemanticArrows string        `json:"semantic_arrows"`
}

type runnerSlide struct {
	Target      string       `json:"target"`
	Feature     string       `json:"feature,omitempty"`
	Deck        string       `json:"deck"`
	Slide       string       `json:"slide"`
	Title       string       `json:"title"`
	RawURL      string       `json:"raw_url"`
	ReviewerURL string       `json:"reviewer_url"`
	Items       []*saga.Item `json:"items"`
}

// Run loads the Saga without mutation, renders into a sibling staging
// directory, stops its loopback server, and only then replaces a prior managed
// output directory.
func Run(ctx context.Context, options Options) (Report, error) {
	root, err := filepath.Abs(options.SagaRoot)
	if err != nil {
		return Report{}, err
	}
	document, validation, err := saga.LoadNarrative(root)
	if err != nil {
		return Report{}, err
	}
	if !validation.Valid {
		return Report{}, errors.New("saga is structurally invalid; run change-saga validate")
	}
	selected, err := selectSlides(document, options)
	if err != nil {
		return Report{}, err
	}
	output, err := safeOutputPath(root, document.Manifest.ID, options.OutputDir)
	if err != nil {
		return Report{}, err
	}
	playwright, err := findPlaywright(options.PlaywrightDir)
	if err != nil {
		return Report{}, err
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return Report{}, errors.New("visual QA requires Node.js; install the version declared by e2e/package.json")
	}

	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Report{}, fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".change-saga-visual-qa-stage-")
	if err != nil {
		return Report{}, fmt.Errorf("create output stage: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.WriteFile(filepath.Join(stage, ".change-saga-visual-qa"), []byte("managed output\n"), 0o644); err != nil {
		return Report{}, err
	}
	runner := filepath.Join(stage, "runner.mjs")
	if err := os.WriteFile(runner, runnerSource, 0o600); err != nil {
		return Report{}, err
	}

	serverCtx, stopServer := context.WithCancel(ctx)
	ready := make(chan string, 1)
	serverDone := make(chan error, 1)
	sourceDir := options.SourceDir
	if sourceDir == "" {
		sourceDir = root
	}
	go func() {
		serverDone <- reviewserver.ListenManaged(serverCtx, root, sourceDir, "127.0.0.1:0", false, &bytes.Buffer{}, reviewserver.ManagedOptions{OnReady: func(address string) error {
			ready <- address
			return nil
		}})
	}()
	var baseURL string
	select {
	case baseURL = <-ready:
	case serverErr := <-serverDone:
		stopServer()
		return Report{}, fmt.Errorf("start reviewer: %w", serverErr)
	case <-ctx.Done():
		stopServer()
		return Report{}, ctx.Err()
	case <-time.After(15 * time.Second):
		stopServer()
		return Report{}, errors.New("reviewer did not become ready within 15 seconds")
	}

	input := runnerInput{
		BaseURL: baseURL, OutputDir: stage, PlaywrightDir: playwright,
		Saga: document.Manifest.ID, Selection: Selection{Feature: options.Feature, Deck: options.Deck, Slide: options.Slide},
		Viewports: StandardViewports, Slides: selected, SemanticArrows: "not_evaluated",
	}
	inputPath := filepath.Join(stage, "input.json")
	encoded, err := json.Marshal(input)
	if err == nil {
		err = os.WriteFile(inputPath, encoded, 0o600)
	}
	if err != nil {
		stopServer()
		<-serverDone
		return Report{}, err
	}

	command := exec.CommandContext(ctx, node, runner, inputPath)
	var commandOutput bytes.Buffer
	command.Stdout, command.Stderr = &commandOutput, &commandOutput
	runErr := command.Run()
	stopServer()
	serverErr := <-serverDone
	if runErr != nil {
		return Report{}, fmt.Errorf("Playwright render failed: %w\n%s", runErr, strings.TrimSpace(commandOutput.String()))
	}
	if serverErr != nil && !errors.Is(serverErr, context.Canceled) {
		return Report{}, fmt.Errorf("stop reviewer: %w", serverErr)
	}

	reportBytes, err := os.ReadFile(filepath.Join(stage, "visual-qa.json"))
	if err != nil {
		return Report{}, fmt.Errorf("read visual QA report: %w", err)
	}
	var report Report
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		return Report{}, fmt.Errorf("decode visual QA report: %w", err)
	}
	if err := os.Remove(runner); err != nil {
		return Report{}, fmt.Errorf("remove staged runner: %w", err)
	}
	if err := os.Remove(inputPath); err != nil {
		return Report{}, fmt.Errorf("remove staged input: %w", err)
	}
	if err := installOutput(stage, output); err != nil {
		return Report{}, err
	}
	return report, nil
}

func selectSlides(document *saga.Saga, options Options) ([]runnerSlide, error) {
	var selected []runnerSlide
	appendDeck := func(feature string, deck *saga.Deck) {
		if options.Deck != "" && options.Deck != deck.ID && options.Deck != deck.Target {
			return
		}
		for _, slide := range deck.Slides {
			if options.Slide != "" && options.Slide != slide.ID && options.Slide != slide.Target {
				continue
			}
			reviewerPath := "/"
			if feature != "" {
				reviewerPath = "/features/" + url.PathEscape(feature)
			}
			selected = append(selected, runnerSlide{
				Target: slide.Target, Feature: feature, Deck: deck.ID, Slide: slide.ID, Title: slide.Title,
				RawURL:      "/f/" + url.PathEscape(slide.ID) + "/" + url.PathEscape(slide.Entrypoint),
				ReviewerURL: reviewerPath + "?view=slides", Items: slide.Items,
			})
		}
	}
	if options.Feature == "" || options.Feature == "onboarding" {
		for _, deck := range document.Onboarding {
			appendDeck("", deck)
		}
	}
	for _, feature := range document.Features {
		if options.Feature != "" && options.Feature != feature.ID && options.Feature != feature.Target {
			continue
		}
		for _, deck := range feature.Decks {
			appendDeck(feature.ID, deck)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no slides matched the requested feature, deck, and slide selection")
	}
	return selected, nil
}

func safeOutputPath(sagaRoot, sagaID, requested string) (string, error) {
	if requested == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		requested = filepath.Join(cwd, "change-saga-visual-qa", sagaID)
	}
	output, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	output = filepath.Clean(output)
	volumeRoot := filepath.VolumeName(output) + string(filepath.Separator)
	if output == volumeRoot {
		return "", errors.New("visual QA output cannot be a filesystem root")
	}
	if home, _ := os.UserHomeDir(); home != "" && samePath(output, home) {
		return "", errors.New("visual QA output cannot replace the home directory")
	}
	rootReal := realPathOrClean(sagaRoot)
	outputReal := realPathOrClean(output)
	if within(outputReal, rootReal) || within(rootReal, outputReal) {
		return "", errors.New("visual QA output must be outside the Saga and cannot contain it")
	}
	if info, statErr := os.Lstat(output); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("visual QA output must be a real directory")
		}
		if _, markerErr := os.Stat(filepath.Join(output, ".change-saga-visual-qa")); markerErr != nil {
			return "", errors.New("refusing to replace an existing directory not created by visual-qa")
		}
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	return output, nil
}

func installOutput(stage, output string) error {
	if info, err := os.Lstat(output); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("visual QA output changed and is no longer a real directory")
		}
		marker, markerErr := os.Lstat(filepath.Join(output, ".change-saga-visual-qa"))
		if markerErr != nil || marker.Mode()&os.ModeSymlink != 0 || !marker.Mode().IsRegular() {
			return errors.New("visual QA output changed and no longer has a regular managed marker")
		}
		if err := os.RemoveAll(output); err != nil {
			return fmt.Errorf("replace prior visual QA output: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		return fmt.Errorf("publish visual QA output: %w", err)
	}
	return nil
}

func findPlaywright(explicit string) (string, error) {
	candidates := []string{explicit, os.Getenv("CHANGE_SAGA_PLAYWRIGHT_DIR")}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "e2e"))
			if next := filepath.Dir(dir); next == dir {
				break
			}
		}
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "e2e"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "node_modules", "playwright", "package.json")); err == nil {
			return abs, nil
		}
	}
	return "", errors.New("Playwright is not ready; run npm ci and npx playwright install chromium in e2e, or pass --playwright-dir")
}

func within(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func realPathOrClean(path string) string {
	if value, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(value)
	}
	parent, base := filepath.Dir(path), filepath.Base(path)
	if value, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(value, base)
	}
	return filepath.Clean(path)
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
