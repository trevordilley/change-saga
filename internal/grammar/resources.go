package grammar

// Resource describes one living record kind: where it lives, how it is named,
// which Saga versions carry it, and how it changes over time.
type Resource struct {
	Kind      string   `json:"kind"`
	URN       string   `json:"urn"`
	Storage   string   `json:"storage"`
	Schema    string   `json:"schema"`
	Versions  []int    `json:"versions"`
	History   string   `json:"history"`
	Lifecycle []string `json:"lifecycle,omitempty"`
	Writers   []string `json:"writers"`
	Notes     string   `json:"notes,omitempty"`
}

// Relation is one row of the persisted relation endpoint matrix.
type Relation struct {
	Type         string   `json:"type"`
	Sources      []string `json:"sources"`
	Targets      []string `json:"targets"`
	Scopes       []string `json:"scopes"`
	RequiredPins []string `json:"required_pins"`
	Axes         []string `json:"axes"`
	Meaning      string   `json:"meaning"`
}

// DerivedEdge is a graph edge the engine computes from validated records and
// never persists.
type DerivedEdge struct {
	Edge   string `json:"edge"`
	Source string `json:"source"`
	Target string `json:"target"`
	Rule   string `json:"rule"`
}

// AxisRule states which persisted records can cover one coverage axis.
type AxisRule struct {
	Axis    string `json:"axis"`
	Covered string `json:"covered_by"`
	Gate    string `json:"gate"`
}

const schemaBase = "https://changesaga.dev/schema/"

// Resources returns the living resource catalogue in stable order.
func Resources() []Resource { return append([]Resource(nil), resources...) }

// Relations returns the persisted relation endpoint matrix.
func Relations() []Relation { return append([]Relation(nil), relations...) }

// DerivedEdges returns the non-persisted edge kinds.
func DerivedEdges() []DerivedEdge { return append([]DerivedEdge(nil), derivedEdges...) }

// AxisRules returns what can satisfy each of the six coverage axes.
func AxisRules() []AxisRule { return append([]AxisRule(nil), axisRules...) }

