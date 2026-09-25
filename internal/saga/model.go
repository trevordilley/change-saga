package saga

import (
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
)

const (
	// ComponentVersion is the version of the narrative, evidence, and review
	// component records a Change Saga composes.
	ComponentVersion = 2

	// SagaVersion is the one Change Saga container format and SagaSchemaURL
	// its manifest schema. Every Saga is this report container: requirements,
	// design, work plan, quality, and the embedded implementation deck.
	SagaVersion   = 5
	SagaSchemaURL = "https://changesaga.dev/schema/v5/saga.schema.json"

	// DeckRecordVersion versions the deck, slide, and Item records of the
	// implementation deck embedded under ___slides/. It is a record version,
	// not a container format.
	DeckRecordVersion = 4

	// CodeDirName holds a narrative target's code-reference evidence records.
	CodeDirName = "___code"

	// MergesDir holds one record per landed change: the commit it landed as
	// and the messages of the branch commits it collapsed.
	MergesDir = "___merges"

	// ReviewsDir holds one <id>.review directory per pull request review.
	ReviewsDir = "___reviews"

	// QualityRootDir is the quality capability root.
	QualityRootDir = "___quality"

	// CurrentVersion is the component record version new records are written
	// with.
	CurrentVersion = ComponentVersion
)

type Manifest struct {
	Schema  string `json:"$schema,omitempty"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Source  Source `json:"source"`
}

type DeckManifest struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Rank      int    `json:"rank"`
	Objective string `json:"objective"`
}

type SlideManifest struct {
	Version            int      `json:"version"`
	ID                 string   `json:"id"`
	DeckID             string   `json:"deck"`
	Title              string   `json:"title"`
	Rank               int      `json:"rank"`
	Section            string   `json:"section,omitempty"`
	Intent             string   `json:"intent"`
	Layout             string   `json:"layout"`
	MediaType          string   `json:"media_type"`
	Entrypoint         string   `json:"entrypoint"`
	Takeaway           string   `json:"takeaway"`
	ReadingOrder       []string `json:"reading_order"`
	ExceptionRationale string   `json:"exception_rationale,omitempty"`
}

type ItemManifest struct {
	Version     int              `json:"version"`
	ID          string           `json:"id"`
	SlideID     string           `json:"slide"`
	Rank        int              `json:"rank"`
	Kind        string           `json:"kind"`
	Label       string           `json:"label"`
	Description string           `json:"description,omitempty"`
	Selector    LandmarkSelector `json:"selector"`
	Hotspot     *LandmarkRegion  `json:"hotspot,omitempty"`
	About       string           `json:"about,omitempty"`
	Body        string           `json:"body,omitempty"`
	Placement   string           `json:"placement,omitempty"`
	Leader      string           `json:"leader,omitempty"`
	// Record is the persona, feature, or story URN an onboarding Item explains.
	// Only onboarding Items carry it; implementation Items reference code.
	Record string `json:"record,omitempty"`
}

type Deck struct {
	Path      string `json:"path"`
	Directory string `json:"-"`
	DeckManifest
	Target string   `json:"target"`
	Slides []*Slide `json:"slides"`
}

type Slide struct {
	Path      string `json:"path"`
	Directory string `json:"-"`
	SlideManifest
	Target             string    `json:"target"`
	Items              []*Item   `json:"items"`
	AuthoringSnapshot  string    `json:"authoring_snapshot,omitempty"`
	AuthoringHeads     []string  `json:"authoring_heads,omitempty"`
	AuthoringConflict  bool      `json:"authoring_conflict,omitempty"`
	AuthoringCreatedAt time.Time `json:"-"`
	// Diagram names the structured source the current transaction revision
	// rendered its SVG from. Hand-authored slides have none.
	Diagram *DiagramSource `json:"-"`
}

type Item struct {
	Path      string `json:"path"`
	Directory string `json:"-"`
	ItemManifest
	Target string     `json:"target"`
	Code   []CodeFile `json:"code,omitempty"`
	// CriterionLinks are present only for complete-slide transaction records.
	// They are exact Item-level links pinned to one story revision.
	CriterionLinks []CriterionLink `json:"criterion_links,omitempty"`
	HasCode        bool            `json:"-"`
}

// Source names the code repository a Saga documents. It holds no
// comparison: a Saga is opened to observe one commit or to compare two, and
// that choice is never stored.
type Source struct {
	Repository string `json:"repository"`
}

type SectionManifest struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Order   int    `json:"order,omitempty"`
}

type ChapterManifest struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Order   int    `json:"order,omitempty"`
}

type Section struct {
	Path      string      `json:"path"`
	Kind      string      `json:"kind"`
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Order     int         `json:"order,omitempty"`
	Target    string      `json:"target"`
	Children  []*Section  `json:"children,omitempty"`
	Fragments []*Fragment `json:"fragments,omitempty"`
	Code      []CodeFile  `json:"code,omitempty"`
	// HasCode records the presence of this target's ___code directory without
	// materializing any evidence records. Narrative-only loads use it to render
	// a lazy linked-code affordance while keeping coverage metadata unopened.
	HasCode bool `json:"-"`
}

type FragmentManifest struct {
	Version    int    `json:"version"`
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	MediaType  string `json:"media_type"`
	Entrypoint string `json:"entrypoint"`
	Order      int    `json:"order,omitempty"`
}

type Fragment struct {
	Path               string         `json:"path"`
	Directory          string         `json:"-"`
	ID                 string         `json:"id"`
	Title              string         `json:"title,omitempty"`
	MediaType          string         `json:"media_type"`
	Entrypoint         string         `json:"entrypoint"`
	Order              int            `json:"order,omitempty"`
	Target             string         `json:"target"`
	Code               []CodeFile     `json:"code,omitempty"`
	HasCode            bool           `json:"-"`
	Landmarks          []Landmark     `json:"landmarks,omitempty"`
	SlideMeta          *SlideManifest `json:"-"`
	DeckRole           string         `json:"-"`
	AuthoringSnapshot  string         `json:"-"`
	AuthoringHeads     []string       `json:"-"`
	AuthoringConflict  bool           `json:"-"`
	AuthoringCreatedAt time.Time      `json:"-"`
}

type Landmark struct {
	Path           string           `json:"-"`
	Directory      string           `json:"-"`
	Version        int              `json:"version"`
	ID             string           `json:"id"`
	Label          string           `json:"label"`
	Description    string           `json:"description,omitempty"`
	Selector       LandmarkSelector `json:"selector"`
	Hotspot        *LandmarkRegion  `json:"hotspot,omitempty"`
	Target         string           `json:"target"`
	Code           []CodeFile       `json:"code,omitempty"`
	CriterionLinks []CriterionLink  `json:"criterion_links,omitempty"`
	HasCode        bool             `json:"-"`
	ItemMeta       *ItemManifest    `json:"-"`
}

type LandmarkSelector struct {
	Type      string  `json:"type"`
	ElementID string  `json:"element_id,omitempty"`
	HeadingID string  `json:"heading_id,omitempty"`
	Exact     string  `json:"exact,omitempty"`
	Prefix    string  `json:"prefix,omitempty"`
	Suffix    string  `json:"suffix,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	Width     float64 `json:"width,omitempty"`
	Height    float64 `json:"height,omitempty"`
}

