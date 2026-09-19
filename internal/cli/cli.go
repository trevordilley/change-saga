package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
	"github.com/twentyideas/changesaga/internal/store"
)

// Version, Commit, and BuildDate describe the running binary. Release builds
// overwrite them with -ldflags -X; local builds keep the -dev default.
var (
	Version   = "0.2.0-dev"
	Commit    = ""
	BuildDate = ""
)

// VersionString renders the version plus whatever build metadata was injected.
func VersionString() string {
	info, _ := debug.ReadBuildInfo()
	return versionString(Version, Commit, BuildDate, info)
}

func versionString(version, commit, buildDate string, info *debug.BuildInfo) string {
	// `go install module@version` cannot supply linker flags. Preserve release
	// injection when present, but make an installed module report its module and
	// VCS metadata instead of the source-tree development placeholder.
	if info != nil {
		if strings.HasSuffix(version, "-dev") && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = strings.TrimPrefix(info.Main.Version, "v")
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "" {
					commit = setting.Value
					if len(commit) > 12 {
						commit = commit[:12]
					}
				}
			case "vcs.time":
				if buildDate == "" {
					buildDate = setting.Value
				}
			}
		}
	}

	out := version
	if commit != "" {
		out += " (" + commit + ")"
	}
	if buildDate != "" {
		out += " built " + buildDate
	}
	return out
}

type StatusError struct{ Code int }

func (e *StatusError) Error() string { return "command reported a non-success status" }

// commandOrder fixes the order the top-level help lists commands in;
// commandUsage is the single source of each command's usage line so the
// overview, the per-command -h banner, and argument errors cannot drift apart.
var commandOrder = []string{
	"init", "epic", "persona", "flag", "prototype", "story", "criterion", "citation", "relation", "design", "plan", "quality", "add-deck", "add-slide", "set-slide-content", "add-item", "add-chapter", "add-section", "add-fragment", "set-fragment-content", "add-landmark", "cover", "remove-coverage", "replace-coverage", "references", "repin", "sync", "add-claim", "verify-claim",
	"review", "validate", "status", "query",
	"serve", "open", "install-skill", "spec",
}

var commandUsage = map[string]string{
	"init":                        "change-saga init [flags] <name.saga>",
	"epic":                        "change-saga epic add [flags] <saga>",
	"epic add":                    "change-saga epic add --id ID --title TEXT [--description TEXT] [flags] <saga>",
	"persona":                     "change-saga persona <add|revise|set-state> [flags] <saga>",
	"persona add":                 "change-saga persona add --id ID --name TEXT --description TEXT [--revision r1] [--event active] [flags] <saga>",
	"persona revise":              "change-saga persona revise --persona URN --revision ID --parent URN... --name TEXT --description TEXT [flags] <saga>",
	"persona set-state":           "change-saga persona set-state --persona URN --event ID --parent URN... --state active|retired [--reason TEXT] [flags] <saga>",
	"flag":                        "change-saga flag <add|revise|set-state> [flags] <saga>",
	"flag add":                    "change-saga flag add --id ID --description TEXT --target URN... [--state off|on] [flags] <saga>",
	"flag revise":                 "change-saga flag revise --flag URN --revision ID --parent URN... --description TEXT --target URN... [flags] <saga>",
	"flag set-state":              "change-saga flag set-state --flag URN --event ID --parent URN... --state off|on|retired [--reason TEXT] [flags] <saga>",
	"story move":                  "change-saga story move --story URN --epic ID [--json] <saga>",
	"prototype":                   "change-saga prototype <add-html|add-external|revise|annotate> [flags] <saga>",
	"prototype add-html":          "change-saga prototype add-html --epic ID --id ID --revision ID --title TEXT --source PATH [--state STATE] [flags] <saga>",
	"prototype add-external":      "change-saga prototype add-external --epic ID --id ID --revision ID --title TEXT --url URL [--embed-url URL --provider ID --embed-origin ORIGIN] [flags] <saga>",
	"prototype revise":            "change-saga prototype revise --prototype URN --revision ID --parent URN... --title TEXT (--source PATH | --url URL) [flags] <saga>",
	"prototype annotate":          "change-saga prototype annotate --prototype URN --id ID --target URN --rationale TEXT --story-revision URN (--prototype-revision URN | --prototype-content-digest DIGEST) [selector] [flags] <saga>",
	"story":                       "change-saga story <add|revise|set-state|move> [flags] <saga>",
	"story add":                   "change-saga story add --epic ID --id ID --revision ID --event ID --title TEXT --statement TEXT --priority TEXT [flags] <saga>",
	"story revise":                "change-saga story revise --story URN --revision ID --parent URN... --title TEXT --statement TEXT --priority TEXT [flags] <saga>",
	"story set-state":             "change-saga story set-state --story URN --event ID --parent URN... --state STATE [flags] <saga>",
	"criterion":                   "change-saga criterion <add|revise|remove> [flags] <saga>",
	"criterion add":               "change-saga criterion add --story URN --parent REVISION --revision ID --id ID --statement TEXT [flags] <saga>",
	"criterion revise":            "change-saga criterion revise --story URN --criterion URN --parent REVISION --revision ID (--statement TEXT|--edit) [flags] <saga>",
	"criterion remove":            "change-saga criterion remove --story URN --criterion URN --parent REVISION --revision ID --reason TEXT [flags] <saga>",
	"citation":                    "change-saga citation add [flags] <saga>",
	"citation add":                "change-saga citation add --epic ID --id ID --kind KIND --title TEXT --reference LOCATOR [flags] <saga>",
	"relation":                    "change-saga relation <add|supersede|status> [flags] <saga>",
	"relation add":                "change-saga relation add --epic ID --id ID --type TYPE --from URN --to URN --rationale TEXT [--scope self|descendants] [flags] <saga>",
	"relation supersede":          "change-saga relation supersede --relation URN [--epic ID] [--request-id ID] [--json] <saga>",
	"relation status":             "change-saga relation status [--relation URN] [--json] <saga>",
	"design":                      "change-saga design <operation> [flags] <saga>",
	"design-add-chapter":          "change-saga design add-chapter --epic ID [flags] <saga> <name>",
	"design-add-section":          "change-saga design add-section [flags] <saga> <section/path>",
	"design-add-fragment":         "change-saga design add-fragment (--epic ID | --section TARGET) [flags] <saga>",
	"design-set-fragment-content": "change-saga design set-fragment-content --target TARGET --source FILE|- [--epic ID] [--json|--quiet] <saga>",
	"plan":                        "change-saga plan <add-wave|revise-wave|add-item|revise-item|add-dependency|add-contract|assign|progress|record-merge> [flags] <saga>",
	"plan add-wave":               "change-saga plan add-wave --epic ID --id ID --revision ID --title TEXT --objective TEXT --request-id ID [flags] <saga>",
	"plan revise-wave":            "change-saga plan revise-wave --wave URN --revision ID --parent URN... --title TEXT --objective TEXT --request-id ID [flags] <saga>",
	"plan add-item":               "change-saga plan add-item --epic ID --id ID --revision ID --title TEXT --objective TEXT --deliverable TEXT... --request-id ID [flags] <saga>",
	"plan revise-item":            "change-saga plan revise-item --item URN --revision ID --parent URN... --title TEXT --objective TEXT --deliverable TEXT... --request-id ID [flags] <saga>",
	"plan add-dependency":         "change-saga plan add-dependency --epic ID --id ID --prerequisite URN --dependent URN --condition KIND --reason TEXT --request-id ID [flags] <saga>",
	"plan add-contract":           "change-saga plan add-contract --epic ID --id ID --revision ID --kind KIND --provider URN --consumer URN --statement TEXT --acceptance TEXT... --request-id ID [flags] <saga>",
	"plan assign":                 "change-saga plan assign --item URN --workspace UUID --repository-id ID --branch NAME --request-id ID [flags] <saga>",
	"plan progress":               "change-saga plan progress --item URN --from EVENT... --to STATE --request-id ID [flags] <saga>",
	"plan record-merge":           "change-saga plan record-merge --item URN --unit ID --state STATE --request-id ID [flags] <saga>",
	"quality":                     "change-saga quality <test-case|policy|evidence|run> <operation> [flags] <saga>",
	"quality test-case":           "change-saga quality test-case <add|revise|set-state> [flags] <saga>",
	"quality test-case add":       "change-saga quality test-case add --epic ID --id ID [--revision r1] [--event proposed] --title TEXT --kind KIND... --automation MODE --step JSON... --expected-result TEXT [--precondition TEXT...] [--from FILE|-] [flags] <saga>",
	"quality test-case revise":    "change-saga quality test-case revise --test URN --parent REVISION... --revision ID [definition flags] [--from FILE|-] [flags] <saga>",
	"quality test-case set-state": "change-saga quality test-case set-state --test URN --parent EVENT... --state STATE [--event ID] [--reason TEXT] [flags] <saga>",
	"quality policy":              "change-saga quality policy set [flags] <saga>",
	"quality policy set":          "change-saga quality policy set --epic ID --criterion URN --story-revision URN --require KIND... --rationale TEXT [--allow MODE...] [--supersedes POLICY...] [--id ID] [flags] <saga>",
	"quality evidence":            "change-saga quality evidence add [flags] <saga>",
	"quality evidence add":        "change-saga quality evidence add --test URN --role ROLE (--code LOCATION... | --verification URN... | --citation URN...) [--test-revision URN] [--supersedes EVIDENCE...] [--id ID] [--batch FILE|-] [flags] <saga>",
	"quality run":                 "change-saga quality run record [flags] <saga>",
	"quality run record":          "change-saga quality run record --test URN --result RESULT --summary TEXT --evidence URN... [--parent RUN...] [--test-revision URN] [--command TEXT] [--commit REV] [--id ID] [flags] <saga>",
	"add-deck":                    "change-saga add-deck (--epic ID | --role onboarding) [flags] <saga> <name>",
	"add-slide":                   "change-saga add-slide --deck TARGET --intent INTENT --layout LAYOUT [flags] <saga> <name>",
	"set-slide-content":           "change-saga set-slide-content --target TARGET --source FILE|- [--json|--quiet] <saga>",
	"add-item":                    "change-saga add-item --slide TARGET --kind KIND [selector] [--record URN] [flags] <saga>",
	"add-chapter":                 "change-saga add-chapter (--epic ID | --app overview|designsystem) [flags] <saga> <name>",
	"add-section":                 "change-saga add-section [flags] <saga> <section/path>",
	"add-fragment":                "change-saga add-fragment (--epic ID | --app overview|designsystem | --section TARGET) [flags] <saga>",
	"set-fragment-content":        "change-saga set-fragment-content --target TARGET --source FILE|- [--json|--quiet] <saga>",
	"add-landmark":                "change-saga add-landmark [flags] <saga>",
	"cover":                       "change-saga cover [flags] [--batch FILE|-] [--dry-run] [--against REV [--head REV]] <saga>",
	"remove-coverage":             "change-saga remove-coverage --record PATH [--dry-run] [--json|--quiet] <saga>",
	"replace-coverage":            "change-saga replace-coverage --record PATH [coverage flags] [--batch FILE|-] [--dry-run] [--against REV [--head REV]] <saga>",
	"references":                  "change-saga references [--stale] [--diff] [--json] [--repo PATH] [--against REV [--head REV]] <saga>",
	"repin":                       "change-saga repin --onto REV [--branch REV] [--dry-run] [--json] [--repo PATH] <saga>",
	"sync":                        "change-saga sync --repo PATH [--commit REV] [--json] <saga>",
	"add-claim":                   "change-saga add-claim --target TARGET --kind KIND --statement TEXT --ref LOCATION [--ref LOCATION...] <saga>",
	"verify-claim":                "change-saga verify-claim --claim ID --status STATUS --summary TEXT [flags] <saga>",
	"review":                      "change-saga review [flags] <saga>",
	"validate":                    "change-saga validate [--json] [--fix] <saga>",
	"status":                      "change-saga status [--json] [--repo PATH] [--against REV [--head REV]] <saga>",
	"query":                       "change-saga query <operation> --saga PATH [--repo PATH] [operation flags]",
	"serve":                       "change-saga serve [--addr ADDR] [--repo PATH] [--open] [--detach] [--against REV [--head REV]] <saga>",
	"open":                        "change-saga open [--addr ADDR] [--repo PATH] [--against REV [--head REV]] <saga>",
	"install-skill":               "change-saga install-skill",
	"spec":                        "change-saga spec [--json]",
}