var resources = []Resource{
	{
		Kind: "feature", URN: "urn:change-saga:<saga>:feature:<feature>", Storage: "___features/<feature>.feature/feature.json",
		Schema: schemaBase + "v5/feature.schema.json", Versions: []int{5}, History: "immutable identity of a durable product domain",
		Writers: []string{"feature add"},
		Notes:   "holds the domain's report content, requirements, design, quality, work plan, and implementation deck; no resource URN names its feature, so IDs are unique across the app",
	},
	{
		Kind: "overview-pitch", URN: "urn:change-saga:<saga>:fragment:<saga>-pitch", Storage: "___overview/pitch.fragment/{fragment.json,content.md}",
		Schema: schemaBase + "v2/fragment.schema.json", Versions: []int{5}, History: "current Markdown content; Git holds its history",
		Writers: []string{"overview set-pitch"},
		Notes:   "the overview's elevator pitch; the overview is the project name (saga.json title), elevator pitch, description, and terms and vocabulary, each optional and shown as a gap when absent",
	},
	{
		Kind: "overview-description", URN: "urn:change-saga:<saga>:fragment:<saga>-description", Storage: "___overview/description.fragment/{fragment.json,content.md}",
		Schema: schemaBase + "v2/fragment.schema.json", Versions: []int{5}, History: "current Markdown content; Git holds its history",
		Writers: []string{"overview set-description"},
		Notes:   "the overview's description: a short essay on what the application does",
	},
	{
		Kind: "term", URN: "urn:change-saga:<saga>:term:<term>", Storage: "___overview/terms/<term>.term/term.json",
		Schema: schemaBase + "v5/term.schema.json", Versions: []int{5}, History: "immutable identity",
		Writers: []string{"term add"},
		Notes:   "the project's own vocabulary; a story page and a line of code both reach the terms that name them",
	},
	{
		Kind: "term-revision", URN: "urn:change-saga:<saga>:term:<term>:revision:<revision>", Storage: "___overview/terms/<term>.term/revisions/<revision>.json",
		Schema: schemaBase + "v5/term-revision.schema.json", Versions: []int{5}, History: "append-only complete snapshots with parent heads; multiple heads are a conflict",
		Writers: []string{"term add", "term revise"},
		Notes:   "name, definition, aliases, the stories and records it names, and code references pinned at a commit; the code is watched for renames and never counts toward changed-line coverage",
	},
	{
		Kind: "term-event", URN: "urn:change-saga:<saga>:term:<term>:event:<event>", Storage: "___overview/terms/<term>.term/events/<event>.json",
		Schema: schemaBase + "v5/term-event.schema.json", Versions: []int{5}, History: "append-only lifecycle graph with parent heads",
		Lifecycle: []string{"active", "retired"}, Writers: []string{"term add", "term set-state"},
	},
	{
		Kind: "persona", URN: "urn:change-saga:<saga>:persona:<persona>", Storage: "___personas/<persona>.persona/persona.json",
		Schema: schemaBase + "v5/persona.schema.json", Versions: []int{5}, History: "immutable identity",
		Writers: []string{"persona add"},
	},
	{
		Kind: "persona-revision", URN: "urn:change-saga:<saga>:persona:<persona>:revision:<revision>", Storage: "___personas/<persona>.persona/revisions/<revision>.json",
		Schema: schemaBase + "v5/persona-revision.schema.json", Versions: []int{5}, History: "append-only complete snapshots with parent heads; multiple heads are a conflict",
		Writers: []string{"persona add", "persona revise"},
	},
	{
		Kind: "persona-event", URN: "urn:change-saga:<saga>:persona:<persona>:event:<event>", Storage: "___personas/<persona>.persona/events/<event>.json",
		Schema: schemaBase + "v5/persona-event.schema.json", Versions: []int{5}, History: "append-only lifecycle graph with parent heads",
		Lifecycle: []string{"active", "retired"}, Writers: []string{"persona add", "persona set-state"},
		Notes: "every active persona needs an accepted story that serves it",
	},
	{
		Kind: "flag", URN: "urn:change-saga:<saga>:flag:<flag>", Storage: "___featureflags/<flag>.flag/flag.json",
		Schema: schemaBase + "v5/flag.schema.json", Versions: []int{5}, History: "immutable identity",
		Writers: []string{"flag add"},
	},
	{
		Kind: "flag-revision", URN: "urn:change-saga:<saga>:flag:<flag>:revision:<revision>", Storage: "___featureflags/<flag>.flag/revisions/<revision>.json",
		Schema: schemaBase + "v5/flag-revision.schema.json", Versions: []int{5}, History: "append-only complete snapshots with parent heads; multiple heads are a conflict",
		Writers: []string{"flag add", "flag revise"},
		Notes:   "targets are story or feature URNs; a flag on a feature gates every story in it",
	},
	{
		Kind: "flag-event", URN: "urn:change-saga:<saga>:flag:<flag>:event:<event>", Storage: "___featureflags/<flag>.flag/events/<event>.json",
		Schema: schemaBase + "v5/flag-event.schema.json", Versions: []int{5}, History: "append-only lifecycle graph with parent heads",
		Lifecycle: []string{"off", "on", "retired"}, Writers: []string{"flag add", "flag set-state"},
		Notes: "a story gated by a current off flag is implemented but not enabled",
	},
	{
		Kind: "story", URN: "urn:change-saga:<saga>:story:<story>", Storage: "___features/<feature>.feature/___requirements/stories/<story>.story/story.json",
		Schema: schemaBase + "v3/story.schema.json", Versions: []int{5}, History: "immutable identity",
		Writers: []string{"story add", "story move"},
		Notes:   "the URN never names the feature; story move changes only which feature's directory holds the package",
	},
	{
		Kind: "story-revision", URN: "urn:change-saga:<saga>:story:<story>:revision:<revision>", Storage: "___features/<feature>.feature/___requirements/stories/<story>.story/revisions/<revision>.json",
		Schema: schemaBase + "v3/story-revision.schema.json", Versions: []int{5}, History: "append-only complete snapshots with parent heads; multiple heads are a conflict",
		Writers: []string{"story add", "story revise", "criterion add", "criterion revise", "criterion remove"},
	},
	{
		Kind: "story-event", URN: "urn:change-saga:<saga>:story:<story>:event:<event>", Storage: "___features/<feature>.feature/___requirements/stories/<story>.story/events/<event>.json",
		Schema: schemaBase + "v3/story-event.schema.json", Versions: []int{5}, History: "append-only lifecycle graph with parent heads",
		Lifecycle: []string{"proposed", "accepted", "deferred", "rejected", "retired"}, Writers: []string{"story add", "story set-state"},
	},
	{
		Kind: "criterion", URN: "urn:change-saga:<saga>:story:<story>:criterion:<criterion>", Storage: "inside every story revision's acceptance_criteria",
		Schema: schemaBase + "v3/story-revision.schema.json", Versions: []int{5}, History: "stable id across revisions; a removed id is never reused",
		Writers: []string{"criterion add", "criterion revise", "criterion remove"},
		Notes:   "inherits the story lifecycle; active only while present in the unique current story revision",
	},
	{
		Kind: "citation", URN: "urn:change-saga:<saga>:citation:<citation>", Storage: "___features/<feature>.feature/___requirements/citations/<citation>.json",
		Schema: schemaBase + "v3/citation.schema.json", Versions: []int{5}, History: "immutable", Writers: []string{"citation add"},
	},
	{
		Kind: "relation", URN: "urn:change-saga:<saga>:relation:<relation>", Storage: "___features/<feature>.feature/___requirements/relations/<relation>.json",
		Schema: schemaBase + "v5/relation.schema.json", Versions: []int{5}, History: "immutable pins; active or superseded",
		Writers: []string{"relation add", "relation supersede"},
		Notes:   "v3 relations remain valid history; v5 relations add scope and visual digest pins",
	},
	{
		Kind: "relation-repin", URN: "urn:change-saga:<saga>:relation:<relation>:repin:<repin>", Storage: "___features/<feature>.feature/___requirements/relation-repins/<relation>/<repin>.json",
		Schema: schemaBase + "v5/relation-repin.schema.json", Versions: []int{5}, History: "append-only; oldest first by recorded time then id, and the fold is the relation's confirmed pins",
		Writers: []string{"relation repin"},
		Notes:   "one immutable confirmation that a relation still holds against the pins it names; the relation record is never rewritten to advance a pin",
	},
	{
		Kind: "prototype", URN: "urn:change-saga:<saga>:prototype:<prototype>", Storage: "___features/<feature>.feature/___requirements/prototypes/<prototype>.prototype/prototype.json",
		Schema: schemaBase + "v3/prototype.schema.json", Versions: []int{5}, History: "immutable identity", Writers: []string{"prototype add-html"},
	},
	{
		Kind: "prototype-revision", URN: "urn:change-saga:<saga>:prototype:<prototype>:revision:<revision>", Storage: "___features/<feature>.feature/___requirements/prototypes/<prototype>.prototype/revisions/<revision>.revision/",
		Schema: schemaBase + "v3/prototype-revision.schema.json", Versions: []int{5}, History: "append-only immutable experiences with parent heads",
		Lifecycle: []string{"draft", "ready", "retired"}, Writers: []string{"prototype add-html"},
	},
	{
		Kind: "prototype-annotation", URN: "urn:change-saga:<saga>:prototype:<prototype>:annotation:<annotation>", Storage: "___features/<feature>.feature/___requirements/prototypes/annotations/",
		Schema: schemaBase + "v3/prototype-annotation.schema.json", Versions: []int{5}, History: "immutable; pinned to a prototype revision or digest and a story revision",
		Writers: []string{"prototype annotate"},
	},
	{
		Kind: "coverage-exception", URN: "urn:change-saga:<saga>:coverage-exception:<exception>", Storage: "___features/<feature>.feature/___requirements/coverage-exceptions/<exception>.json",
		Schema: schemaBase + "v5/coverage-exception.schema.json", Versions: []int{5}, History: "immutable decisions; supersession graph with one head per criterion/axis",
		Writers: []string{"coverage-exception add", "coverage-exception supersede"},
		Notes:   "pins the current story revision; never excuses changed-source accounting",
	},
	{
		Kind: "review", URN: "urn:change-saga:<saga>:review:<review>", Storage: "___reviews/<review>.review/review.json and deck/ (a flat deck bundle, role review)",
		Schema: schemaBase + "v5/review.schema.json", Versions: []int{5}, History: "one per pull request; its head follows a ref until repin freezes the merged base and head",
		Writers: []string{"review create", "repin"},
		Notes:   "a review is the pull request's slide deck; its slide and Item URNs are <review>:slide:<slide>[:item:<item>], its Items reference code (viewed as a diff against the base) and may reference records; it never counts toward coverage",
	},
	{
		Kind: "review-approval", URN: "urn:change-saga:<saga>:review:<review>:slide:<slide>", Storage: "___reviews/<review>.review/approvals/<event>.json",
		Schema: schemaBase + "v5/review-approval.schema.json", Versions: []int{5}, History: "append-only; the latest decision per reviewer and slide is current",
		Lifecycle: []string{"approved", "changes_requested", "none"}, Writers: []string{"review approve", "review request-changes", "review withdraw"},
		Notes: "records the head commit and the slide digest it was given at; out of date when the slide or the code it references changed since",
	},
	{
		Kind: "review-comment", URN: "urn:change-saga:<saga>:review:<review>:slide:<slide>[:item:<item>]", Storage: "___reviews/<review>.review/comments/<comment>.json",
		Schema: schemaBase + "v5/review-comment.schema.json", Versions: []int{5}, History: "append-only; a reply names its parent and may resolve or reopen the thread",
		Lifecycle: []string{"open", "resolved"}, Writers: []string{"review comment"},
	},
	{
		Kind: "test-case", URN: "urn:change-saga:<saga>:test-case:<test-case>", Storage: "___features/<feature>.feature/___quality/test-cases/<test-case>.test/test-case.json",
		Schema: schemaBase + "v5/test-case.schema.json", Versions: []int{5}, History: "immutable identity", Writers: []string{"quality test-case add"},
	},
	{
		Kind: "test-case-revision", URN: "urn:change-saga:<saga>:test-case:<test-case>:revision:<revision>", Storage: "___features/<feature>.feature/___quality/test-cases/<test-case>.test/revisions/<revision>.json",
		Schema: schemaBase + "v5/test-case-revision.schema.json", Versions: []int{5}, History: "append-only complete definitions with ordered steps; removed step ids are never reused",
		Writers: []string{"quality test-case add", "quality test-case revise"},
	},
	{
		Kind: "test-case-event", URN: "urn:change-saga:<saga>:test-case:<test-case>:event:<event>", Storage: "___features/<feature>.feature/___quality/test-cases/<test-case>.test/events/<event>.json",
		Schema: schemaBase + "v5/test-case-event.schema.json", Versions: []int{5}, History: "append-only lifecycle graph",
		Lifecycle: []string{"proposed", "active", "deprecated", "retired"}, Writers: []string{"quality test-case add", "quality test-case set-state"},
	},
	{
		Kind: "quality-policy", URN: "urn:change-saga:<saga>:quality-policy:<policy>", Storage: "___features/<feature>.feature/___quality/policies/<policy>.json",
		Schema: schemaBase + "v5/quality-policy.schema.json", Versions: []int{5}, History: "immutable; one unsuperseded head per criterion/story revision",
		Writers: []string{"quality policy set"}, Notes: "absent policy means positive is required",
	},
	{
		Kind: "quality-evidence", URN: "urn:change-saga:<saga>:test-case:<test-case>:evidence:<evidence>", Storage: "___features/<feature>.feature/___quality/test-cases/<test-case>.test/evidence/<evidence>.json",
		Schema: schemaBase + "v5/quality-evidence.schema.json", Versions: []int{5}, History: "immutable; supersession graph",
		Writers: []string{"quality evidence add"},
		Notes:   "roles: test_implementation, implementation_under_test, execution_artifact",
	},
	{
		Kind: "test-run", URN: "urn:change-saga:<saga>:test-case:<test-case>:run:<run>", Storage: "___features/<feature>.feature/___quality/test-cases/<test-case>.test/runs/<run>.json",
		Schema: schemaBase + "v5/test-run.schema.json", Versions: []int{5}, History: "append-only results with parent heads; multiple heads are a conflict",
		Lifecycle: []string{"passed", "failed", "blocked", "skipped"}, Writers: []string{"quality run record"},
		Notes: "current only for the current test revision, the current source comparison, and current evidence",
	},
}