type LandmarkRegion struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// CodeFile is one evidence record: the code references a narrative target
// explains. It never contains a diff; comparisons are computed from it.
type CodeFile struct {
	Path       string              `json:"-"`
	Version    int                 `json:"version"`
	References []coderef.Reference `json:"references"`
}

// Claim is one falsifiable assertion made by the change author. Claims are
// deliberately independent records: adding a second claim never rewrites the
// first record and two authors do not contend on one aggregate manifest.
// Evidence here does not contribute to coverage; it references code at a
// commit that an independent reviewer can inspect when testing the assertion.
type Claim struct {
	Path      string              `json:"-"`
	Version   int                 `json:"version"`
	ID        string              `json:"id"`
	Target    string              `json:"target"`
	Kind      string              `json:"kind"`
	Statement string              `json:"statement"`
	Evidence  []coderef.Reference `json:"evidence"`
	CreatedAt time.Time           `json:"created_at"`
}

// Verification is an append-only result for one claim. The latest result is
// useful for navigation, but the complete history remains committed as
// separate files so a later result never erases the earlier one.
type Verification struct {
	Path      string    `json:"-"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Claim     string    `json:"claim"`
	Status    string    `json:"status"`
	Method    string    `json:"method,omitempty"`
	Summary   string    `json:"summary"`
	Command   string    `json:"command,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ReviewerIdentity describes the persona that made a review decision. Git
// remains the authority for who introduced the event; this metadata says
// whether that person acted directly or through a particular AI reviewer.
// It is optional only so review records created before this field existed stay
// readable without being incorrectly relabeled as human decisions.
type ReviewerIdentity struct {
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Agent string `json:"agent,omitempty"`
	Model string `json:"model,omitempty"`
}

// Merge records a change landing on its target branch. Commit is the landed
// commit that references were re-pinned to; Commits are the branch's commits,
// oldest first, so a squash merge keeps the per-commit reasoning Git's own
// history no longer carries.
type Merge struct {
	Path    string `json:"-"`
	Version int    `json:"version"`
	Commit  string `json:"commit"`
	// Base is the commit the change was compared from, so the comparison
	// "--against <base> --head <commit>" reproduces the landed change. It is
	// absent when the landed commit has no parent.
	Base    string         `json:"base,omitempty"`
	Commits []MergedCommit `json:"commits"`
	// Review is the pull request review of the landed change, frozen at the
	// same time, when the Saga has one.
	Review   string    `json:"review,omitempty"`
	PinnedAt time.Time `json:"pinned_at"`
}

type MergedCommit struct {
	Commit  string    `json:"commit"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Subject string    `json:"subject"`
	Body    string    `json:"body,omitempty"`
}

// MergeFilename is the record name for a landed commit.
func MergeFilename(commit string) string { return commit + ".json" }

type Saga struct {
	Root     string   `json:"root"`
	Manifest Manifest `json:"manifest"`
	// Section is the one app-wide tree every target index walks: the app's
	// overview and design system, every feature's report content and design,
	// and the projected decks, joined beneath the app root.
	Section *Section `json:"section"`
	// Decks are every feature's implementation decks.
	Decks []*Deck `json:"decks,omitempty"`
	// Overview and DesignSystem are the app-level report roots, or nil.
	Overview     *Section `json:"overview,omitempty"`
	DesignSystem *Section `json:"design_system,omitempty"`
	// Onboarding is the app's onboarding deck, whose Items reference records.
	Onboarding []*Deck `json:"onboarding,omitempty"`
	// Features groups the same nodes by durable product domain.
	Features []*Feature `json:"features,omitempty"`
	// Reviews are the pull requests' review decks. They are not
	// documentation and never count toward coverage.
	Reviews       []*Review      `json:"reviews,omitempty"`
	Claims        []Claim        `json:"claims,omitempty"`
	Verifications []Verification `json:"verifications,omitempty"`
	Merges        []Merge        `json:"merges,omitempty"`
}

type Issue struct {
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

type Validation struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

func SagaTarget(sagaID string) string {
	return "urn:change-saga:" + sagaID + ":saga"
}

func SectionTarget(sagaID, sectionID string) string {
	return "urn:change-saga:" + sagaID + ":section:" + sectionID
}

func ChapterTarget(sagaID, chapterID string) string {
	return "urn:change-saga:" + sagaID + ":chapter:" + chapterID
}

func FragmentTarget(sagaID, fragmentID string) string {
	return "urn:change-saga:" + sagaID + ":fragment:" + fragmentID
}

func LandmarkTarget(sagaID, fragmentID, landmarkID string) string {
	return FragmentTarget(sagaID, fragmentID) + ":landmark:" + landmarkID
}

func DeckTarget(sagaID, deckID string) string {
	return "urn:change-saga:" + sagaID + ":deck:" + deckID
}

func SlideTarget(sagaID, slideID string) string {
	return "urn:change-saga:" + sagaID + ":slide:" + slideID
}

func ItemTarget(sagaID, slideID, itemID string) string {
	return SlideTarget(sagaID, slideID) + ":item:" + itemID
}