func PrintHelp(out io.Writer) {
	fmt.Fprint(out, `Change Saga — capture a big change from its first prototype to the code it changed

A Change Saga is for a big change: one that needs product requirements, UX/UI,
technical design, quality, and an implementation deck. It starts with the big
work and ends with the big work, even when the whole change lands in one PR.

A repository has one app Saga. It holds the app's overview, personas, design
system, onboarding deck, and feature flags, plus durable epics: the product
domains that each hold their own stories, design, quality, and implementation
deck. Commands that write epic content take --epic.

The workflow:
  0. App: "init" the app Saga and add the product domain the change belongs
     to ("epic"). Name who the app serves ("persona") and gate unreleased
     work ("flag") whenever that becomes useful; neither is required.
  1. Product: prototype the experience ("prototype") and write user stories
     with acceptance criteria ("story", "criterion"); a story may name the
     personas it serves. Stories can "story move" between epics. Cite
     where each requirement came from ("citation"). Prototypes and stories
     inform each other; revise both as the change is clarified.
  2. Design: develop the UX, UI, and technical design ("design") and relate
     each design artifact to the stories and criteria it addresses ("relation").
  3. Quality: define test cases for every acceptance criterion ("quality") and
     link them with verifies relations.
  4. Implementation: explain the delivered change as the implementation deck
     ("add-deck", "add-slide", "add-item"). Every meaningful node, edge,
     region, and callout is an Item.
  5. Code: reference every changed line from the Item that explains it
     ("cover"), then "validate", check "status", and "serve" the Saga for review.

Stories, prototypes, design, test cases, and deck bundles are Git-native
records partitioned so parallel workspaces can author them and merge cleanly;
"plan" organizes that work into dependency-aware waves.

Usage:
`)
	for _, command := range commandOrder {
		fmt.Fprintf(out, "  %s\n", commandUsage[command])
	}
	fmt.Fprint(out, `
Run "change-saga <command> -h" for command-specific options.

Using a coding agent?
  If its Change Saga skill is not installed or current, run
  "change-saga install-skill" and give the resulting agent-agnostic bootstrap
  prompt to the agent. The command does not modify the repository or create a Saga.
`)
}

// commandFlags builds a flag set whose -h output always leads with the command
// it was actually invoked as. The stock flag banner prints the flag set's name
// and nothing else, which made "change-saga open -h" claim to be serve and gave
// a flagless command like install-skill an empty, unexplained banner.
func commandFlags(name, usage string, out io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		fmt.Fprintf(out, "Usage:\n  %s\n", usage)
		if description := commandDescription[name]; description != "" {
			fmt.Fprintf(out, "\n%s\n", description)
		}
		count := 0
		flags.VisitAll(func(*flag.Flag) { count++ })
		if count > 0 {
			fmt.Fprint(out, "\nFlags:\n")
			flags.PrintDefaults()
		}
	}
	return flags
}

var commandDescription = map[string]string{
	"init":                        "Create the app Saga: the saga.json manifest, a reviewer README, and the app\noverview under ___overview. Then add an epic and author its content; personas\nand flags are optional and can come later.",
	"epic":                        "Add a durable product domain. An epic holds its own report content, stories,\ndesign, quality, work plan, and implementation deck. Story identity never\nnames an epic, so a story can move between epics without breaking a link.",
	"persona":                     "Author the people the app serves. Personas are optional living records: a story\nrevision may name the personas it serves, and status reports each active persona\nno accepted story serves as a coverage gap. Nothing blocks on personas.",
	"flag":                        "Author feature flags that gate stories or whole epics. A gated story can be\nimplemented but not enabled; status reports it that way.",
	"prototype":                   "Author revisioned interactive HTML experiences or explicitly allowed external\nembeds and pin them to the stories and criteria they clarify. A prototype may lead, follow,\nor evolve alongside its requirements.",
	"prototype add-html":          "Add an interactive HTML prototype and its first immutable revision. The authored\nsource is copied into the revision package, so later edits outside the Saga never change it.",
	"prototype add-external":      "Add a prototype that lives outside the Saga. A plain --url is a reference; an --embed-url\nrenders inline only with explicit provider, origin, sandbox, and permission allowlisting.",
	"prototype revise":            "Append a complete immutable prototype revision. Name every current head; an html\nrevision requires a fresh --source so no mutable directory becomes part of the revision.",
	"prototype annotate":          "Pin one part of a prototype to the story or criterion it means, with a rationale and an\nelement, text, region, or provider selector. A prototype may stay unlinked while exploration\ncontinues; it simply cannot contribute to readiness until a current annotation connects it.",
	"quality":                     "Author test cases, per-criterion kind policies, exact evidence, and immutable runs.\nEvery record is append-only; results stay visible and nothing is scored or summarized as a percentage.",
	"quality test-case":           "Define test cases with ordered steps, lifecycle them, and revise them as the behavior they\nverify evolves.",
	"quality test-case add":       "Add a test case: immutable identity, a complete first revision with ordered steps and declared\npositive/negative/edge kinds, and a proposed lifecycle root. Link it to criteria with verifies\nrelations; an unlinked test is an orphan and cannot satisfy coverage.",
	"quality test-case revise":    "Append a complete immutable revision. Name every current head; with one parent, omitted\nfields are inherited, while reconciling several heads requires the complete definition. Step IDs\nremoved earlier cannot be reused. Runs and evidence pinned to the old revision become stale.",
	"quality test-case set-state": "Append a proposed, active, deprecated, or retired lifecycle event. Name every current head;\nnaming several heads reconciles a conflict. Lifecycle is not a run result.",
	"quality policy":              "Declare which coverage kinds a criterion requires at an exact story revision.",
	"quality policy set":          "Record an immutable policy for one criterion and story revision. Without a policy only\npositive is required. Supersede every current head for the same pin so exactly one remains.",
	"quality evidence":            "Map a test revision to exact test code, implementation under test, or execution artifacts.",
	"quality evidence add":        "Record immutable evidence. test_implementation code references join global changed-source\naccounting; implementation_under_test references name the code the test exercises;\nexecution_artifact cites verifications or citations. --batch validates the whole set before the\nfirst write.",
	"quality run":                 "Record what executed and what happened.",
	"quality run record":          "Append an immutable run/result event pinned to a test revision, the code commit it ran against\n(--commit, default HEAD of --repo), and evidence.\nName every current run head: a failed or concurrent run stays visible and is only succeeded by a\nlater run. The command is recorded, never executed.",
	"story":                       "Create and append revisions or lifecycle events to user stories and acceptance\ncriteria. Stories may lead, follow, or evolve alongside prototypes; cite their source\nand revise them as the feature is clarified.",
	"story add":                   "Add a sourced user story and its first complete acceptance-criteria revision.\nIt may begin from a prototype, precede one, or evolve alongside one.",
	"story revise":                "Append a complete story revision as requirements or prototypes evolve. Name every\ncurrent parent head when reconciling concurrent edits; prior revisions remain history.",
	"story set-state":             "Append a lifecycle decision without rewriting the story. Acceptance records intent,\nnot implementation completion or peer-review approval.",
	"criterion":                   "Add, revise, or remove one acceptance criterion by creating a complete immutable\nstory revision from an explicit current parent head.",
	"criterion add":               "Add one explicitly identified acceptance criterion. Historical criterion IDs are\nnever reusable, including after removal.",
	"criterion revise":            "Revise one criterion's wording without changing its stable identity. Use --edit to\ninspect the complete proposed story revision in $EDITOR.",
	"criterion remove":            "Remove one criterion through a complete story revision. The required reason is\nreturned as commit guidance; no mutable tombstone is stored.",
	"citation":                    "Create immutable provenance records for requirements and design decisions.",
	"citation add":                "Record where a requirement or decision came from: an external URL, issue, document,\nrepository commit, or recorded decision. Provenance is context, not delivery evidence.",
	"relation":                    "Create, explicitly supersede, or check the currency of pinned traceability relations.",
	"relation add":                "Link stories and criteria to design, work items, slide explanations, and verification\nevidence with a typed rationale. Pin mutable requirement endpoints so their links go stale\nafter later edits. Test cases may verify criteria,\nscope is self unless a Deck/Slide source declares descendants, and each omitted required\nrevision pin defaults to the endpoint's unique current head (reported as it is pinned).",
	"relation supersede":          "Retire one relation without erasing it. Add its corrected replacement separately;\na pivot is represented by normal requirement, design, plan, and relation revisions.",
	"relation status":             "Report each relation as current, stale, conflicted, invalid, or superseded by comparing\nits persisted revision and digest pins with current heads. Only a current relation counts\nas coverage; every other status lists concrete reasons.",
	"design":                      "Develop technical-design chapters, sections, and fragments that trace to user\nstories and acceptance criteria. Design may evolve alongside prototypes and requirements;\nits addressable packages are partitioned for parallel authoring and clean merges.",
	"design-add-chapter":          "Add one independently authored technical-design concern. Chapters may be developed\nin parallel and should cite the requirements their contained design addresses.",
	"design-add-section":          "Partition a design chapter around one coherent subsystem, workflow, or decision so\nconcurrent agents can work without contending on a shared fragment.",
	"design-add-fragment":         "Add an addressable technical-design artifact. Prefer a diagram, prototype walkthrough,\nor concrete example and relate it to the stories and acceptance criteria it develops.",
	"design-set-fragment-content": "Replace authored design content while preserving its stable target. Refresh\ncontent-digest-pinned relations after intentional changes so downstream work is not stale.",
	"plan":                        "Turn requirements and design into dependency-aware waves, parallel workspace\nassignments, explicit convergence, progress, and immutable merge evidence. Planning\nmay begin while the technical design is still maturing and must track later revisions.",
	"plan add-wave":               "Add one delivery phase with explicit entry and exit conditions. Waves describe when\nparallel workspace lanes may fan out or must converge; display order is not dependency.",
	"plan revise-wave":            "Append a complete wave revision as sequencing or convergence changes. Reconcile every\ncurrent parent head so concurrent planning remains visible until intentionally merged.",
	"plan add-item":               "Add one independently assignable, mergeable unit of work and link it to the\nrequirements and design it advances. Keep ownership narrow enough for parallel work.",
	"plan revise-item":            "Append a complete work-item revision when scope, touch areas, deliverables, or merge\nunits change. Preserve prior plans and refresh revision-pinned downstream contracts.",
	"plan add-dependency":         "Record a real prerequisite between work items. Do not serialize independent items;\nuse dependencies to express the minimum safe fan-out and convergence graph.",
	"plan add-contract":           "Define the versioned interface between parallel provider and consumer work items.\nUse its acceptance checks as the stable seam that lets both workspaces proceed safely.",
	"plan assign":                 "Bind a work item to a concrete workspace and branch so progress can be shown in the\nlive Saga. Assignment is coordination state, not evidence of implementation.",
	"plan progress":               "Append explicit workspace progress against the item. Progress helps coordination but\nnever proves correctness, acceptance-criterion coverage, or delivery.",
	"plan record-merge":           "Append merge evidence for a declared merge unit. A merged state contributes delivery\nevidence only when its immutable commit and diff links resolve.",
	"add-deck":                    "Add an implementation deck. The implementation decks are the Saga's Implementation\nsection; split the delivered change into decks only where a concern warrants its own review.",
	"add-slide":                   "Add one visual argument to an implementation deck. Intent names the\nreviewer job; layout names geometry, not meaning. Establish the system model, then\nforeground consequential tradeoffs, hidden coupling, and deviations that may surprise a reviewer.",
	"set-slide-content":           "Replace a slide's visual entrypoint while preserving its stable target and items.",
	"add-item":                    "Add one semantic visual item, including an evidence-bearing callout overlay, and append\nit to the slide reading order. Code references attach here.",
	"set-fragment-content":        "Replace a fragment entrypoint through the supported authoring API. Use --source -\nto read content from standard input; the fragment media type and metadata are preserved.",
	"add-chapter":                 "Add one independently reviewable narrative chapter to the Saga.",
	"add-section":                 "Group related narrative content inside a chapter.",
	"add-fragment":                "Add a narrative artifact to a chapter or section. Implementation evidence belongs on\ndeck Items; use add-slide and add-item for the implementation deck.",
	"add-landmark":                "Create a coverable target for one Markdown heading, exact text span, HTML/SVG\nelement, or normalized image region inside a fragment. An SVG --element-id is\nmeasured into an on-canvas link automatically; --hotspot overrides its bounds.\nHTML elements need --hotspot for an on-canvas link. Visual landmarks require a\nsemantic --description for non-visual consumers.",
	"cover": `Reference the code a review target explains, pinned at a commit with a digest of
its content. --side new pins the comparison's head and --side old its merge-base (for
deleted lines); --commit pins any revision; --ref names a location directly. --file
references a whole file (renames, mode and binary changes). In the implementation deck
the target is an Item. Narrative targets are section/fragment paths, target URNs, and
<fragment-path>#<landmark-id>.
--batch reads newline-delimited JSON records (or one JSON array) with the per-record
fields target, path, side, lines, changed_lines, file, commit, refs, note, and name; the
whole batch is resolved before anything is written, and a failing record leaves the saga
untouched.`,
	"remove-coverage":  "Delete one exact coverage record named by query mappings or fragment-diffs.",
	"replace-coverage": "Atomically replace one coverage record with one or more newly resolved records.\nUse --batch to split or retarget broad evidence without leaving partial coverage.",
	"references":       "List every code reference: current (remapped when its lines only moved), or stale with the\nreason its code changed. Observing, health is judged at --head; comparing, at both sides.\n--diff adds the patch since the pin.",
	"repin":            "After a change lands, re-pin evidence references to the landed commit (following moved\nlines, or the content digest when the branch commit is gone) and record the branch's commit\nmessages in ___merges/<commit>.json so a squash merge keeps its reasoning.",
	"sync":             "Move a companion Saga's sync cursor (sync.json) to the code commit it now documents,\ndefault HEAD of --repo. Move it in every Saga commit that updates the documentation, so a\ncomparison reads the Saga that documented its merge-base. A Saga in its code repository\nhas no cursor: it documents the commit it is read at. repin moves it too.",
	"add-claim":        "Record one falsifiable author assertion and the code that supports it. Claims do not\ncount toward coverage and are independently verified.",
	"verify-claim":     "Append an independent verification result without rewriting the claim or prior results.",
	"open":             "Start a managed loopback reviewer, open it in a browser, and return after\nprinting the PID and active URL. Without --against it observes the app at --head: every\nnode current, stale references as health warnings, approvals shown only as history.\nWith --against it compares what --head changes since their merge-base, the way a pull\nrequest does, and highlights the Changed, Affected, and Code layers.",
	"serve":            "Serve the saga on loopback for review. Detached instances are managed with\nchange-saga serve status [SAGA] and change-saga serve stop [SAGA].",
	"install-skill":    "Print the agent-agnostic prompt that installs the change-saga authoring skill.\nPipe it to a coding agent; it neither writes to this repository nor creates a saga.",
	"validate":         "Check the format and authoring completeness, including a warning for every Markdown\nfootnote without an evidence-bearing exact-text landmark. --fix adds missing stable\nheading anchors and changes nothing else.",
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == name {
			found = true
		}
	})
	return found
}

