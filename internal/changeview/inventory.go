// Package changeview opens a Saga as a comparison of two commits: the Saga
// records the change added, revised, or retired (Changed), the records it did
// not edit but invalidated (Affected), and the changed code grouped under the
// records that reference it (Code). Nothing here is stored; every layer is
// computed from the Saga at the merge-base, the Saga at head, and the code
// between them.
package changeview

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Node kinds. Design, overview, and designsystem nodes are report targets
// (sections, fragments, and landmarks) under those roots.
const (
	KindApp       = "app"
	KindPersona   = "persona"
	KindFeature   = "feature"
	KindFlag      = "flag"
	KindStory     = "story"
	KindCitation  = "citation"
	KindRelation  = "relation"
	KindPrototype = "prototype"
	KindTestCase  = "test_case"
	KindDesign    = "design"
	KindReport    = "report"
	KindDeck      = "deck"
	KindSlide     = "slide"
	KindItem      = "item"
	KindTerm      = "term"
)

// maxText bounds the authored text a node carries for before and after.
const maxText = 16 << 10

// landmarksDir holds a fragment's landmark packages, each a record of its own.
const landmarksDir = "___landmarks"

// approvalsDir holds review decisions about a target. They are review
// records, not the target's content, so they never make it a new revision.
const approvalsDir = "___approvals"