const (
	reportDesign = "report design target (chapter, section, fragment, or landmark under ___design)"
	deckSlide    = "deck or slide"
	item         = "item"
	story        = "story"
	criterion    = "criterion"
)

var relations = []Relation{
	{
		Type: "refines", Sources: []string{story, criterion}, Targets: []string{story, criterion}, Scopes: []string{"self"},
		RequiredPins: []string{"from_revision", "to_revision"}, Axes: []string{},
		Meaning: "navigational decomposition; not evidence; active graph is acyclic",
	},
	{
		Type: "addresses", Sources: []string{reportDesign, "deck", "slide", item}, Targets: []string{story, criterion}, Scopes: []string{"self", "descendants"},
		RequiredPins: []string{"from_content_digest", "to_revision"}, Axes: []string{"ux", "technical", "implementation"},
		Meaning: "counts design coverage; descendants is legal only for a deck or slide source and lets the path reach contained Items and their code references",
	},
	{
		Type: "implements", Sources: []string{"work-item"}, Targets: []string{reportDesign, criterion}, Scopes: []string{"self"},
		RequiredPins: []string{"from_revision", "to_revision or to_content_digest"}, Axes: []string{},
		Meaning: "planning path only; progress never proves delivery",
	},
	{
		Type: "explains", Sources: []string{deckSlide, item}, Targets: []string{story, criterion}, Scopes: []string{"self", "descendants"},
		RequiredPins: []string{"to_revision"}, Axes: []string{"implementation"},
		Meaning: "reviewer explanation (legacy_review_explanation); reaches code through Items but never counts as design coverage",
	},
	{
		Type: "verifies", Sources: []string{"claim", "verification", "test-case"}, Targets: []string{criterion}, Scopes: []string{"self"},
		RequiredPins: []string{"to_revision", "from_revision when the source is a test case"}, Axes: []string{"quality"},
		Meaning: "test intent; a test case needs a current passing run before quality readiness",
	},
	{
		Type: "supersedes", Sources: []string{"any resource"}, Targets: []string{"a resource of the same kind"}, Scopes: []string{"self"},
		RequiredPins: []string{}, Axes: []string{},
		Meaning: "explicit replacement; no automatic lifecycle mutation; acyclic",
	},
	{
		Type: "conflicts_with", Sources: []string{story, criterion}, Targets: []string{story, criterion}, Scopes: []string{"self"},
		RequiredPins: []string{"from_revision", "to_revision"}, Axes: []string{},
		Meaning: "symmetric diagnostic in canonical lexical order; no inferred winner",
	},
}