func Init(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("init", commandUsage["init"], out)
	title := flags.String("title", "", "saga title")
	id := flags.String("id", "", "stable saga identifier")
	repoDir := flags.String("repo", ".", "source repository checkout")
	repository := flags.String("repository", "", "portable absolute source repository URI; defaults to origin")
	allowLocalRepository := flags.Bool("allow-local-repository", false, "persist a local file:// repository identity when origin is unavailable")
	allowRepositoryMismatch := flags.Bool("allow-repository-mismatch", false, "accept an explicitly declared repository that differs from origin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["init"])
	}
	root := flags.Arg(0)
	if !strings.HasSuffix(filepath.Base(root), ".saga") {
		return fmt.Errorf("saga directory must end in .saga")
	}
	if _, err := os.Stat(root); err == nil {
		return fmt.Errorf("%s already exists", root)
	} else if !os.IsNotExist(err) {
		return err
	}
	if *title == "" {
		*title = strings.TrimSuffix(filepath.Base(root), ".saga")
	}
	if *id == "" {
		*id = store.Slug(strings.TrimSuffix(filepath.Base(root), ".saga"))
	}
	if !saga.ValidID(*id) {
		return fmt.Errorf("--id must be a stable 1-128 character identifier")
	}
	repositoryURI, _, err := discoverRepository(ctx, *repoDir, *repository, *allowLocalRepository, *allowRepositoryMismatch)
	if err != nil {
		return err
	}
	manifest := saga.Manifest{Schema: saga.SagaSchemaURL, Version: saga.SagaVersion, ID: *id, Title: *title, Source: saga.Source{Repository: repositoryURI}}
	// The app overview is the elevator pitch for the whole application. Epics,
	// personas, and everything else are authored after init.
	overview := saga.FragmentManifest{Version: saga.CurrentVersion, ID: *id + "-overview", Title: "Overview", MediaType: "text/markdown", Entrypoint: "content.md"}
	// A failed init must not leave a half-built .saga behind, because the
	// directory would then block a retry while never loading.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	// The saga itself is staged and published atomically; its containing
	// directories are ordinary parents and may be created up front. The parent
	// is then resolved, because a saga may legitimately live under a symlinked
	// ancestor such as macOS /tmp even though nothing inside a saga may be a
	// symlink.
	if err := os.MkdirAll(filepath.Dir(absRoot), 0o755); err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absRoot))
	if err != nil {
		return err
	}
	err = store.CommitDir(parent, filepath.Join(parent, filepath.Base(absRoot)), func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		reservedDirs := []string{"___claims", "___verifications", saga.CodeDirName}
		for _, dir := range reservedDirs {
			if err := os.MkdirAll(filepath.Join(stage, dir), 0o755); err != nil {
				return err
			}
		}
		if err := store.WriteJSON(filepath.Join(stage, saga.ManifestName), manifest, true); err != nil {
			return err
		}
		if err := store.WriteFile(filepath.Join(stage, "README.md"), []byte(reviewerBootstrapREADME), 0o644, true); err != nil {
			return err
		}
		return populateFragment(filepath.Join(stage, applayout.OverviewDir, "overview.fragment"), overview, "", nil)
	})
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists", root)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Created %s\nNext: add the product domain this change belongs to, then author its content:\n  change-saga epic add --id ID --title TEXT %s\n  change-saga story add --epic ID --id ID --revision r1 --event proposed --title TEXT --statement TEXT --priority TEXT %s\nPersonas are optional; name who the app serves when it helps:\n  change-saga persona add --id ID --name TEXT --description TEXT %s\n", root, root, root, root)
	return nil
}

func AddChapter(ctx context.Context, args []string, out io.Writer) error {
	return addChapter(ctx, args, out, narrativeAuthoring)
}

func addChapter(_ context.Context, args []string, out io.Writer, scope authoringScope) error {
	command := scope.command("add-chapter")
	flags := commandFlags(command, commandUsage[command], out)
	title := flags.String("title", "", "chapter title")
	id := flags.String("id", "", "stable chapter identifier")
	order := flags.Int("order", 0, "display order")
	epic, app := scope.placeFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: %s", commandUsage[command])
	}
	scope, err := scope.placed(epic, app)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(flags.Arg(1), ".chapter")
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || filepath.Clean(name) != name || filepath.Base(name) != name || strings.HasPrefix(name, "___") {
		return fmt.Errorf("chapter name must be a single non-reserved path component")
	}
	var created, createdID, createdTarget string
	err = authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		hierarchyRoot, err := scope.hierarchyRoot(document)
		if err != nil {
			return err
		}
		chapterTitle, chapterID := *title, *id
		if chapterTitle == "" {
			chapterTitle = strings.ReplaceAll(name, "-", " ")
		}
		if chapterID == "" {
			chapterID = store.Slug(name)
		}
		overviewID := chapterID + "-overview"
		if !saga.ValidID(chapterID) || !saga.ValidID(overviewID) || targetIDExists(document, chapterID) || targetIDExists(document, overviewID) {
			return fmt.Errorf("chapter id %q or its overview id is invalid or already used", chapterID)
		}
		dir := filepath.Join(hierarchyRoot, store.Slug(name)+".chapter")
		manifest := saga.ChapterManifest{Version: saga.CurrentVersion, ID: chapterID, Title: chapterTitle, Order: *order}
		overview := saga.FragmentManifest{Version: saga.CurrentVersion, ID: overviewID, MediaType: "text/markdown", Entrypoint: "content.md"}
		// One staged rename: a failed chapter never leaves a manifest-less
		// directory behind that would invalidate the saga for every later
		// command.
		err = store.CommitDir(document.Root, dir, func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			for _, reserved := range []string{saga.CodeDirName} {
				if err := os.Mkdir(filepath.Join(stage, reserved), 0o755); err != nil {
					return err
				}
			}
			if err := store.WriteJSON(filepath.Join(stage, "chapter.json"), manifest, true); err != nil {
				return err
			}
			return populateFragment(filepath.Join(stage, "overview.fragment"), overview, "", nil)
		})
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("chapter %s already exists", filepath.Base(dir))
		}
		rel, _ := filepath.Rel(document.Root, dir)
		created = filepath.ToSlash(rel)
		createdID = chapterID
		createdTarget = saga.ChapterTarget(document.Manifest.ID, chapterID)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added chapter %s\n", created)
	fmt.Fprintf(out, "Target: %s\n", createdTarget)
	fmt.Fprintf(out, "Next: %s --title \"Section title\" %s%s %s/section-name\n", scope.commandText("add-section"), scope.placeArguments(), flags.Arg(0), createdID)
	return nil
}

func AddSection(ctx context.Context, args []string, out io.Writer) error {
	return addSection(ctx, args, out, narrativeAuthoring)
}

func addSection(_ context.Context, args []string, out io.Writer, scope authoringScope) error {
	command := scope.command("add-section")
	flags := commandFlags(command, commandUsage[command], out)
	title := flags.String("title", "", "section title")
	id := flags.String("id", "", "stable section identifier")
	order := flags.Int("order", 0, "display order")
	epic, app := scope.placeFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: %s", commandUsage[command])
	}
	scope, err := scope.placed(epic, app)
	if err != nil {
		return err
	}
	sectionPath := filepath.Clean(flags.Arg(1))
	parentPath, name := filepath.Dir(sectionPath), filepath.Base(sectionPath)
	if parentPath == "." || name == "." || strings.HasPrefix(name, "___") || strings.HasSuffix(name, ".chapter") || strings.HasSuffix(name, ".fragment") {
		return fmt.Errorf("sections must be created inside an existing chapter or section")
	}
	var created, createdID, createdTarget string
	err = authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		parentDir, _, err := scope.resolveTarget(document, parentPath, false)
		if err != nil {
			return fmt.Errorf("resolve parent: %w", err)
		}
		dir := filepath.Join(parentDir, name)
		sectionTitle, sectionID := *title, *id
		if sectionTitle == "" {
			sectionTitle = strings.ReplaceAll(name, "-", " ")
		}
		if sectionID == "" {
			sectionID = store.Slug(strings.ReplaceAll(filepath.ToSlash(flags.Arg(1)), "/", "-"))
		}
		if !saga.ValidID(sectionID) || targetIDExists(document, sectionID) {
			return fmt.Errorf("section id %q is invalid or already used", sectionID)
		}
		manifest := saga.SectionManifest{Version: saga.CurrentVersion, ID: sectionID, Title: sectionTitle, Order: *order}
		err = store.CommitDir(document.Root, dir, func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			for _, reserved := range []string{saga.CodeDirName} {
				if err := os.Mkdir(filepath.Join(stage, reserved), 0o755); err != nil {
					return err
				}
			}
			return store.WriteJSON(filepath.Join(stage, "section.json"), manifest, true)
		})
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("section %s already exists", flags.Arg(1))
		}
		if err == nil {
			rel, _ := filepath.Rel(document.Root, dir)
			created = filepath.ToSlash(rel)
			createdID = sectionID
			createdTarget = saga.SectionTarget(document.Manifest.ID, sectionID)
		}
		return err
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added section %s\n", created)
	fmt.Fprintf(out, "Target: %s\n", createdTarget)
	fmt.Fprintf(out, "Next: %s --section %s --title \"Fragment title\" %s%s\n", scope.commandText("add-fragment"), createdID, scope.placeArguments(), flags.Arg(0))
	return nil
}

func AddFragment(ctx context.Context, args []string, out io.Writer) error {
	return addFragment(ctx, args, out, narrativeAuthoring)
}

