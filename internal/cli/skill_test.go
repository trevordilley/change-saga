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
	"init": Init, "feature": Feature, "review": Review, "overview": Overview, "term": Term, "persona": Persona,
	"setup-initial-saga": func(_ context.Context, args []string, out io.Writer) error { return SetupInitialSaga(args, out) },
	"flag":               FeatureFlag, "prototype": Prototype, "story": Story, "criterion": Criterion, "citation": Citation,
	"relation": Relation, "plan": Plan, "design": Design, "quality": Quality, "add-deck": AddDeck,
	"add-slide": AddSlide, "apply-slide": ApplySlide, "set-slide-content": SetSlideContent, "add-item": AddItem, "add-section": AddSection,
	"add-chapter": AddChapter, "add-fragment": AddFragment, "set-fragment-content": SetFragmentContent,
	"add-landmark": AddLandmark, "revise-deck": ReviseDeck, "remove-deck": RemoveDeck, "revise-slide": ReviseSlide, "remove-slide": RemoveSlide, "revise-item": ReviseItem, "remove-item": RemoveItem, "revise-chapter": ReviseChapter, "remove-chapter": RemoveChapter, "revise-section": ReviseSection, "remove-section": RemoveSection, "revise-fragment": ReviseFragment, "remove-fragment": RemoveFragment, "cover": Cover, "remove-coverage": RemoveCoverage,
	"replace-coverage": ReplaceCoverage, "references": References, "repin": Repin, "sync": Sync,
	"add-claim": AddClaim, "verify-claim": VerifyClaim, "validate": Validate, "status": Status, "check": Check, "query": Query,
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