var derivedEdges = []DerivedEdge{
	{Edge: "contains", Source: "deck -> slide -> item", Target: "item", Rule: "exact parent ids in validated v4 records"},
	{Edge: "owns_code", Source: "item", Target: "code reference", Rule: "valid 40-e record in the Item's embedded bundle whose reference is current in the comparison"},
	{Edge: "has_quality_evidence", Source: "test-case revision or run", Target: "quality-evidence", Rule: "exact URN reference in validated v5 records"},
	{Edge: "matches_item_code", Source: "implementation_under_test evidence", Target: "item", Rule: "both reference the same changed atoms once resolved in the current comparison"},
}

var axisRules = []AxisRule{
	{Axis: "prototype", Covered: "a current prototype annotation pinned to the criterion (direct) or its story (broad)", Gate: "product_ready"},
	{Axis: "ux", Covered: "a current addresses relation from a deck, slide, or Item of a ux-role deck", Gate: "design_ready"},
	{Axis: "ui", Covered: "no UI reference resource is recorded yet; only an explicit ui coverage exception resolves this axis", Gate: "design_ready"},
	{Axis: "technical", Covered: "a current addresses relation from a report design target or a non-ux deck, slide, or Item", Gate: "design_ready"},
	{Axis: "quality", Covered: "active test cases linked by current verifies relations whose current passing runs cover every required kind", Gate: "quality_ready"},
	{Axis: "implementation", Covered: "a current addresses or explains path through an Item to a current code reference", Gate: "implementation_trace_ready"},
}