func addFragment(_ context.Context, args []string, out io.Writer, scope authoringScope) error {
	command := scope.command("add-fragment")
	flags := commandFlags(command, commandUsage[command], out)
	section := flags.String("section", ".", "containing section")
	id := flags.String("id", "", "stable fragment identifier")
	title := flags.String("title", "", "fragment title")
	kind := flags.String("type", "markdown", "markdown, html, svg, image, or text")
	mediaType := flags.String("media-type", "", "explicit media type")
	source := flags.String("source", "", "source file to copy into the fragment")
	entrypointFlag := flags.String("entrypoint", "", "entrypoint within a source directory")
	name := flags.String("name", "", "fragment directory name without .fragment")
	order := flags.Int("order", 0, "display order")
	epic, app := scope.placeFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[command])
	}
	scope, err := scope.placed(epic, app)
	if err != nil {
		return err
	}
	entrypoint, content, resolvedType, err := fragmentContent(*kind, *mediaType, *source)
	if err != nil {
		return err
	}
	if *entrypointFlag != "" {
		// Entrypoints use the format's portable slash-path grammar. Do not
		// normalize a Windows backslash into a different, accepted path.
		entrypoint = *entrypointFlag
	}
	if reason := saga.EntrypointError(entrypoint); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	if !saga.ValidMediaType(resolvedType) {
		return fmt.Errorf("unsupported fragment media type %q", resolvedType)
	}
	var created, createdTarget string
	err = authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		sectionDir, _, err := scope.resolveTarget(document, *section, false)
		if err != nil {
			return err
		}
		fragmentName, fragmentID := *name, *id
		if fragmentName == "" {
			fragmentName = store.Slug(firstNonEmpty(*title, fragmentID, *kind))
		}
		if fragmentID == "" {
			sectionName := filepath.ToSlash(*section)
			if scope.design {
				sectionName = "design-" + sectionName
			}
			fragmentID = store.Slug(document.Manifest.ID + "-" + strings.ReplaceAll(sectionName, "/", "-") + "-" + fragmentName)
		}
		if !saga.ValidID(fragmentID) || targetIDExists(document, fragmentID) {
			return fmt.Errorf("fragment id %q is invalid or already used", fragmentID)
		}
		fragmentDir := filepath.Join(sectionDir, store.Slug(fragmentName)+".fragment")
		manifest := saga.FragmentManifest{Version: saga.CurrentVersion, ID: fragmentID, Title: *title, MediaType: resolvedType, Entrypoint: entrypoint, Order: *order}
		err = createFragment(document.Root, fragmentDir, manifest, *source, content)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("fragment %s already exists", fragmentDir)
		}
		rel, _ := filepath.Rel(document.Root, fragmentDir)
		created = filepath.ToSlash(rel)
		createdTarget = saga.FragmentTarget(document.Manifest.ID, fragmentID)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added fragment %s\n", created)
	fmt.Fprintf(out, "Target: %s\n", createdTarget)
	fmt.Fprintf(out, "Next: %s --target %s --source FILE|- %s\n", scope.commandText("set-fragment-content"), createdTarget, flags.Arg(0))
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func Validate(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("validate", commandUsage["validate"], out)
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	fix := flags.Bool("fix", false, "add missing stable anchors to Markdown headings in narrative fragments")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["validate"])
	}
	fixes := []AnchorFix{}
	if *fix {
		applied, err := fixHeadingAnchors(flags.Arg(0))
		if err != nil {
			return err
		}
		fixes = applied
	}
	document, validation, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	appendPrototypeIssues(flags.Arg(0), document, &validation)
	appendQualityIssues(flags.Arg(0), document, &validation)
	appendAppIssues(flags.Arg(0), document, &validation)
	if *jsonOutput {
		if err := writeJSON(out, validationOutput{Validation: validation, Fixes: fixes}); err != nil {
			return err
		}
	} else {
		for _, applied := range fixes {
			fmt.Fprintf(out, "Fixed %s:%d heading %q now anchors as {#%s}\n", applied.Path, applied.Line, applied.Heading, applied.Anchor)
		}
		fmt.Fprintln(out, map[bool]string{true: "Valid saga", false: "Invalid saga"}[validation.Valid])
		for _, issue := range validation.Issues {
			fmt.Fprintf(out, "  %s: %s: %s\n", issue.Severity, issue.Path, issue.Message)
		}
	}
	if !validation.Valid {
		return &StatusError{Code: 1}
	}
	return nil
}

// appendPrototypeIssues reports the prototype capability through
// the same validation surface as the rest of the Saga. The prototype loader
// validates every identity, immutable revision, html digest, and pinned
// annotation as it reads, so a load failure is the validation result.
func appendPrototypeIssues(root string, document *saga.Saga, validation *saga.Validation) {
	if document == nil {
		return
	}
	if _, err := prototypes.Load(root, ""); err != nil {
		appendLoadIssues(validation, "___requirements/prototypes", err)
	}
}

// appendQualityIssues reports the quality capability. The quality
// loader validates identities, revision/lifecycle/run graphs, step history,
// evidence and policy supersession as it reads, so a load failure is the
// validation result. An epic without a ___quality root has simply not adopted it.
func appendQualityIssues(root string, document *saga.Saga, validation *saga.Validation) {
	if document == nil {
		return
	}
	if _, err := quality.Load(root); err != nil {
		appendLoadIssues(validation, quality.RootDir, err)
	}
}

// appendAppIssues reports the app-level records and the requirements that
// depend on them: personas, flags, stories and the personas they serve,
// app-unique IDs across epics, and the records onboarding Items explain.
func appendAppIssues(root string, document *saga.Saga, validation *saga.Validation) {
	if document == nil {
		return
	}
	if _, err := requirements.Load(root, document.Manifest.ID); err != nil {
		appendLoadIssues(validation, "___requirements", err)
		return
	}
	for _, deck := range document.Onboarding {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if item.Record == "" {
					continue
				}
				if err := requireAppRecord(root, document.Manifest.ID, item.Record); err != nil {
					validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: item.Path, Message: "onboarding item: " + err.Error()})
					validation.Valid = false
				}
			}
		}
	}
}

func appendLoadIssues(validation *saga.Validation, path string, err error) {
	for _, message := range strings.Split(err.Error(), "\n") {
		validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: path, Message: message})
	}
	validation.Valid = false
}

// AnchorFix is one heading that --fix gave a stable anchor.
type AnchorFix struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Heading string `json:"heading"`
	Anchor  string `json:"anchor"`
}

// validationOutput keeps validate --json a single JSON value while still
// reporting what --fix changed. "fixes" is always present, empty when nothing
// was rewritten, so a consumer never has to test for the key.
type validationOutput struct {
	saga.Validation
	Fixes []AnchorFix `json:"fixes"`
}

// fixHeadingAnchors is the only mutating part of validate. It rewrites narrative
// Markdown fragments in place and deliberately never touches review-overlay
// fragments: thread messages are append-only history, not authored content.
func fixHeadingAnchors(root string) ([]AnchorFix, error) {
	var applied []AnchorFix
	err := authorMutation(root, func(document *saga.Saga) error {
		var walk func(*saga.Section) error
		walk = func(section *saga.Section) error {
			for _, fragment := range section.Fragments {
				if fragment.MediaType != "text/markdown" || fragment.Entrypoint == "" {
					continue
				}
				entrypoint := filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))
				content, err := os.ReadFile(entrypoint)
				if err != nil {
					// A fragment whose entrypoint cannot be read is already an
					// invalid saga; report that through validation rather than
					// failing the fix of every other fragment.
					continue
				}
				fixed, added := saga.FixMarkdownHeadingAnchors(content, reservedLandmarkIDs(fragment))
				if len(added) == 0 {
					continue
				}
				if err := store.WriteFile(entrypoint, fixed, 0o644, false); err != nil {
					return err
				}
				for _, anchor := range added {
					applied = append(applied, AnchorFix{Path: filepath.ToSlash(filepath.Join(fragment.Path, fragment.Entrypoint)), Line: anchor.Line, Heading: anchor.Heading, Anchor: anchor.Anchor})
				}
			}
			for _, child := range section.Children {
				if err := walk(child); err != nil {
					return err
				}
			}
			return nil
		}
		return walk(document.Section)
	})
	return applied, err
}

// reservedLandmarkIDs keeps a generated heading anchor from claiming an id a
// non-heading landmark already owns, which the loader rejects as a conflict.
func reservedLandmarkIDs(fragment *saga.Fragment) map[string]bool {
	reserved := map[string]bool{}
	for index := range fragment.Landmarks {
		landmark := &fragment.Landmarks[index]
		if landmark.Selector.Type != "heading" {
			reserved[landmark.ID] = true
		}
	}
	return reserved
}

func Status(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("status", commandUsage["status"], out)
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	maxItems := flags.Int("max", 100, "maximum uncovered items and next actions in text mode; 0 means all")
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	opening := registerOpenFlags(flags)
	allowRepositoryMismatch := flags.Bool("allow-repository-mismatch", false, "use a checkout whose origin differs from the declared repository")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["status"])
	}
	status, err := buildStatus(ctx, flags.Arg(0), *repoDir, opening.rng(), *allowRepositoryMismatch)
	if err != nil {
		return err
	}
	if *jsonOutput {
		if err := writeJSON(out, status); err != nil {
			return err
		}
	} else {
		printReport(out, status.Report, status.Opening, *maxItems)
		printComparison(out, status.Comparison, *maxItems)
		printLivingStatus(out, status, *maxItems)
	}
	if !status.readyForReview() {
		return &StatusError{Code: 3}
	}
	return nil
}

func buildReport(ctx context.Context, root, repoDir string, rng gitdiff.Range, allowRepositoryMismatch ...bool) (coverage.Report, error) {
	value, err := readComparison(ctx, root, repoDir, rng, len(allowRepositoryMismatch) > 0 && allowRepositoryMismatch[0])
	if err != nil {
		return coverage.Report{}, err
	}
	return value.report, nil
}

