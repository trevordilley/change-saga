package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/skills"
)

// The skill files under skills are the one source of the shipped authoring
// skill. These tests keep "change-saga install-skill" printing them verbatim
// and keep every command, flag, and query operation they name true to the CLI,
// so the skill cannot drift from the product again.

var shippedSkills = []struct {
	name  string
	files []skills.File
}{
	{name: "change-saga", files: skills.ChangeSaga()},
}

func TestInstallSkillPrintsTheSkillFilesVerbatim(t *testing.T) {
	var output bytes.Buffer
	if err := InstallSkill(nil, &output); err != nil {
		t.Fatal(err)
	}
	want := installSkillPreamble
	for _, skill := range shippedSkills {
		for _, file := range skill.files {
			want += fmt.Sprintf(installSkillFileHeader, skill.name+"/"+file.Path) + file.Content
		}
	}
	if output.String() != want {
		t.Fatal("install-skill output is not the preamble followed by the skill files")
	}
	for _, skill := range shippedSkills {
		root := filepath.Join("..", "..", "skills", skill.name)
		var paths []string
		for _, file := range skill.files {
			paths = append(paths, file.Path)
			onDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
			if err != nil || string(onDisk) != file.Content {
				t.Fatalf("embedded %s/%s differs from the file on disk (err=%v)", skill.name, file.Path, err)
			}
		}
		if len(paths) == 0 || paths[0] != "SKILL.md" {
			t.Fatalf("install-skill must lead %s with SKILL.md: %v", skill.name, paths)
		}
		for _, file := range skill.files {
			for _, link := range regexp.MustCompile(`\]\((references/[^)]+)\)`).FindAllStringSubmatch(file.Content, -1) {
				if !containsString(paths, link[1]) {
					t.Errorf("%s/%s links to %s, which install-skill does not print", skill.name, file.Path, link[1])
				}
			}
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !containsString(paths, rel) {
				t.Errorf("%s/%s is not embedded; install-skill would omit it", skill.name, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A routed skill is useful only if an installed copy sends a realistic task
// to the smallest complete set of references and every routed file ships with
// it. Build a temporary installed fixture rather than reading repository files
// so this covers the same package users receive from install-skill.
func TestInstalledSkillRoutesFocusedTasks(t *testing.T) {
	root := t.TempDir()
	for _, file := range skills.ChangeSaga() {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entrypoint, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(normalizeSkillNewlines(string(entrypoint)), "\n")
	referenceLink := regexp.MustCompile(`\]\((references/[^)]+)\)`)

	tests := []struct {
		name     string
		fixture  string
		rowHint  string
		want     []string
		unwanted []string
	}{
		{
			name: "compact feature context", fixture: "Load bounded context for the checkout feature without reading the whole Saga.",
			rowHint: "load compact feature context", want: []string{"references/query.md"},
			unwanted: []string{"references/diagrams.md", "references/stories.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "feature handoff audit", fixture: "Audit whether checkout has a current exact implementation handoff.",
			rowHint: "Audit whether one feature", want: []string{"references/query.md"},
			unwanted: []string{"references/diagrams.md", "references/stories.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "diagram for a changed request flow", fixture: "Draw a request-flow diagram and attach each Item to the exact changed code.",
			rowHint: "Author diagrams", want: []string{"references/query.md", "references/diagrams.md"},
			unwanted: []string{"references/stories.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "mechanical visual QA", fixture: "Render the checkout slides and inspect the visual QA contact sheet.",
			rowHint: "Render slides", want: []string{"references/diagrams.md"},
			unwanted: []string{"references/query.md", "references/stories.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "accepted story with provenance", fixture: "Add an accepted customer story with one confirmed criterion and its source citation.",
			rowHint: "author or revise personas", want: []string{"references/query.md", "references/stories.md"},
			unwanted: []string{"references/diagrams.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "feature interview before a candidate story", fixture: "Interview me about reusable architecture, using the personas already in app.saga before drafting stories.",
			rowHint: "Interview for a feature", want: []string{"references/query.md", "references/stories.md"},
			unwanted: []string{"references/diagrams.md", "references/terms.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "term rename", fixture: "Revise the Retention Window term after its defining constant was renamed.",
			rowHint: "Author or revise the overview", want: []string{"references/query.md", "references/terms.md"},
			unwanted: []string{"references/diagrams.md", "references/stories.md", "references/integration.md", "references/ci.md"},
		},
		{
			name: "comparison recovery handoff", fixture: "Reconcile stale evidence after a branch update and hand off the remaining conflicting heads.",
			rowHint: "Reconcile a comparison", want: []string{"references/query.md", "references/integration.md"},
			unwanted: []string{"references/diagrams.md", "references/stories.md", "references/terms.md", "references/ci.md"},
		},
		{
			name: "parallel proposal consolidation", fixture: "Compare two proposal branches, then consolidate a confirmed duplicate.",
			rowHint: "Compare parallel proposal branches", want: []string{"references/query.md", "references/integration.md", "references/stories.md"},
			unwanted: []string{"references/diagrams.md", "references/terms.md", "references/ci.md"},
		},
		{
			name: "pull request review visual", fixture: "Prepare this pull request's review deck and explain its retry path with exact evidence.",
			rowHint: "Prepare or update a pull-request review artifact", want: []string{"references/query.md", "references/integration.md", "references/diagrams.md"},
			unwanted: []string{"references/stories.md", "references/terms.md", "references/ci.md"},
		},
		{
			name: "CI policy only", fixture: "Make CI require implementation and health coverage for this repository.",
			rowHint: "Define a CI acceptance rule", want: []string{"references/ci.md", "references/query.md"},
			unwanted: []string{"references/diagrams.md", "references/stories.md", "references/terms.md", "references/integration.md"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if strings.TrimSpace(test.fixture) == "" {
				t.Fatal("realistic fixture is empty")
			}
			var row string
			for _, candidate := range rows {
				if strings.HasPrefix(candidate, "|") && strings.Contains(candidate, test.rowHint) {
					row = candidate
					break
				}
			}
			if row == "" {
				t.Fatalf("installed SKILL.md has no route for %q", test.rowHint)
			}
			links := map[string]bool{}
			for _, match := range referenceLink.FindAllStringSubmatch(row, -1) {
				links[match[1]] = true
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(match[1]))); err != nil {
					t.Errorf("route links to an unshipped reference %q: %v", match[1], err)
				}
			}
			for _, want := range test.want {
				if !links[want] {
					t.Errorf("route for fixture %q omitted %s: %s", test.fixture, want, row)
				}
			}
			for _, unwanted := range test.unwanted {
				if links[unwanted] {
					t.Errorf("route for fixture %q preloads unrelated %s: %s", test.fixture, unwanted, row)
				}
			}
		})
	}

	compatibility, err := os.ReadFile(filepath.Join(root, "references", "authoring.md"))
	if err != nil {
		t.Fatalf("authoring compatibility router was not installed: %v", err)
	}
	for _, reference := range []string{"diagrams.md", "stories.md", "terms.md", "integration.md", "query.md"} {
		if !strings.Contains(string(compatibility), "]("+reference+")") {
			t.Errorf("authoring compatibility router omitted %s", reference)
		}
	}
	if len(strings.Fields(string(compatibility))) > 100 {
		t.Fatalf("authoring compatibility router became a blanket manual: %d words", len(strings.Fields(string(compatibility))))
	}
}

// renderQueryOperations is the operation list references/query.md must carry,
// generated from the table the query command dispatches on.
func renderQueryOperations() string {
	var list strings.Builder
	for _, operation := range queryOperations {
		fmt.Fprintf(&list, "- `%s`: %s.\n  `%s`\n", operation, queryPurpose[operation], queryUsage[operation])
	}
	return list.String()
}

const (
	queryOperationsBegin = "<!-- query-operations:begin -->\n"
	queryOperationsEnd   = "<!-- query-operations:end -->\n"
)

// Run with CHANGE_SAGA_UPDATE_SKILL=1 to regenerate the list after changing
// the query operations.
func TestSkillQueryReferenceListsExactlyTheQueryOperations(t *testing.T) {
	path := filepath.Join("..", "..", "skills", "change-saga", "references", "query.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := normalizeSkillNewlines(string(data))
	begin, end := strings.Index(text, queryOperationsBegin), strings.Index(text, queryOperationsEnd)
	if begin < 0 || end < begin {
		t.Fatalf("%s has no query-operations markers", path)
	}
	current, want := text[begin+len(queryOperationsBegin):end], renderQueryOperations()
	if current == want {
		return
	}
	if os.Getenv("CHANGE_SAGA_UPDATE_SKILL") == "1" {
		updated := text[:begin+len(queryOperationsBegin)] + want + text[end:]
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("%s lists stale query operations; rerun with CHANGE_SAGA_UPDATE_SKILL=1. Want:\n%s", path, want)
}

// skillCommands maps each top-level command to the function that runs it, so
// a test can read the real flag set from its -h output.
var skillCommands = map[string]func(context.Context, []string, io.Writer) error{
	"preintegrate": Preintegrate,
	"init":         Init, "feature": Feature, "review": Review, "overview": Overview, "component": func(ctx context.Context, args []string, out io.Writer) error {
		return Technical(ctx, "component", args, out)
	},
	"system": func(ctx context.Context, args []string, out io.Writer) error {
		return Technical(ctx, "system", args, out)
	},
	"data-entity": func(ctx context.Context, args []string, out io.Writer) error {
		return Technical(ctx, "data-entity", args, out)
	},
	"erd": func(ctx context.Context, args []string, out io.Writer) error {
		return Technical(ctx, "erd", args, out)
	},
	"erd-overlay": func(ctx context.Context, args []string, out io.Writer) error {
		return Technical(ctx, "erd-overlay", args, out)
	},
	"inventory": Inventory,
	"term":      Term, "persona": Persona,
	"setup-initial-saga": func(_ context.Context, args []string, out io.Writer) error { return SetupInitialSaga(args, out) },
	"flag":               FeatureFlag, "prototype": Prototype, "story": Story, "criterion": Criterion, "citation": Citation,
	"relation": Relation, "plan": Plan, "design": Design, "quality": Quality, "add-deck": AddDeck,
	"add-slide": AddSlide, "apply-slide": ApplySlide, "set-slide-content": SetSlideContent, "add-item": AddItem, "add-section": AddSection,
	"add-chapter": AddChapter, "add-fragment": AddFragment, "set-fragment-content": SetFragmentContent,
	"add-landmark": AddLandmark, "revise-deck": ReviseDeck, "remove-deck": RemoveDeck, "revise-slide": ReviseSlide, "remove-slide": RemoveSlide, "revise-item": ReviseItem, "remove-item": RemoveItem, "revise-chapter": ReviseChapter, "remove-chapter": RemoveChapter, "revise-section": ReviseSection, "remove-section": RemoveSection, "revise-fragment": ReviseFragment, "remove-fragment": RemoveFragment, "cover": Cover, "remove-coverage": RemoveCoverage,
	"replace-coverage": ReplaceCoverage, "references": References, "repin": Repin, "sync": Sync,
	"add-claim": AddClaim, "verify-claim": VerifyClaim, "validate": Validate, "status": Status, "reconcile": Reconcile, "check": Check, "query": Query,
	"visual-qa":     VisualQA,
	"serve":         func(ctx context.Context, args []string, out io.Writer) error { return Serve(ctx, args, out, false) },
	"open":          func(ctx context.Context, args []string, out io.Writer) error { return Serve(ctx, args, out, true) },
	"install-skill": func(_ context.Context, args []string, out io.Writer) error { return InstallSkill(args, out) },
	"spec":          func(_ context.Context, args []string, out io.Writer) error { return Spec(args, out) },
}

func TestSkillCommandTableCoversEveryCommand(t *testing.T) {
	for _, command := range commandOrder {
		if skillCommands[command] == nil {
			t.Errorf("skillCommands has no entry for %q", command)
		}
	}
}

// hasSubcommands reports whether name is a command family, such as "review"
// or "quality test-case", rather than a runnable command.
func hasSubcommands(name string) bool {
	if name == "query" || name == "design" || name == "serve" {
		return true
	}
	for known := range commandUsage {
		if strings.HasPrefix(known, name+" ") {
			return true
		}
	}
	for _, command := range grammar.Commands() {
		if strings.HasPrefix(command.Name, name+" ") {
			return true
		}
	}
	return false
}

var commandWord = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// resolveSkillCommand resolves the words after "change-saga" to a command
// path, or explains why they name no command.
func resolveSkillCommand(words []string) ([]string, error) {
	if len(words) == 0 || !commandWord.MatchString(words[0]) {
		return nil, nil
	}
	if skillCommands[words[0]] == nil || !containsString(commandOrder, words[0]) {
		return nil, fmt.Errorf("names no command %q", words[0])
	}
	path := []string{words[0]}
	for hasSubcommands(strings.Join(path, " ")) {
		family := strings.Join(path, " ")
		if len(path) == len(words) || !commandWord.MatchString(words[len(path)]) {
			return path, nil // the family itself, as in "change-saga serve --open"
		}
		next := words[len(path)]
		name := family + " " + next
		switch {
		case family == "query":
			if !containsString(queryOperations, next) {
				return nil, fmt.Errorf("names no query operation %q", next)
			}
			return append(path, next), nil
		case family == "serve":
			if next != "status" && next != "stop" {
				return nil, fmt.Errorf("names no serve subcommand %q", next)
			}
			return append(path, next), nil
		case family == "design":
			if _, ok := commandUsage["design-"+next]; !ok {
				return nil, fmt.Errorf("names no design subcommand %q", next)
			}
			return append(path, next), nil
		}
		_, declared := grammar.Lookup(name)
		if _, used := commandUsage[name]; !used && !declared {
			return nil, fmt.Errorf("names no subcommand %q", name)
		}
		path = append(path, next)
	}
	return path, nil
}

// realFlags returns the flags a command's own -h output declares.
func realFlags(t *testing.T, path []string) map[string]bool {
	t.Helper()
	flags := map[string]bool{"h": true, "help": true}
	if path[0] == "query" {
		if len(path) > 1 {
			for _, match := range regexp.MustCompile(`--([a-z][a-z0-9-]*)`).FindAllStringSubmatch(queryUsage[path[1]], -1) {
				flags[match[1]] = true
			}
		}
		return flags
	}
	var output bytes.Buffer
	_ = skillCommands[path[0]](context.Background(), append(append([]string{}, path[1:]...), "-h"), &output)
	for _, match := range regexp.MustCompile(`(?m)^\s+-([a-z][a-z0-9-]*)`).FindAllStringSubmatch(output.String(), -1) {
		flags[match[1]] = true
	}
	return flags
}

// skillInvocations returns every command the skill files show: each line of a
// fenced code block (with continuations joined) and each inline code span that
// starts with "change-saga ".
func skillInvocations(content string) []string {
	var invocations []string
	var prose strings.Builder
	inFence := false
	pending := ""
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			prose.WriteString(line + "\n")
			continue
		}
		pending += strings.TrimSpace(line)
		if strings.HasSuffix(pending, "\\") {
			pending = strings.TrimSuffix(pending, "\\") + " "
			continue
		}
		if strings.HasPrefix(pending, "change-saga ") {
			invocations = append(invocations, pending)
		}
		pending = ""
	}
	flat := strings.Join(strings.Fields(prose.String()), " ")
	for _, span := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(flat, -1) {
		if strings.HasPrefix(span[1], "change-saga ") {
			invocations = append(invocations, span[1])
		}
	}
	return invocations
}

func TestSkillNamesOnlyRealCommandsAndFlags(t *testing.T) {
	mention := regexp.MustCompile(`(?:^|[^\w:/-])change-saga ([a-z][a-z0-9 -]*)`)
	frontmatter := regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)
	for _, skill := range shippedSkills {
		for _, file := range skill.files {
			if filepath.Ext(file.Path) != ".md" {
				continue
			}
			body := frontmatter.ReplaceAllString(normalizeSkillNewlines(file.Content), "")
			flat := strings.Join(strings.Fields(body), " ")
			for _, match := range mention.FindAllStringSubmatch(flat, -1) {
				if _, err := resolveSkillCommand(strings.Fields(match[1])); err != nil {
					t.Errorf("%s/%s: %q %v", skill.name, file.Path, strings.TrimSpace(match[0]), err)
				}
			}
			flagPattern := regexp.MustCompile(`(?:^|\s)--([a-z][a-z0-9-]*)`)
			for _, invocation := range skillInvocations(body) {
				words := strings.Fields(strings.TrimPrefix(invocation, "change-saga "))
				path, err := resolveSkillCommand(words)
				if err != nil {
					t.Errorf("%s/%s: %q %v", skill.name, file.Path, invocation, err)
					continue
				}
				if path == nil {
					continue
				}
				flags := realFlags(t, path)
				for _, flag := range flagPattern.FindAllStringSubmatch(invocation, -1) {
					if !flags[flag[1]] {
						t.Errorf("%s/%s: %q passes --%s, which %q does not accept", skill.name, file.Path, invocation, flag[1], strings.Join(path, " "))
					}
				}
			}
		}
	}
}

func normalizeSkillNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