// Node is one Saga record as a comparison sees it. Revision is a digest of
// every file the record owns, so any edit to the record, including its code
// references, is a new revision. Links are the adjacent chain edges the
// record declares or implies: what it explains, addresses, verifies, or
// belongs to, and for a story the personas it serves.
type Node struct {
	URN      string   `json:"urn"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Feature  string   `json:"feature,omitempty"`
	State    string   `json:"state,omitempty"`
	Revision string   `json:"revision"`
	Files    []string `json:"files"`
	Links    []string `json:"links,omitempty"`
	// Parent is the containing record: a slide for an Item, a deck for a
	// slide, a fragment for a landmark.
	Parent string `json:"parent,omitempty"`
	// Text is the record's authored content, bounded, for before and after.
	Text string `json:"-"`
	// Code is every code reference the record owns.
	Code []coderef.Reference `json:"-"`
	// Explains lists, for a relation, its from and to endpoints.
	Explains [2]string `json:"-"`
	// relationType is the relation's type for a relation node.
	relationType string
	// Targets are the stories and criteria the record addresses, explains,
	// verifies, or implements through its active relations, kept at criterion
	// granularity so two records that explain the same criterion can be paired.
	Targets []string `json:"-"`
	// contained marks a record whose parent holds it as part of one
	// explanation (an Item in its slide, a slide in its deck, a landmark in its
	// fragment), so what affects it affects the parent. A fragment's section
	// only groups it.
	contained bool
}

// Retired reports whether the record's lifecycle has ended.
func (node *Node) Retired() bool {
	switch node.State {
	case "retired", "removed", "deprecated", "superseded", "rejected", "obsolete":
		return true
	}
	return false
}

// Inventory is every record of one Saga snapshot, keyed by URN. Criteria
// resolve to their story.
type Inventory struct {
	Nodes   map[string]*Node
	aliases map[string]string
	// Supersedes maps a record to the records an active supersedes relation
	// says it replaces: the explicit replacement link.
	Supersedes map[string][]string
}

// Resolve returns the node a URN names, following criterion and revision
// URNs to their story.
func (inventory *Inventory) Resolve(urn string) *Node {
	if inventory == nil {
		return nil
	}
	if node := inventory.Nodes[urn]; node != nil {
		return node
	}
	if alias, ok := inventory.aliases[urn]; ok {
		return inventory.Nodes[alias]
	}
	if index := strings.Index(urn, ":criterion:"); index > 0 {
		return inventory.Nodes[urn[:index]]
	}
	if index := strings.Index(urn, ":revision:"); index > 0 {
		return inventory.Nodes[urn[:index]]
	}
	return nil
}

// Sorted returns the nodes in URN order.
func (inventory *Inventory) Sorted() []*Node {
	if inventory == nil {
		return nil
	}
	nodes := make([]*Node, 0, len(inventory.Nodes))
	for _, node := range inventory.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].URN < nodes[j].URN })
	return nodes
}

// Build indexes one loaded Saga snapshot. root is the Saga directory the
// records were loaded from; files are read to digest each record.
func Build(root string, document *saga.Saga, inputs livingapp.StatusInputs) *Inventory {
	if document.Root != "" {
		root = document.Root
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	builder := &inventoryBuilder{root: root, sagaID: document.Manifest.ID, inventory: &Inventory{Nodes: map[string]*Node{}, aliases: map[string]string{}}}
	if document.Section != nil {
		builder.add(&Node{URN: document.Section.Target, Kind: KindApp, Title: document.Manifest.Title, Files: []string{saga.ManifestName}, Text: document.Manifest.Title})
	}
	builder.app(inputs)
	builder.report(document.Section, "")
	builder.decks(document.Decks)
	builder.inventory.Supersedes = map[string][]string{}
	for _, relation := range builder.inventory.Nodes {
		if relation.Kind != KindRelation || relation.Retired() {
			continue
		}
		switch relation.relationType {
		case "addresses", "explains", "verifies", "implements":
			if from := builder.inventory.Nodes[relation.Explains[0]]; from != nil {
				from.Targets = append(from.Targets, relation.Explains[1])
			}
		case "supersedes":
			builder.inventory.Supersedes[relation.Explains[0]] = append(builder.inventory.Supersedes[relation.Explains[0]], relation.Explains[1])
		}
	}
	for _, node := range builder.inventory.Nodes {
		node.Targets = uniqueSorted(node.Targets)
		sort.Strings(node.Files)
		node.Links = uniqueSorted(node.Links)
		node.Revision = builder.digest(node.Files)
	}
	return builder.inventory
}

type inventoryBuilder struct {
	root, sagaID string
	inventory    *Inventory
}

func (b *inventoryBuilder) add(node *Node) *Node {
	if node.URN == "" {
		return nil
	}
	if existing := b.inventory.Nodes[node.URN]; existing != nil {
		existing.Files = append(existing.Files, node.Files...)
		return existing
	}
	b.inventory.Nodes[node.URN] = node
	return node
}

func (b *inventoryBuilder) app(inputs livingapp.StatusInputs) {
	for _, feature := range inputs.Features {
		b.add(&Node{URN: applayout.FeatureURN(b.sagaID, feature.ID), Kind: KindFeature, Title: feature.Title, Feature: feature.ID,
			Files: []string{applayout.FeatureRel(feature.ID) + "/" + applayout.FeatureManifestName}, Text: feature.Title + "\n\n" + feature.Description})
	}
	for _, persona := range inputs.Personas {
		urn, _ := requirements.PersonaURN(b.sagaID, persona.Identity.ID)
		node := &Node{URN: urn, Kind: KindPersona, Title: persona.Identity.ID, Files: b.tree(applayout.PersonasDir + "/" + persona.Identity.ID + ".persona")}
		if revision := persona.CurrentRevision; revision != nil {
			node.Title, node.Text = revision.Name, revision.Name+"\n\n"+revision.Description
		}
		if persona.CurrentLifecycle != nil {
			node.State = string(persona.CurrentLifecycle.State)
		}
		b.add(node)
	}
	for _, term := range inputs.Terms {
		urn, _ := requirements.TermURN(b.sagaID, term.Identity.ID)
		node := &Node{URN: urn, Kind: KindTerm, Title: term.Identity.ID, Files: b.tree(requirements.TermPackagePath(term.Identity.ID))}
		if revision := term.CurrentRevision; revision != nil {
			node.Title = revision.Name
			node.Text = revision.Name + "\n\n" + revision.Definition
			if len(revision.Aliases) > 0 {
				node.Text += "\n\nalso: " + strings.Join(revision.Aliases, ", ")
			}
			node.Links = append(append(node.Links, revision.Stories...), revision.Records...)
			node.Code = append(node.Code, revision.Code...)
		}
		if term.CurrentLifecycle != nil {
			node.State = string(term.CurrentLifecycle.State)
		}
		b.add(node)
	}
	for _, flag := range inputs.Flags {
		urn, _ := requirements.FlagURN(b.sagaID, flag.Identity.ID)
		node := &Node{URN: urn, Kind: KindFlag, Title: flag.Identity.ID, Files: b.tree(applayout.FeatureFlagsDir + "/" + flag.Identity.ID + ".flag")}
		if revision := flag.CurrentRevision; revision != nil {
			node.Text = revision.Description + "\n\ngates: " + strings.Join(revision.Targets, ", ")
		}
		if flag.CurrentLifecycle != nil {
			node.State = string(flag.CurrentLifecycle.State)
		}
		b.add(node)
	}
	for _, story := range inputs.Stories {
		urn, _ := livingid.Story(b.sagaID, story.Identity.ID)
		node := &Node{URN: urn, Kind: KindStory, Title: story.Identity.ID, Feature: story.Feature,
			Files: b.tree(applayout.FeatureRel(story.Feature) + "/" + applayout.RequirementsDir + "/stories/" + story.Identity.ID + ".story")}
		if revision := story.CurrentRevision; revision != nil {
			node.Title = revision.Title
			var text strings.Builder
			text.WriteString(revision.Title + "\n\n" + revision.Statement + "\n")
			for _, criterion := range revision.AcceptanceCriteria {
				text.WriteString("\n- " + criterion.ID + ": " + criterion.Statement)
				criterionURN, _ := livingid.Criterion(b.sagaID, story.Identity.ID, criterion.ID)
				b.inventory.aliases[criterionURN] = urn
			}
			node.Text = text.String()
			node.Links = append(node.Links, revision.Personas...)
		}
		if story.CurrentLifecycle != nil {
			node.State = string(story.CurrentLifecycle.State)
		}
		b.add(node)
	}
	for _, citation := range inputs.Citations {
		urn, _ := livingid.Citation(b.sagaID, citation.ID)
		b.add(&Node{URN: urn, Kind: KindCitation, Title: citation.Title, Feature: citation.Feature,
			Files: []string{applayout.FeatureRel(citation.Feature) + "/" + applayout.RequirementsDir + "/citations/" + citation.ID + ".json"}, Text: citation.Title})
	}
	relations := map[string]livingapp.Link{}
	for _, link := range inputs.Links {
		relations[link.URN] = link
	}
	for _, feature := range inputs.Features {
		dir := applayout.FeatureRel(feature.ID) + "/" + applayout.RequirementsDir + "/relations"
		entries, _ := os.ReadDir(filepath.Join(b.root, filepath.FromSlash(dir)))
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), ".json")
			urn, _ := livingid.Relation(b.sagaID, id)
			link, ok := relations[urn]
			if !ok || entry.IsDir() {
				continue
			}
			state := "active"
			if !link.Active {
				state = "superseded"
			}
			b.add(&Node{URN: urn, Kind: KindRelation, Title: string(link.Type) + " " + link.From + " -> " + link.To, Feature: feature.ID, State: state,
				Files: []string{dir + "/" + entry.Name()}, Explains: [2]string{link.From, link.To}, relationType: string(link.Type),
				Text: string(link.Type) + "\nfrom: " + link.From + "\nto: " + link.To})
		}
	}
	for _, prototype := range inputs.Prototypes.Prototypes {
		urn, _ := prototypes.PrototypeURN(b.sagaID, prototype.Identity.ID)
		node := &Node{URN: urn, Kind: KindPrototype, Title: prototype.Identity.ID, Feature: prototype.Feature,
			Files: b.tree(applayout.FeatureRel(prototype.Feature) + "/" + applayout.RequirementsDir + "/prototypes/" + prototype.Identity.ID + ".prototype")}
		if revision := prototype.CurrentRevision; revision != nil {
			node.Title, node.State, node.Text = revision.Title, string(revision.State), revision.Title
		}
		b.add(node)
	}
	for _, testCase := range inputs.Quality.TestCases {
		urn, _ := qualityid.TestCase(b.sagaID, testCase.Identity.ID)
		node := &Node{URN: urn, Kind: KindTestCase, Title: testCase.Identity.ID, Feature: testCase.Feature,
			Files: b.tree(applayout.FeatureRel(testCase.Feature) + "/" + applayout.QualityDir + "/test-cases/" + testCase.Identity.ID + ".test")}
		if revision := testCase.CurrentRevision; revision != nil {
			node.Title = revision.Title
			var text strings.Builder
			text.WriteString(revision.Title + "\n")
			for index, step := range revision.Steps {
				text.WriteString("\n" + strconv.Itoa(index+1) + ". " + step.Action)
			}
			text.WriteString("\n\nexpected: " + revision.ExpectedResult)
			node.Text = text.String()
		}
		if testCase.CurrentLifecycle != nil {
			node.State = string(testCase.CurrentLifecycle.State)
		}
		for _, evidence := range testCase.Evidence {
			node.Code = append(node.Code, evidence.Code...)
		}
		b.add(node)
	}
}

// report indexes the report tree: the app overview and design system, and
// every feature's report content and design. Projected decks are indexed from
// the typed deck tree instead.
func (b *inventoryBuilder) report(section *saga.Section, parent string) {
	if section == nil || section.Kind == "deck" {
		return
	}
	if section.Kind != "saga" && section.Path != "" && section.Path != "." {
		node := b.add(&Node{URN: section.Target, Kind: b.reportKind(section.Path), Title: section.Title, Feature: featureOf(section.Path),
			Files: b.shallow(section.Path), Code: codeOf(section.Code), Parent: parent})
		if node != nil {
			node.Text = section.Title
			parent = node.URN
		}
	}
	for _, fragment := range section.Fragments {
		if fragment.SlideMeta != nil {
			continue
		}
		node := b.add(&Node{URN: fragment.Target, Kind: b.reportKind(fragment.Path), Title: firstNonEmpty(fragment.Title, fragment.ID), Feature: featureOf(fragment.Path),
			Files: b.treeExcept(fragment.Path, landmarksDir), Code: codeOf(fragment.Code), Parent: parent})
		if node == nil {
			continue
		}
		node.Text = b.text(fragment.Path+"/"+fragment.Entrypoint, fragment.MediaType)
		for _, landmark := range fragment.Landmarks {
			if landmark.ItemMeta != nil {
				continue
			}
			landmarkPath := fragment.Path + "/" + landmarksDir + "/" + landmark.ID + ".landmark"
			b.add(&Node{URN: landmark.Target, Kind: node.Kind, Title: firstNonEmpty(landmark.Label, landmark.ID), Feature: node.Feature,
				Files: b.tree(landmarkPath), Code: codeOf(landmark.Code), Parent: node.URN, contained: true, Text: landmark.Label + "\n\n" + landmark.Description})
		}
	}
	for _, child := range section.Children {
		b.report(child, parent)
	}
}

func (b *inventoryBuilder) decks(decks []*saga.Deck) {
	for _, deck := range decks {
		deckNode := b.add(&Node{URN: deck.Target, Kind: KindDeck, Title: deck.Title, Feature: featureOf(deck.Path),
			Files: b.stem(deck.Path), Text: deck.Title + "\n\n" + deck.Objective})
		for _, slide := range deck.Slides {
			slideNode := b.add(&Node{URN: slide.Target, Kind: KindSlide, Title: slide.Title, Feature: deckNode.Feature, Parent: deck.Target, contained: true,
				Files: b.stem(slide.Path), Text: slide.Title + "\n\nintent: " + slide.Intent + "\ntakeaway: " + slide.Takeaway})
			if slideNode != nil && slide.Entrypoint != "" {
				if text := b.text(path.Join(path.Dir(b.rel(slide.Path)), slide.Entrypoint), slide.MediaType); text != "" {
					slideNode.Text += "\n\n" + text
				}
			}
			for _, item := range slide.Items {
				node := b.add(&Node{URN: item.Target, Kind: KindItem, Title: firstNonEmpty(item.Label, item.ID), Feature: deckNode.Feature, Parent: slide.Target, contained: true,
					Files: b.stem(item.Path), Code: codeOf(item.Code), Text: item.Label + "\n\n" + item.Description})
				if node == nil {
					continue
				}
				for _, file := range item.Code {
					node.Files = append(node.Files, b.rel(file.Path))
				}
				if item.Record != "" {
					node.Links = append(node.Links, item.Record)
				}
			}
		}
	}
}

func (b *inventoryBuilder) reportKind(rel string) string {
	if strings.Contains("/"+b.rel(rel)+"/", "/"+applayout.DesignDir+"/") {
		return KindDesign
	}
	return KindReport
}

// rel turns a loaded path into a slash path relative to the Saga root.
func (b *inventoryBuilder) rel(value string) string {
	if filepath.IsAbs(value) {
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = resolved
		}
		if relative, err := filepath.Rel(b.root, value); err == nil {
			value = relative
		}
	}
	return filepath.ToSlash(value)
}

// tree lists every regular file beneath dir.
func (b *inventoryBuilder) tree(dir string) []string {
	return b.treeExcept(dir, "")
}

// treeExcept lists every regular file beneath dir, skipping the named child
// directory (a fragment's landmarks are records of their own).
func (b *inventoryBuilder) treeExcept(dir, skip string) []string {
	dir = b.rel(dir)
	files := []string{}
	_ = filepath.WalkDir(filepath.Join(b.root, filepath.FromSlash(dir)), func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && (skip != "" && entry.Name() == skip || entry.Name() == approvalsDir) {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			files = append(files, b.rel(current))
		}
		return nil
	})
	return files
}

// shallow lists a section's own files: its manifest and its code
// references, not the fragments and sections it contains.
func (b *inventoryBuilder) shallow(dir string) []string {
	dir = b.rel(dir)
	files := []string{}
	entries, _ := os.ReadDir(filepath.Join(b.root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		switch {
		case entry.Type().IsRegular():
			files = append(files, dir+"/"+entry.Name())
		case entry.IsDir() && entry.Name() == saga.CodeDirName:
			files = append(files, b.tree(dir+"/"+entry.Name())...)
		}
	}
	return files
}

// stem lists a flat deck record's files: the record and every sibling that
// shares its name, such as a slide's visual entrypoint.
func (b *inventoryBuilder) stem(file string) []string {
	file = b.rel(file)
	dir, name := path.Dir(file), path.Base(file)
	stem := strings.TrimSuffix(name, path.Ext(name))
	files := []string{}
	entries, _ := os.ReadDir(filepath.Join(b.root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.TrimSuffix(entry.Name(), path.Ext(entry.Name())) == stem {
			files = append(files, dir+"/"+entry.Name())
		}
	}
	return files
}

func (b *inventoryBuilder) text(file, mediaType string) string {
	if !strings.HasPrefix(mediaType, "text/") && mediaType != "image/svg+xml" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(b.rel(file))))
	if err != nil || !utf8.Valid(data) {
		return ""
	}
	if len(data) > maxText {
		return string(data[:maxText]) + "\n…"
	}
	return string(data)
}

func (b *inventoryBuilder) digest(files []string) string {
	hash := sha256.New()
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		hash.Write([]byte(file + "\x00" + hex.EncodeToString(sum[:]) + "\n"))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func codeOf(files []saga.CodeFile) []coderef.Reference {
	var references []coderef.Reference
	for _, file := range files {
		references = append(references, file.References...)
	}
	return references
}

// featureOf names the feature a slash path lies in, or "" for app-level content.
func featureOf(value string) string {
	value = filepath.ToSlash(value)
	index := strings.Index(value, applayout.FeaturesDir+"/")
	if index < 0 {
		return ""
	}
	rest := value[index+len(applayout.FeaturesDir)+1:]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	return strings.TrimSuffix(rest, applayout.FeatureSuffix)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