func shortOID(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

// printCursor says which code commit a companion Saga documents.
func printCursor(out io.Writer, view opening) {
	switch {
	case !view.Companion:
	case view.Cursor == "":
		fmt.Fprintln(out, "Companion Saga with no sync cursor; run change-saga sync --repo PATH after documenting the code")
	case view.Cursor != view.HeadOID:
		fmt.Fprintf(out, "Companion Saga documents %s, not head; run change-saga sync after updating it\n", shortOID(view.Cursor))
	default:
		fmt.Fprintf(out, "Companion Saga documents head %s\n", shortOID(view.Cursor))
	}
}

func printReport(out io.Writer, report coverage.Report, view opening, maxItems int) {
	if view.Mode == gitdiff.ModeObserve {
		fmt.Fprintf(out, "OBSERVING %s (%s) — no change to account for; pass --against REV to compare\n", view.Head, shortOID(view.HeadOID))
		printCursor(out, view)
		fmt.Fprintf(out, "Stale references: %d  Remapped: %d\n", report.Summary.Stale, report.Summary.Remapped)
	} else {
		state := "MAPPING GAPS"
		if report.Complete {
			state = "ALL ATOMS MAPPED"
		}
		fmt.Fprintf(out, "COMPARING %s..%s (merge-base %s)\n", view.Against, view.Head, shortOID(view.BaseOID))
		printCursor(out, view)
		fmt.Fprintf(out, "%s — %d/%d product changes mapped\n", state, report.Summary.Covered, report.Summary.Total)
		fmt.Fprintln(out, "Mapping detects omissions; it does not establish explanation quality or correctness.")
		fmt.Fprintf(out, "Uncovered: %d  Overlapping: %d  Stale references: %d  Remapped: %d  Saga-only changes: %d\n", report.Summary.Uncovered, report.Summary.Overlapping, report.Summary.Stale, report.Summary.Remapped, report.Summary.SagaChanges)
	}
	if len(report.SchemaIssues) > 0 {
		fmt.Fprintln(out, "\nSchema issues:")
		for _, issue := range report.SchemaIssues {
			fmt.Fprintf(out, "  %s: %s: %s\n", issue.Severity, issue.Path, issue.Message)
		}
	}
	if len(report.Uncovered) > 0 {
		fmt.Fprintln(out, "\nUncovered changes:")
		limit := len(report.Uncovered)
		if maxItems > 0 && maxItems < limit {
			limit = maxItems
		}
		for _, atom := range report.Uncovered[:limit] {
			fmt.Fprintf(out, "  %s\n    %s\n", coverage.DescribeAtom(atom), atom.Ref)
		}
		if limit < len(report.Uncovered) {
			fmt.Fprintf(out, "  … and %d more (use --max 0 or --json)\n", len(report.Uncovered)-limit)
		}
	}
	if len(report.StaleReferences) > 0 {
		fmt.Fprintln(out, "\nStale code references:")
		for _, stale := range report.StaleReferences {
			fmt.Fprintf(out, "  %s reference %d (%s): %s\n", stale.Assignment.EvidenceFile, stale.Assignment.Reference, stale.Reference.Location(), stale.Reason)
		}
	}
}

// Serve runs the loopback reviewer. It is reached as both "serve" and "open":
// open launches a managed background reviewer and browser, while serve stays
// attached by default. The flag set is named after the command the user typed
// so its help describes that public behavior.
func Serve(ctx context.Context, args []string, out io.Writer, openByDefault ...bool) error {
	opensBrowser := len(openByDefault) > 0 && openByDefault[0]
	detach := false
	name := "serve"
	if opensBrowser {
		name = "open"
		var err error
		args, detach, err = normalizeLegacyOpenDetach(args)
		if err != nil {
			return err
		}
	}
	if name == "serve" && len(args) > 0 && (args[0] == "status" || args[0] == "stop") {
		return manageDetachedServers(ctx, args[0], args[1:], out)
	}
	flags := commandFlags(name, commandUsage[name], out)
	addr := flags.String("addr", "127.0.0.1:7342", "loopback listen address; remote serving is disabled")
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	opening := registerOpenFlags(flags)
	openBrowser := flags.Bool("open", opensBrowser, "open the review in a browser")
	if !opensBrowser {
		flags.BoolVar(&detach, "detach", false, "run in the background and return the PID and active URL")
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	if detach {
		if !flagWasSet(flags, "addr") {
			*addr = "127.0.0.1:0"
		}
		return startDetachedServer(ctx, flags.Arg(0), *repoDir, opening.rng(), *addr, *openBrowser, out)
	}
	if statePath, token := os.Getenv(runtimeStateEnv), os.Getenv(runtimeTokenEnv); statePath != "" && token != "" {
		return runManagedServer(ctx, flags.Arg(0), *repoDir, opening.rng(), *addr, *openBrowser, statePath, token, out)
	}
	return reviewserver.ListenManaged(ctx, flags.Arg(0), *repoDir, *addr, *openBrowser, out, reviewserver.ManagedOptions{Range: opening.rng()})
}

// normalizeLegacyOpenDetach keeps pre-0.0.9 `open --detach` calls working
// without carrying the implementation detail in the public open command. An
// explicit false retains the old foreground behavior; `serve --open` is the
// documented spelling for that mode.
func normalizeLegacyOpenDetach(args []string) ([]string, bool, error) {
	detach := true
	normalized := make([]string, 0, len(args))
	afterTerminator := false
	for _, arg := range args {
		if afterTerminator {
			normalized = append(normalized, arg)
			continue
		}
		if arg == "--" {
			afterTerminator = true
			normalized = append(normalized, arg)
			continue
		}
		if arg == "-detach" || arg == "--detach" {
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if hasValue && (name == "-detach" || name == "--detach") {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return nil, false, fmt.Errorf("invalid value %q for -detach: %w", value, err)
			}
			detach = parsed
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized, detach, nil
}

func Spec(args []string, out io.Writer) error {
	flags := commandFlags("spec", commandUsage["spec"], out)
	jsonOutput := flags.Bool("json", false, "emit the contract vocabulary as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s", commandUsage["spec"])
	}
	if *jsonOutput {
		return writeJSON(out, map[string]any{
			"version": saga.SagaVersion, "manifest": saga.ManifestName, "component_version": saga.ComponentVersion, "chapter_suffix": ".chapter", "chapter_manifest": "chapter.json", "fragment_suffix": ".fragment", "fragment_manifest": "fragment.json",
			"hierarchy":            []string{"overview", "chapter", "section", "fragment"},
			"media_types":          []string{"text/markdown", "text/html", "text/plain", "image/svg+xml", "image/*"},
			"target_scheme":        "urn:change-saga",
			"code_reference":       map[string]any{"fields": []string{"commit", "path", "start", "end", "digest"}, "location": "<commit>:<path>[#L<start>[-L<end>]]", "digest": coderef.DigestPrefix + "<hex>"},
			"reviewer_bootstrap":   "README.md",
			"reserved_directories": []string{saga.CodeDirName, "___claims", "___verifications", saga.MergesDir},
			"app_layout": map[string]any{
				"app_roots":    applayout.AppRootDirs,
				"epic_storage": applayout.EpicsDir + "/<id>" + applayout.EpicSuffix + "/" + applayout.EpicManifestName,
				"epic_roots":   applayout.EpicRootDirs,
				"epic_content": "report content (chapters and fragments) plus the epic roots; no URN names its epic, so IDs are unique across the app",
				"onboarding":   applayout.OnboardingDir + "/<id>" + saga.EmbeddedDeckSuffix + " with role onboarding; its Items carry a persona, epic, or story record instead of code evidence",
			},
			"author_assertions": "one claim per ___claims/*.json; one append-only result per ___verifications/*.json",
			"implementation_deck": map[string]any{
				"storage": applayout.EpicsDir + "/<epic>" + applayout.EpicSuffix + "/" + saga.EmbeddedSlidesDir + "/<id>" + saga.EmbeddedDeckSuffix, "layout": "flat", "max_basename": saga.FlatMaxBasename, "max_absolute_path": saga.FlatMaxPath,
				"categories": map[string]string{"10-d": "deck", "20-s": "slide", "30-i": "item", "40-e": "evidence"},
				"content":    "one self-contained visual file sharing its slide manifest stem",
				"visual_forms": map[string]string{
					"system-context": "actors, external systems, boundaries, and changed interfaces", "architecture": "containment, dependencies, and responsibilities",
					"data-flow": "directed inputs, transformations, storage, and outputs", "sequence": "participants, time, calls, responses, and exceptional returns",
					"state-machine": "states, events, guards, and terminal states", "entity-relationship": "entities, keys, ownership, and cardinality",
					"logic-flow": "predicates, branches, joins, loops, and outcomes", "before-after": "matched axes and the meaningful delta",
					"failure-path": "trigger, propagation, containment, cleanup, recovery, and outcome", "evidence": "claims or risks connected to tests, measurements, or observations",
				},
				"editorial_goal": "establish the system model, then maximize reviewer information gain by surfacing consequential surprises",
				"surprise_contract": map[string]any{
					"sequence":             []string{"reasonable expectation", "actual behavior", "rationale", "consequence"},
					"preferred_expression": "an evidence-bearing callout Item attached to the responsible node, edge, state, or transition",
					"grounding":            "documented behavior, established repository patterns, historical design, or a plausible reviewer mental model; never manufactured novelty",
				},
				"composition_audits": []string{"silhouette", "relationship", "surprise", "contact-sheet"},
			},
			"living": livingSpec(),
		})
	}
	fmt.Fprint(out, specText)
	return nil
}

// InstallSkill prints an agent-agnostic bootstrap prompt. The active coding
// agent owns the platform-specific skill location and format; the saga binary
// supplies the behavior contract without mutating the user's repository.
func InstallSkill(args []string, out io.Writer) error {
	flags := commandFlags("install-skill", commandUsage["install-skill"], out)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s", commandUsage["install-skill"])
	}
	_, err := io.WriteString(out, installSkillPrompt())
	return err
}

type lineRange struct{ Start, End int }

func parseRanges(value string) ([]lineRange, error) {
	var ranges []lineRange
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, "-")
		if len(parts) > 2 {
			return nil, fmt.Errorf("invalid line range %q", raw)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil || start < 1 {
			return nil, fmt.Errorf("invalid line range %q", raw)
		}
		end := start
		if len(parts) == 2 {
			end, err = strconv.Atoi(parts[1])
			if err != nil || end < start {
				return nil, fmt.Errorf("invalid line range %q", raw)
			}
		}
		ranges = append(ranges, lineRange{Start: start, End: end})
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("at least one line range is required")
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start != ranges[j].Start {
			return ranges[i].Start < ranges[j].Start
		}
		return ranges[i].End < ranges[j].End
	})
	canonical := ranges[:0]
	for _, current := range ranges {
		if len(canonical) == 0 || current.Start > canonical[len(canonical)-1].End && current.Start-canonical[len(canonical)-1].End > 1 {
			canonical = append(canonical, current)
			continue
		}
		if current.End > canonical[len(canonical)-1].End {
			canonical[len(canonical)-1].End = current.End
		}
	}
	return canonical, nil
}

func discoverRepository(ctx context.Context, repoDir, explicit string, options ...bool) (string, string, error) {
	allowLocal := len(options) > 0 && options[0]
	allowMismatch := len(options) > 1 && options[1]
	rootOutput, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("locate source repository: %s", strings.TrimSpace(string(rootOutput)))
	}
	root := strings.TrimSpace(string(rootOutput))
	if explicit != "" {
		canonical, err := coderef.CanonicalRepository(explicit)
		if err != nil {
			return "", "", fmt.Errorf("--repository: %w", err)
		}
		parsed, _ := url.Parse(canonical)
		if !allowMismatch && (parsed.Scheme == "file" || repositoryOriginAvailable(ctx, root)) {
			if err := gitdiff.VerifyRepository(ctx, root, canonical); err != nil {
				return "", "", err
			}
		}
		return canonical, root, nil
	}
	remoteOutput, remoteErr := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").CombinedOutput()
	if remoteErr == nil && strings.TrimSpace(string(remoteOutput)) != "" {
		canonical, err := normalizeRepositoryURI(strings.TrimSpace(string(remoteOutput)), root)
		if err != nil {
			return "", "", fmt.Errorf("canonicalize origin repository: %w", err)
		}
		parsed, _ := url.Parse(canonical)
		if parsed.Scheme == "file" && !allowLocal {
			return "", "", fmt.Errorf("origin resolves to a local file repository; provide a portable --repository URI or explicitly opt in with --allow-local-repository")
		}
		return canonical, root, nil
	}
	if !allowLocal {
		return "", "", fmt.Errorf("origin is unavailable; provide a portable --repository URI or explicitly opt in with --allow-local-repository")
	}
	canonical, err := coderef.FileRepository(root)
	return canonical, root, err
}

func repositoryOriginAvailable(ctx context.Context, root string) bool {
	output, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func normalizeRepositoryURI(value, root string) (string, error) {
	// On Windows, url.Parse interprets the drive letter in C:\repo as a URI
	// scheme. Recognize native absolute paths before parsing portable URIs.
	if filepath.IsAbs(value) {
		return coderef.FileRepository(value)
	}
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
		return coderef.CanonicalRepository(parsed.String())
	}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		if colon := strings.Index(value[at:], ":"); colon > 0 {
			colon += at
			return coderef.CanonicalRepository("ssh://" + value[at+1:colon] + "/" + value[colon+1:])
		}
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(root, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return coderef.FileRepository(abs)
}

func resolveTarget(document *saga.Saga, value string, allowFragment bool) (string, string, error) {
	if fragmentPath, landmarkID, ok := splitLandmarkTarget(value); ok {
		return resolveLandmark(document, fragmentPath, landmarkID, allowFragment)
	}
	if strings.HasPrefix(value, "urn:change-saga:") {
		var foundDir string
		walkTargets(document.Root, document.Section, func(target, dir string, fragment bool) {
			if target == value && (allowFragment || !fragment) {
				foundDir = dir
			}
		})
		if value == saga.SagaTarget(document.Manifest.ID) {
			foundDir = document.Root
		}
		if foundDir == "" {
			return "", "", fmt.Errorf("target %q does not exist%s", value, targetHint(document, allowFragment))
		}
		return foundDir, value, nil
	}
	if dir, target, found, err := resolveTargetID(document, value, allowFragment); found || err != nil {
		return dir, target, err
	}
	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(document.Root, candidate)
	}
	candidateAbs, _ := filepath.Abs(candidate)
	if dir, target, found := resolveTargetRecordPath(document, candidateAbs, allowFragment); found {
		return dir, target, nil
	}
	var directTarget string
	walkTargets(document.Root, document.Section, func(target, dir string, fragment bool) {
		dirAbs, _ := filepath.Abs(dir)
		if dirAbs == candidateAbs && (allowFragment || !fragment) {
			directTarget = target
		}
	})
	if directTarget != "" {
		return candidateAbs, directTarget, nil
	}
	targetKinds := map[bool]string{true: "chapter, section, fragment, or landmark", false: "chapter or section"}[allowFragment]
	dir, err := store.ResolveSection(document.Root, value)
	if err != nil {
		// Report content lives beneath reserved roots (___epics, ___overview),
		// so a missing path there fails section resolution; still point the
		// author at the query API rather than only at the path rule.
		return "", "", fmt.Errorf("target %q is not a valid %s: %v%s", value, targetKinds, err, targetHint(document, allowFragment))
	}
	abs, _ := filepath.Abs(dir)
	if abs == document.Root {
		return abs, saga.SagaTarget(document.Manifest.ID), nil
	}
	var foundTarget string
	var isFragment bool
	walkTargets(document.Root, document.Section, func(target, candidate string, fragment bool) {
		candidateAbs, _ := filepath.Abs(candidate)
		if candidateAbs == abs {
			foundTarget, isFragment = target, fragment
		}
	})
	if foundTarget == "" || isFragment && !allowFragment {
		return "", "", fmt.Errorf("target %q is not a valid %s%s", value, targetKinds, targetHint(document, allowFragment))
	}
	return abs, foundTarget, nil
}

// resolveTargetRecordPath recognizes the record filename printed by deck
// authoring commands. Every slide and Item in a deck bundle shares the bundle
// as its storage directory, so comparing Directory alone would make them
// collide. The Path field remains unique.
func resolveTargetRecordPath(document *saga.Saga, candidateAbs string, allowFragment bool) (string, string, bool) {
	var foundDir, foundTarget string
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Path != "" {
			pathAbs, _ := filepath.Abs(filepath.Join(document.Root, filepath.FromSlash(section.Path)))
			if pathAbs == candidateAbs {
				foundDir = pathAbs
				foundTarget = section.Target
			}
		}
		if allowFragment {
			for _, fragment := range section.Fragments {
				pathAbs, _ := filepath.Abs(filepath.Join(document.Root, filepath.FromSlash(fragment.Path)))
				if fragment.Path != "" && pathAbs == candidateAbs {
					foundDir, foundTarget = fragment.Directory, fragment.Target
				}
				for index := range fragment.Landmarks {
					landmark := &fragment.Landmarks[index]
					pathAbs, _ := filepath.Abs(filepath.Join(document.Root, filepath.FromSlash(landmark.Path)))
					if landmark.Path != "" && pathAbs == candidateAbs {
						foundDir, foundTarget = landmark.Directory, landmark.Target
					}
				}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return foundDir, foundTarget, foundTarget != ""
}

// resolveTargetID makes the stable IDs printed by authoring commands usable
// everywhere a target is accepted. Paths and URNs remain supported, but an
// agent no longer has to rediscover a full URN merely to put a section under a
// chapter it just created. Landmark IDs may repeat across fragments, so an
// ambiguous shorthand is rejected with the matching URNs instead of guessed.
func resolveTargetID(document *saga.Saga, value string, allowFragment bool) (string, string, bool, error) {
	if value == "" || value == "." || filepath.IsAbs(value) || strings.ContainsAny(value, `/\\#`) {
		return "", "", false, nil
	}
	type match struct{ dir, target string }
	var matches []match
	if document.Manifest.ID == value || document.Section.ID == value {
		matches = append(matches, match{document.Root, saga.SagaTarget(document.Manifest.ID)})
	}
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Path != "" && section.ID == value {
			matches = append(matches, match{filepath.Join(document.Root, filepath.FromSlash(section.Path)), section.Target})
		}
		if allowFragment {
			for _, fragment := range section.Fragments {
				if fragment.ID == value {
					matches = append(matches, match{fragment.Directory, fragment.Target})
				}
				for index := range fragment.Landmarks {
					landmark := &fragment.Landmarks[index]
					if landmark.ID == value {
						matches = append(matches, match{landmark.Directory, landmark.Target})
					}
				}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	if len(matches) == 0 {
		return "", "", false, nil
	}
	if len(matches) > 1 {
		targets := make([]string, len(matches))
		for i := range matches {
			targets[i] = matches[i].target
		}
		sort.Strings(targets)
		return "", "", true, fmt.Errorf("target id %q is ambiguous; use one of: %s", value, strings.Join(targets, ", "))
	}
	return matches[0].dir, matches[0].target, true, nil
}

// splitLandmarkTarget recognizes the <fragment>#<landmark-id> shorthand. A
// landmark lives inside a reserved ___landmarks directory that ordinary section
// resolution refuses to enter, so without this an author had to spell out the
// full landmark URN before they had any way to discover it.
func splitLandmarkTarget(value string) (string, string, bool) {
	if strings.HasPrefix(value, "urn:change-saga:") {
		return "", "", false
	}
	hash := strings.LastIndex(value, "#")
	if hash < 0 {
		return "", "", false
	}
	return value[:hash], value[hash+1:], true
}

func resolveLandmark(document *saga.Saga, fragmentPath, landmarkID string, allowFragment bool) (string, string, error) {
	if !allowFragment {
		return "", "", fmt.Errorf("landmark targets cannot contain chapters, sections, or fragments")
	}
	if strings.TrimSpace(fragmentPath) == "" {
		return "", "", fmt.Errorf("landmark target %q must name its fragment, as <fragment-path>#<landmark-id>", "#"+landmarkID)
	}
	_, fragmentTarget, err := resolveTarget(document, fragmentPath, true)
	if err != nil {
		return "", "", err
	}
	fragment := findFragmentByTarget(document.Section, fragmentTarget)
	if fragment == nil {
		return "", "", fmt.Errorf("%q is not a fragment, so it cannot contain landmark %q", fragmentPath, landmarkID)
	}
	for index := range fragment.Landmarks {
		landmark := &fragment.Landmarks[index]
		if landmark.ID == landmarkID {
			return landmark.Directory, landmark.Target, nil
		}
	}
	if len(fragment.Landmarks) == 0 {
		return "", "", fmt.Errorf("fragment %q (%s) declares no landmarks; add %s/___landmarks/%s.landmark/landmark.json before covering it", fragmentPath, fragmentTarget, fragment.Path, landmarkID)
	}
	available := make([]string, 0, len(fragment.Landmarks))
	for index := range fragment.Landmarks {
		available = append(available, fragment.Landmarks[index].ID)
	}
	return "", "", fmt.Errorf("fragment %q has no landmark %q; it declares %s", fragmentPath, landmarkID, strings.Join(available, ", "))
}

func findFragmentByTarget(section *saga.Section, target string) *saga.Fragment {
	var found *saga.Fragment
	var walk func(*saga.Section)
	walk = func(current *saga.Section) {
		for _, fragment := range current.Fragments {
			if fragment.Target == target {
				found = fragment
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(section)
	return found
}

func findFragment(section *saga.Section, dir string) *saga.Fragment {
	dirAbs, _ := filepath.Abs(dir)
	var found *saga.Fragment
	var walk func(*saga.Section)
	walk = func(current *saga.Section) {
		for _, fragment := range current.Fragments {
			if fragmentAbs, _ := filepath.Abs(fragment.Directory); fragmentAbs == dirAbs {
				found = fragment
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(section)
	return found
}

// maxTargetHints bounds the suggestion list so a large saga still produces a
// readable error instead of dumping its whole target space.
const maxTargetHints = 12

// targetHint turns "that target does not exist" into something actionable. The
// query API is the supported way to enumerate targets, so the hint names it
// rather than inviting the reader to go read metadata files.
func targetHint(document *saga.Saga, allowFragment bool) string {
	var targets []string
	targets = append(targets, saga.SagaTarget(document.Manifest.ID))
	walkTargets(document.Root, document.Section, func(target, _ string, fragment bool) {
		if allowFragment || !fragment {
			targets = append(targets, target)
		}
	})
	sort.Strings(targets)
	hint := "; run \"change-saga query children --saga " + filepath.Base(document.Root) + " --parent " + saga.SagaTarget(document.Manifest.ID) + "\" to list targets"
	if len(targets) == 0 {
		return hint
	}
	shown := targets
	suffix := ""
	if len(shown) > maxTargetHints {
		shown, suffix = shown[:maxTargetHints], fmt.Sprintf(", and %d more", len(targets)-maxTargetHints)
	}
	return fmt.Sprintf("%s. Known targets: %s%s", hint, strings.Join(shown, ", "), suffix)
}

func walkTargets(root string, section *saga.Section, fn func(target, dir string, fragment bool)) {
	// Section paths are resolved by the caller from the saga root; fragment
	// directories are already absolute because they can also live in messages.
	for _, fragment := range section.Fragments {
		fn(fragment.Target, fragment.Directory, true)
		for index := range fragment.Landmarks {
			landmark := &fragment.Landmarks[index]
			fn(landmark.Target, landmark.Directory, true)
		}
	}
	for _, child := range section.Children {
		fn(child.Target, filepath.Join(root, filepath.FromSlash(child.Path)), false)
		walkTargets(root, child, fn)
	}
}

func targetIDExists(document *saga.Saga, id string) bool {
	found := id == document.Manifest.ID || id == document.Section.ID
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Path != "" && section.ID == id {
			found = true
		}
		for _, fragment := range section.Fragments {
			if fragment.ID == id {
				found = true
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return found
}

// createFragment publishes a complete fragment package with one rename. A
// fragment that is missing its entrypoint invalidates the whole saga, so a
// half-written package must never become visible under its final name.
func createFragment(root, dir string, manifest saga.FragmentManifest, source string, content []byte) error {
	return store.CommitDir(root, dir, func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		return populateFragment(stage, manifest, source, content)
	})
}

// populateFragment fills an already-created directory. Callers that are
// themselves staging a larger entity reuse it directly.
func populateFragment(dir string, manifest saga.FragmentManifest, source string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := store.WriteJSON(filepath.Join(dir, "fragment.json"), manifest, true); err != nil {
		return err
	}
	target := filepath.Join(dir, filepath.FromSlash(manifest.Entrypoint))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if source != "" {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyFragmentPackage(source, dir); err != nil {
				return err
			}
			if entry, err := os.Stat(target); err != nil || entry.IsDir() {
				return fmt.Errorf("entrypoint %q does not exist in source directory", manifest.Entrypoint)
			}
			return nil
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}
	return os.WriteFile(target, content, 0o644)
}

// authorMutation serializes an authoring write against every other supported
// writer and re-resolves the saga under that lock, so target resolution cannot
// go stale between the read and the commit.
func authorMutation(root string, operation func(*saga.Saga) error) error {
	return store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, _, err := saga.Load(root)
		if err != nil {
			return err
		}
		return operation(document)
	})
}

func fragmentContent(kind, explicitType, source string) (string, []byte, string, error) {
	mediaType := explicitType
	if mediaType == "" {
		switch kind {
		case "markdown":
			mediaType = "text/markdown"
		case "html":
			mediaType = "text/html"
		case "svg":
			mediaType = "image/svg+xml"
		case "text":
			mediaType = "text/plain"
		case "image":
			mediaType = mime.TypeByExtension(strings.ToLower(filepath.Ext(source)))
		default:
			return "", nil, "", fmt.Errorf("unsupported fragment type %q", kind)
		}
	}
	if source != "" {
		if info, err := os.Stat(source); err == nil && info.IsDir() {
			switch mediaType {
			case "text/html":
				return "index.html", nil, mediaType, nil
			case "image/svg+xml":
				return "image.svg", nil, mediaType, nil
			default:
				return "", nil, "", fmt.Errorf("--entrypoint is required for a %s source directory", mediaType)
			}
		}
		return filepath.Base(source), nil, mediaType, nil
	}
	switch mediaType {
	case "text/markdown":
		return "content.md", nil, mediaType, nil
	case "text/plain":
		return "content.txt", nil, mediaType, nil
	case "text/html":
		return "index.html", []byte(defaultHTMLFragment), mediaType, nil
	case "image/svg+xml":
		return "image.svg", []byte(defaultSVGFragment), mediaType, nil
	default:
		return "", nil, "", fmt.Errorf("--source is required for %s", mediaType)
	}
}

func copyFragmentPackage(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fragment source cannot contain symlink %s", rel)
		}
		if strings.HasPrefix(entry.Name(), "___") || rel == "fragment.json" || rel == "slide.json" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		destination := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

const defaultHTMLFragment = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><style>body{font:16px system-ui;margin:0;padding:24px}button{font:inherit}</style></head>
<body><h1>Interactive fragment</h1><p>Bundle JavaScript, CSS, images, and data beside this file.</p><button id="demo">Clicks: 0</button><script>let n=0;document.querySelector('#demo').onclick=e=>e.target.textContent='Clicks: '+(++n)</script></body></html>
`

// installSkillPrompt renders the bootstrap prompt. The query operation list is
// generated from the same table the query command dispatches on, so an
// operation can never be added to the CLI and left out of the installed skill.
func installSkillPrompt() string {
	var operations strings.Builder
	for _, operation := range queryOperations {
		fmt.Fprintf(&operations, "   - `%s` — %s\n     `%s`\n", operation, queryPurpose[operation], queryUsage[operation])
	}
	return strings.Replace(installSkillTemplate, "{{query_operations}}\n", operations.String()+"\n", 1)
}

const installSkillTemplate = `Install or update a project-local agent skill named "change-saga" using this coding agent's native skill mechanism. Do not create a Change Saga as part of installation. Preserve the following behavior in the installed skill:

# Change Saga authoring

A Change Saga is the authored change proposal submitted for review: a visual,
executable successor to a flat pull-request title and description. It is the
thing to be reviewed, not the review itself.

## Implementation deck

The implementation deck is the Saga's Implementation section. Its spine is
Deck → Slide → Item. Author with ` + "`add-deck`" + `, ` + "`add-slide`" + `,
` + "`set-slide-content`" + `, and ` + "`add-item`" + `.
Each slide is one 16:9 visual argument with one takeaway and no more than seven
semantic Items in a standard layout. Nodes, edges, regions, transitions,
examples, risks, metrics, statements, and overlaid callouts are all Items. A
callout may name another Item and may own its own code references. Reference
every changed line in the deck from the narrowest Item; deck- and
slide-level coverage is refused. Read it with the ` + "`slide`" + ` and ` + "`slide-diffs`" + ` query
operations.

Each deck is an independently mergeable bundle under ` + "`___slides/<id>.deck/`" + `
with compact category-prefixed records (` + "`10-d`" + ` decks, ` + "`20-s`" + ` slides,
` + "`30-i`" + ` Items, ` + "`40-e`" + ` evidence, and ` + "`80`" + `–` + "`85`" + ` reviews at the Saga root).
Titles and source paths belong inside records, never filenames. Always use CLI
commands and query targets; never invent, rename, nest, glob, or infer meaning
from deck storage files. Slide HTML must be self-contained.

Optimize for reviewer understanding and information gain, not exhaustive
retelling. Establish enough of the surrounding system for the reviewer to form
an accurate mental model, then foreground where that model may break:
counterintuitive behavior, hidden coupling, consequential constraints,
intentional deviations from repository conventions, rejected alternatives,
and tradeoffs whose costs land elsewhere. For each meaningful surprise, show
the reasonable reviewer expectation, actual behavior, rationale, and consequence. A
surprise is especially effective as a callout Item attached to the node, edge,
state, or transition that creates it; give the callout exact evidence when it
makes a code-backed claim. Do not manufacture novelty when the investigation
finds no material deviation.

### Storyboard visual questions before creating slides

Do not start by choosing a reusable SVG template. First inspect the change and
write a private storyboard. For every proposed slide, name:

- the specific reviewer question it answers, not merely its topic;
- its rhetorical intent and one-sentence takeaway;
- whether it establishes the system model or resolves a specific reviewer
  surprise, including expectation, actuality, rationale, and consequence;
- the relationship the picture must make visible;
- the visual form that truthfully encodes that relationship; and
- the meaningful nodes, edges, states, regions, or callouts that will become
  evidence-bearing Items.

Choose visual form from the explanation, not from styling convenience:

- a system-context diagram shows actors, external systems, trust or ownership
  boundaries, and the changed interface;
- an architecture/composition diagram shows containment, dependencies,
  responsibilities, and what moved or was introduced;
- a data-flow diagram shows direction, inputs, transformations, storage, and
  outputs;
- a sequence diagram shows participants, time ordering, calls, responses, and
  exceptional returns;
- a state machine shows states, labeled events, guards, and terminal states;
- an entity-relationship diagram shows entities, keys, ownership, and
  cardinality;
- a logic or decision flow shows predicates, branches, joins, loops, and
  outcomes;
- a before/after comparison uses matched axes and highlights the meaningful
  delta;
- a failure-path diagram traces trigger, propagation, containment, cleanup,
  recovery, and observable outcome; and
- an evidence view connects a concrete claim or risk to tests, measurements,
  or observable results.

` + "`intent`" + ` states the slide's job and ` + "`layout`" + ` states its canvas arrangement;
neither is a substitute for the correct visual form. A grid or row of labeled
cards is valid only when category membership or matched comparison is itself
the relationship being explained. Do not use cards as a universal container
for architecture, flow, lifecycle, data, or failure semantics. Boxes connected
only by reading order are an outline, not a diagram.

Before handoff, run three visual audits:

1. **Silhouette test:** mentally remove labels, prose, and color. The remaining
   topology should still communicate whether this is containment, flow,
   sequence, state, entity structure, branching, or comparison.
2. **Relationship test:** every relationship essential to the takeaway is
   visibly encoded with an edge, boundary, lane, nesting, cardinality, axis,
   or transition—not left to nearby prose.
3. **Surprise test:** after reading the deck, a reviewer can name the system
   model, the highest-consequence deviation from likely expectation, why it
   exists, and the tradeoff it creates.
4. **Contact-sheet test:** inspect all slides together. Reuse a visual grammar
   only when the underlying relationship is genuinely the same. If unrelated
   slides reduce to the same number and arrangement of cards, rewrite them.

Perform these audits before chasing complete diff coverage. Coverage is the
final omission check; it must not rationalize a generic visual after the fact.

## One Saga, from the big work to the big work

A Change Saga is the record of one big change: the kind that warrants product
requirements, UX and UI design, technical design, quality verification, and an
implementation walkthrough. There is one kind of Saga. It always has the same
four parts, in the same order:

- **Product**: prototypes, and user stories with acceptance criteria;
- **Design**: UX flows, UI references, and technical design;
- **Quality**: test cases that verify the acceptance criteria;
- **Implementation**: the deck that explains the change, whose Items reference
  the code.

The only hard requirement is that code maps back to user stories. Designs,
specifications, and test cases map to stories, so code reaches a story
transitively through them. Do not author code-to-story links by hand when a
design or test case can carry the path. Every relation pins the revision it
relied on, so a story revision makes its dependents visibly stale.

This is not a waterfall. Prototypes and stories may come in either order and
iterate together; design starts while they mature; a discovery during
implementation becomes an explicit new revision of the story it changes,
preserving history rather than rewriting it. When the implementation already
exists, build the Saga the same way: the product intent and design are what the
reviewer needs in order to judge the code, so recover them from the source
material and the user rather than skipping them.

Parallel authoring is a core property of the document. Partition ownership by
stable stories, prototypes, design fragments, test cases, work items, and deck
bundles so agents can fan out and merge their Saga changes alongside the code.
Consolidate the lanes before the final status and validation passes.

## Drive the work with status

` + "`" + `change-saga status --json <saga>` + "`" + ` is the work queue. It reports the readiness
gates, each accepted criterion's coverage on the prototype, UX, UI, technical,
quality, and implementation axes, the stale set with pinned and current
revisions, changed-source accounting, and ordered ` + "`" + `next_actions` + "`" + `. Loop:

1. run ` + "`" + `change-saga spec --json` + "`" + ` once to learn the resources, legal relations,
   and command shapes;
2. run ` + "`" + `status --json` + "`" + ` and take the first next action;
3. a ` + "`" + `command` + "`" + ` action carries a valid command shape: fill in its author inputs
   and run it; a ` + "`" + `question` + "`" + ` action needs product judgment, external access, or
   an explicit exclusion: ask the user its one question and run the command for
   their answer;
4. run ` + "`" + `validate` + "`" + `, then repeat from step 2.

Stop when no required gap remains. A clean status proves nothing is missing or
stale; it never proves the Saga is good. Status never reduces coverage to a
score, and neither should you.

When a user asks to draft, prepare, or create a pull request or otherwise make a
big change ready for review, use the repository's existing PR-authoring
processes, templates, issue context, conventions, and checks, but express the
result as a Change Saga. Do not replace useful existing authoring discipline;
extend it into this format.

During authoring, speak as the change author and guide. Do not create review
comments, findings, approvals, rejections, or other review-overlay records.
Only perform those actions when the user explicitly asks to conduct a review of
an already-authored saga.

Use the installed "change-saga" CLI as the source of truth. Begin with
"change-saga --help" and "change-saga spec" when necessary. Resolve the exact PR, branch, commit range, or
working-tree comparison; inspect the full change and its existing PR context;
then initialize and author the .saga directory.

## Read the saga through the versioned query API

Never read, glob, or grep saga metadata files to learn what a saga contains.
Files under a .saga directory are an implementation detail and their layout is
not a compatibility promise. Use "change-saga query", the versioned read API,
for every read. It is deterministic, paginated, and safe to call concurrently;
it never starts a server and never mutates either repository.

Pass "--saga <path>" to every query, and "--repo <source-checkout>" when the
source repository is separate from the saga. The exception is "change-saga
query schema <operation>", which describes the operation's data paths and
pagination contract without opening a saga. Use it instead of probing or
guessing response shapes. Each cursor schema names the response collection
counted by "page.total" and "page.returned" as "pagination.counted_path".
Every invocation writes exactly one JSON envelope
carrying "schema", "ok", "snapshot", "data", and "page"; failures carry
"error.code". Branch on "ok" and "error.code"; never parse message text. For
cursor-paginated operations, the current page length at
"pagination.counted_path" must equal "page.returned". Follow
"page.next_cursor" while "page.has_more" is true and confirm the aggregate
count equals "page.total". Do not raise "--limit"
to silently swallow a partial result. Compare "snapshot" across calls to detect
a saga that changed underneath a multi-step read.

The operations are:

{{query_operations}}
Start at "query overview", walk one level at a time with "query children", and
read narrative content through "query fragment". "query children" is also how
you discover landmark targets: a fragment's children are its landmarks, and each
one reports the target URN to pass to "change-saga cover --target".
Hierarchy nodes report inclusive "diffs.current" and "diffs.stale" totals plus
"direct_current", "direct_stale", "descendant_current", and
"descendant_stale", so evidence owned by a landmark is not mistaken for a
fragment with no explained code.

## Author the saga

Use this authoring loop, consulting each command's "-h" output for exact flags:

1. "change-saga init" records the repository and title. The Saga stores no
   comparison: every command that reads one takes "--against REV" (and
   optionally "--head REV", default HEAD) and compares the merge-base of the
   two with head, the way a pull request does. Without "--against" a command
   observes the app at head, with no changed lines to account for.
2. "change-saga query gaps --kind uncovered --against main --saga <path>" pages the coverage
   work queue. Use "--kind stale" for reconciliation and "--kind overlap" for
   mappings that need justification; preserve the returned snapshot across the
   loop.
3. "change-saga add-chapter", "change-saga add-section", and "change-saga add-fragment" build the
   reviewer-oriented narrative and its Markdown, SVG, image, or HTML packages.
   Write or replace an entrypoint only with "change-saga set-fragment-content
   --target TARGET --source FILE|-"; do not edit fragment package files directly.
4. "change-saga add-landmark" makes a Markdown heading, HTML/SVG element, exact
   text, or image region independently addressable. Give every meaningful
   visual landmark a semantic "--description" that explains its role without
   relying on geometry, color, or position. Every concrete prose claim about
   implementation, behavior, an invariant, or a data transition must carry a
   focused Markdown footnote citation or live under a deliberately
   evidence-bearing heading. Make each plain-text footnote definition an
   exact-text landmark and attach its exact evidence there. Do not finish a
   substantive implementation narrative with zero citations merely because
   its atoms are covered at fragment scope. Give each code-bearing SVG/HTML
   node, edge, arrow, transition, or state its own stable element ID and
   evidence-bearing landmark instead of covering only the enclosing fragment.
   SVG element bounds become on-canvas links automatically; use "--hotspot"
   only to override awkward geometry.
   Apply the same completion rule to prose citations and visual nodes: a
   footnote marker and definition are not linked until the definition is an
   exact-text landmark with focused code references. Repair every per-footnote validation
   warning before handoff. Requirements provenance from "citation add" records
   source context and does not replace implementation evidence.
5. "change-saga cover" references the code a focused fragment or landmark
   explains, pinned at a commit with a digest of its content, and includes a
   concise what-and-why note. "--side new --lines" pins added lines at the
   comparison's head; "--side old --lines" pins deleted lines at its
   merge-base; "--file" references a whole file for renames, mode and binary
   changes. "--target" accepts a path, a target URN, or the
   "<fragment-path>#<landmark-id>" shorthand. Use "--dry-run" to see exactly
   which records an invocation would write before writing them. When every
   changed line in one file genuinely belongs to the same target, "--path FILE
   --changed-lines" references exactly those lines in dense ranges, and whole
   files for file events such as "add". Never use it to hide multiple concerns
   in one broad record. Generated evidence paths identify their reference set,
   so a second explanation for the same references requires "replace-coverage"
   rather than creating a duplicate record.
6. When referencing many ranges, pipe newline-delimited JSON records to
   "change-saga cover --batch -". Each record carries its own "target", "path",
   "side", "lines", "changed_lines", "file", "commit", "refs", "note", and
   "name". The whole batch is resolved before anything is written and a failing
   record leaves the saga untouched, so a batch is a delivery optimization only:
   every record still references exactly the code it explains.
7. Run "change-saga query mappings --sort scrutiny" and use each
   "evidence_file" as the stable repair handle. "change-saga replace-coverage
   --record PATH --batch -" atomically splits, retargets, or rewrites a record;
   "change-saga remove-coverage --record PATH" deletes one. Prefer
   landmark-level ownership when the score explains that a mapping deserves
   more skepticism. The score is a work queue, not a correctness grade.
   Then use "query children" and "query fragment-diffs" to audit every
   substantive fragment. Move direct fragment-level evidence to citations,
   headings, SVG nodes, or SVG edges whenever the authored content identifies
   that narrower target.
   References follow their code: when later commits only move the referenced
   lines they are remapped automatically. When the lines themselves change the
   reference is stale; "change-saga references --stale --diff" shows why, and
   "replace-coverage" re-authors it. After the change lands, "change-saga repin
   --onto <landed commit>" re-pins evidence to that commit and records the
   branch's commit messages.
8. Record falsifiable assertions with "change-saga add-claim" and append an
   explicit result with "change-saga verify-claim". Claims never contribute to
   coverage. Use "unverified" when an assertion has not actually been checked;
   otherwise record the reproducible method and command when applicable.
9. Repeat status, then run "change-saga validate --json" before "change-saga open" presents
   the authored proposal for review. "change-saga validate --fix" adds missing
   stable anchors to Markdown headings and changes nothing else.

## Maintain a codebase Saga

"change-saga status --json --against <base> <app.saga>" scopes the work queue
to one change and reports three layers under "comparison": "changed" (Saga
records added, revised, or retired, each with its before and after),
"affected" (records the change did not edit but invalidated: pinned to a
revision that changed, or referencing code that changed, followed up the
persona, story, design, and code chain), and "code" (the changed hunks grouped
under the nodes that reference them, plus every changed line nothing
references). Update the affected records, then cover the unreferenced lines.
When the Saga lives in its own repository, pass the code checkout with
"--repo" and move the sync cursor with "change-saga sync" in every Saga commit
that updates the documentation.

Lead with pictures and show by example. The root should establish the goal,
system/change map, affected workflows, and chapter path before dense prose.
Every substantial chapter should begin with an SVG diagram, self-contained
interactive HTML walkthrough, or concrete before/after example. Highlight
end-to-end workflows, data flows, data models, state transitions, boundaries,
failure paths, compatibility, and observable outcomes. Give meaningful diagram
nodes, edges, and interactive elements stable landmarks with "change-saga
add-landmark" so they can link to the exact code that realizes them. In prose,
use Markdown footnote citations for every concrete implementation claim and map
the exact-text reference definition to the code it cites; the renderer makes
both the inline marker and reference entry open that code. A
citation-free implementation narrative is unfinished even when status reports
complete coverage. Audit every visual before moving on: enumerate its
meaningful nodes and edges and attach focused evidence to each code-bearing
landmark. A visual with no landmarks or direct code mapping is unfinished, not
decorative completeness.

Organize the change into independently reviewable chapters based on behavior,
risk, architecture, or reviewer intent rather than file type. Use Markdown to
orient and connect visual artifacts, not as the default container for the whole
explanation.

Run "change-saga query gaps --kind uncovered --saga <name>.saga" as the coverage
work queue. Reference only the code a fragment or landmark explains, with
concise notes saying what changed and why that content owns it. Never widen
mappings only to reach 100 percent. Iterate until every product change is
mapped and no mapping is stale, then inspect "query mappings --sort scrutiny".
All-atoms-mapped is an omission invariant, not proof of explanation quality or
correctness. Then run "change-saga validate --json"
and "change-saga status --json" before handoff.

Use "--json" for bounded machine-readable coverage mutation summaries and
"--quiet" when no successful output is needed. "change-saga open" starts a
managed background reviewer; inspect or stop it with "change-saga serve
status" and "change-saga serve stop". Use "change-saga serve --open" only
when the reviewer should remain attached to the current terminal.

Never leave generated instructions, blank scaffold fragments, or example
diagram/HTML content in the handed-off saga. Treat every validation warning as
an authoring task unless it is explicitly justified. For a PR request, verify
the provider-reported head OID/branch and changed files match the inspected
checkout; never guess a PR number, and omit PR metadata rather than recording
an unverified identity.

Opening the saga presents the authored proposal for a human to review. It does
not authorize the agent to conduct that review. When explicitly asked to review
one, first read the code diff independently and record provisional findings;
then inspect mappings, claims, verifications, and narrative intent; finally
reconcile contradictions and independently test author claims. Do not let the
author's explanation anchor the first correctness pass.
When recording a decision, always declare the reviewer persona. Use
"--reviewer-kind human" only for a decision the human made directly. For the
agent's own review, use "--reviewer-kind ai" together with an independent
"--reviewer-name", "--agent", and the exact "--model"; never turn an AI pass
into a human approval. Give simultaneous passes stable distinct names such as
"Claude 1" and "Claude 2" even when their model is identical. Multiple reviewers
may decide the same target, and one persona's later decision supersedes only
that same persona's prior decision.
`

const defaultSVGFragment = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 300" role="img" aria-label="Diagram placeholder">
  <rect width="800" height="300" rx="24" fill="#eef3ee"/><text x="400" y="150" text-anchor="middle" dominant-baseline="middle" font-family="system-ui" font-size="28" fill="#244235">Replace with a useful diagram</text>
</svg>
`

const specText = `Change Saga format (experimental)

A Change Saga is one directory whose saga.json declares version 5. It carries
the whole of a big change: prototypes and user stories under ___requirements,
UX/UI and technical design under ___design, delivery waves under ___workplan,
test cases under ___quality, and the implementation deck under ___slides.

The implementation deck is a set of independently mergeable bundles under
___slides/<id>.deck/. Compact prefixes group decks (10-d), slides (20-s), Items
(30-i), and evidence (40-e); their review history (80-85) lives at the Saga
root. Fixed-width ranks and deterministic keys make ordinary filename sorting
stable; semantic IDs, titles, parentage, and durable target URNs remain in the
records. Basenames are at most 64 characters and the default portability
budget is 240 characters for an absolute path. Each slide owns one
self-contained SVG, image, or HTML file sharing its manifest stem. Deck
evidence may target only Items.

For authoring, ` + "`intent`" + ` names the reviewer job and ` + "`layout`" + ` names the canvas
arrangement; neither names the diagram's meaning. Storyboard the specific
question, relationship, and visual form before creating a slide. Use system
boundaries for context, containment and dependencies for architecture,
directed transformations for data flow, lanes and messages for sequence,
labeled transitions for state, keys and cardinality for entity relationships,
branches for logic, matched axes for before/after, and
trigger-to-recovery paths for failure behavior. A repeated row of cards is not
a neutral visual language. Audit silhouette, encoded relationships, and the
whole contact sheet before treating coverage as complete.

Optimize the deck for reviewer understanding and surprise reduction. Establish
the minimum system model, then foreground evidence-backed gaps between a
reasonable reviewer expectation and the actual behavior: hidden coupling,
counterintuitive outcomes, consequential constraints, tradeoffs, and deliberate
deviations from repository norms. Show expectation, actual behavior, rationale,
and consequence together. Prefer a callout Item attached to the responsible
visual element and give it exact evidence. Do not manufacture novelty. The
surprise audit fails when a reviewer cannot identify the system model, the
highest-consequence deviation, why it exists, and the tradeoff it creates.

Report components

A saga root includes a reviewer-facing README.md with safe installation,
opening, and structured-query guidance. The file is informational bootstrap
material rather than authored narrative or code evidence.

A saga begins with its overview and direct *.chapter directories. Chapters are
independently reviewable boundaries roughly corresponding to the PRs one might
have created when splitting the change. Sections recurse inside chapters and
contain directory-backed fragments. Each *.fragment has fragment.json and an
entrypoint. Markdown, HTML, SVG, text, and raster images are supported. HTML and SVG may bundle JavaScript
and assets; the reference viewer executes them in sandboxed frames.

Any saga, chapter, section, or fragment may own ___code/*.json evidence. Every
evidence entry is a code reference: a commit, a repository path, an optional
inclusive line range (absent for a whole file), and a sha256 digest of the
referenced content. The repository is the Saga's declared source repository.
A diff is never stored. Coverage is computed per comparison: every changed line
must lie inside a reference that is current where the line lives, added lines
at the head commit and deleted lines at the merge-base, and file events inside
a whole-file reference. A reference whose lines only moved is remapped; one
whose lines changed is stale until it is re-authored.

When a change lands, change-saga repin --onto <commit> re-pins evidence
references to the landed commit, following moved lines or the content digest
when a squash merge left the branch commit unreachable, and records the
branch's commit messages in ___merges/<commit>.json.

Addressable Markdown headings, exact text, HTML/SVG elements, and image regions
live in independent ___landmarks/<id>.landmark packages beneath a fragment.
Create them with change-saga add-landmark, then pass the printed path or URN to
change-saga cover so a reviewer can move directly from a visual node to code.
For SVG fragments, --element-id identifies the node and its rendered bounds
automatically become the on-canvas link. Use --hotspot only to override that
geometry. HTML element landmarks need an explicit hotspot to appear on-canvas;
without one they remain reachable through Marked places and deep links.
Meaningful visual landmarks include a semantic description so query clients can
understand their role without interpreting SVG or HTML geometry.
Markdown footnotes are the prose citation convention. When a footnote
definition is an exact-text landmark with evidence, the renderer turns both its
inline reference and footer entry into controls that open the linked code.
A prose citation and a code-bearing visual node have the same completion rule:
each needs a stable landmark with focused code references. A rendered footnote
without that association is incomplete. Requirements provenance citations are
separate and do not prove implementation.

Falsifiable author assertions live as independent ___claims/<id>.json records.
Append-only ___verifications/<id>.json records mark them unverified, verified,
failed, or inconclusive and preserve the method and reproducible command. Claim
evidence never contributes to coverage. Git history supplies attribution.

Review threads live under ___review/threads. They target stable
urn:change-saga:* identifiers and anchor to a whole fragment, normalized shapes,
freehand drawings, quoted text, a placed sticky note, or a code reference.
A sticky note carries its visible text, a normalized centre point, and an
optional color; moving, rewording, or recoloring it appends an anchor event. Thread messages contain
fragments, so replies may include Markdown, HTML, SVG, and images. Suggestion
threads include replacement code. Append-only whole-file reference events track reviewed
state, and approvals may target the saga, a chapter, a section, or a fragment.
Every comment owns a thread directory, every reply owns a message directory, and
each state transition is a new file; review operations never update shared arrays.
Approval events declare a human or AI reviewer persona in addition to their
Git-derived author. AI personas name an independent review seat, their agent
kind, and model. The latest event is projected per author and persona,
preserving concurrent decisions by
other reviewers; legacy events without persona metadata remain unspecified.
Every decision is an independent file, so parallel review branches add records
instead of rewriting a shared reviewer list.

All-atoms-mapped is an omission invariant, not a correctness or explanation-
quality verdict. Use query mappings --sort scrutiny to inspect broad or thin
coverage records, and query claims/verifications to independently test author
assertions.

The authoritative specification is SPEC.md in the Change Saga repository.
`
