package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The overview's Terms and vocabulary: the project's own words, each with its
// definition, aliases, the stories it belongs to, and the code that defines
// it, rendered as that code reads at the head. The documentation carries no
// approvals or comments, so neither page offers them.

var errTermNotFound = errors.New("term not found")

type termsPageView struct {
	Active bool
	// Index is the Terms and vocabulary page, which is the table of every
	// term; otherwise Term is the one term a page shows.
	Index     bool
	Directory *directoryView
	Term      *termView
}

type termView struct {
	ID         string
	Target     string
	Href       string
	Name       string
	Definition string
	Aliases    []string
	Retired    bool
	Stale      bool
	Stories    []termLinkView
	Records    []termLinkView
	Code       []*termCodeView
}

type termLinkView struct {
	Title  string
	Href   string
	Target string
}

// termCodeView is one code reference rendered as code: the lines as they are
// at the head, or, when the code changed since the term was pinned, as they
// were at the pin.
type termCodeView struct {
	Path     string
	Location string
	Stale    bool
	Note     string
	Lines    []termCodeLine
}

type termCodeLine struct {
	Number int
	Text   string
	// Referenced marks the lines the term names; the others are context.
	Referenced bool
}

// termContext is the unreferenced lines shown around a term's code.
const termContext = 2

// maxTermCodeLines bounds the lines rendered for one reference, so a
// whole-file reference cannot render an entire large file.
const maxTermCodeLines = 60

func isTermsPath(value string) bool {
	return value == "/terms" || strings.HasPrefix(value, "/terms/")
}

func termHref(id string) string { return "/terms/" + url.PathEscape(id) }

// makeTermsPage projects the vocabulary for the Terms pages. Code is read
// only for the one term a page shows.
func (a *app) makeTermsPage(ctx context.Context, document requirements.Document, stories map[string]string, path, termID, query string) (*termsPageView, error) {
	page := &termsPageView{Active: isTermsPath(path)}
	if !page.Active {
		return page, nil
	}
	if termID == "" {
		page.Index = true
		page.Directory = termsDirectory(document, a.termPlaces(ctx, document), query)
		return page, nil
	}
	term := document.FindTerm(termID)
	if term == nil {
		return nil, errTermNotFound
	}
	page.Term = makeTermView(document, *term, stories)
	if term.CurrentRevision != nil {
		page.Term.Code = a.termCode(ctx, term.CurrentRevision.Code)
		for _, code := range page.Term.Code {
			page.Term.Stale = page.Term.Stale || code.Stale
		}
	}
	return page, nil
}

func makeTermView(document requirements.Document, term requirements.Term, stories map[string]string) *termView {
	urn, _ := requirements.TermURN(document.SagaID, term.Identity.ID)
	view := &termView{ID: term.Identity.ID, Target: urn, Href: termHref(term.Identity.ID), Name: term.Identity.ID, Retired: !term.Active()}
	revision := term.CurrentRevision
	if revision == nil {
		return view
	}
	view.Name, view.Definition, view.Aliases = revision.Name, revision.Definition, revision.Aliases
	for _, story := range revision.Stories {
		ref, err := livingid.Parse(story)
		if err != nil {
			continue
		}
		title := stories[story]
		if title == "" {
			title = ref.ID
		}
		view.Stories = append(view.Stories, termLinkView{Title: title, Href: requirementStoryHref(ref.ID), Target: story})
	}
	for _, record := range revision.Records {
		view.Records = append(view.Records, recordLink(document, record))
	}
	return view
}

// recordLink names another record a term references and links it when the
// reviewer has a page for it.
func recordLink(document requirements.Document, record string) termLinkView {
	link := termLinkView{Title: record, Target: record}
	parts := strings.Split(record, ":")
	if len(parts) != 5 {
		return link
	}
	kind, id := parts[3], parts[4]
	switch kind {
	case "term":
		link.Href = termHref(id)
		if term := document.FindTerm(id); term != nil && term.CurrentRevision != nil {
			link.Title = term.CurrentRevision.Name
		}
	case "persona":
		if persona := document.FindPersona(id); persona != nil && persona.CurrentRevision != nil {
			link.Title = "Persona: " + persona.CurrentRevision.Name
		}
	case "epic":
		if epic, ok := applayout.Find(document.Epics, id); ok {
			link.Title = "Epic: " + epic.Title
		}
	case "flag":
		link.Title = "Feature flag: " + id
	}
	return link
}

// termPlace is where one code reference of a term resolves now, and whether
// it still points at the code the term was written for.
type termPlace struct {
	Path string
	// Where is the reference as a reader reads it: the file, and the lines
	// inside it when the reference names some. The commit is left out, since
	// a current reference is at the head a reader is already looking at and a
	// stale one is stated as stale.
	Where string
	Stale bool
}

// termWhere names a resolved location the way a reader would say it.
func termWhere(location coderef.Location) string {
	if location.WholeFile() {
		return location.Path
	}
	if location.Start == location.End {
		return fmt.Sprintf("%s:%d", location.Path, location.Start)
	}
	return fmt.Sprintf("%s:%d-%d", location.Path, location.Start, location.End)
}

// termPlaces resolves every term's code references to where they are at the
// head. It reads only what resolving needs and never renders the code, so the
// vocabulary table can state where each term is defined without opening
// thirty files' worth of it. A reviewer with no code repository gets no
// places, which the table states as a gap rather than an error.
func (a *app) termPlaces(ctx context.Context, document requirements.Document) map[string][]termPlace {
	places := map[string][]termPlace{}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		return places
	}
	defer resolver.Close()
	head := firstNonEmptyString(a.rng.Head, "HEAD")
	headOID, _ := gitOutput(ctx, a.sourceDir, "rev-parse", "--verify", "--end-of-options", head+"^{commit}")
	for _, term := range document.Terms {
		if term.CurrentRevision == nil {
			continue
		}
		for _, reference := range term.CurrentRevision.Code {
			place := termPlace{Path: reference.Path, Where: termWhere(reference.Location())}
			if at := resolver.Resolve(ctx, reference, headOID); at.Current() {
				place.Path, place.Where = at.Location.Path, termWhere(at.Location)
			} else {
				place.Stale = true
			}
			places[term.Identity.ID] = append(places[term.Identity.ID], place)
		}
	}
	return places
}

// termCode renders each reference as code at the head, or at its pin when it
// went stale.
func (a *app) termCode(ctx context.Context, references []coderef.Reference) []*termCodeView {
	return a.referenceCode(ctx, references, "term")
}

// referenceCode renders each reference as code at the head, or at its pin
// when it went stale. subject names the record the references belong to.
func (a *app) referenceCode(ctx context.Context, references []coderef.Reference, subject string) []*termCodeView {
	var result []*termCodeView
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		for _, reference := range references {
			result = append(result, &termCodeView{Path: reference.Path, Location: reference.Location().String(), Note: "The code repository is not available to this reviewer."})
		}
		return result
	}
	defer resolver.Close()
	head := firstNonEmptyString(a.rng.Head, "HEAD")
	headOID, _ := gitOutput(ctx, a.sourceDir, "rev-parse", "--verify", "--end-of-options", head+"^{commit}")
	for _, reference := range references {
		view := &termCodeView{Path: reference.Path, Location: reference.Location().String()}
		location := reference.Location()
		if at := resolver.Resolve(ctx, reference, headOID); at.Current() {
			location = at.Location
		} else {
			view.Stale = true
			view.Note = "This code changed after the " + subject + " was written (" + at.Reason + "). It is shown as it was pinned; revise the " + subject + " to the code as it is now."
		}
		view.Path = location.Path
		content, found, err := resolver.Blob(ctx, location.Commit, location.Path)
		if err != nil || !found {
			view.Note = strings.TrimSpace(view.Note + " The code could not be read from this checkout.")
			result = append(result, view)
			continue
		}
		view.Lines = codeLines(content, location)
		if last := view.Lines; len(last) > 0 && !location.WholeFile() && last[len(last)-1].Number < location.End {
			view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" Showing lines %d–%d of the referenced %d–%d.", last[0].Number, last[len(last)-1].Number, location.Start, location.End))
		}
		result = append(result, view)
	}
	return result
}

// codeLines returns the referenced lines with a little context, bounded.
func codeLines(content []byte, location coderef.Location) []termCodeLine {
	lines := coderef.Lines(content)
	start, end := 1, len(lines)
	if !location.WholeFile() {
		start, end = max(1, location.Start-termContext), min(len(lines), location.End+termContext)
	}
	if end-start+1 > maxTermCodeLines {
		end = start + maxTermCodeLines - 1
	}
	var result []termCodeLine
	for number := start; number <= end; number++ {
		referenced := location.WholeFile() || number >= location.Start && number <= location.End
		result = append(result, termCodeLine{Number: number, Text: strings.TrimRight(string(lines[number-1]), "\r\n"), Referenced: referenced})
	}
	return result
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// overviewNav is the Overview place: the project's name, elevator pitch,
// description, and terms and vocabulary. Each absent part is a stated gap.
// active is "" on the overview itself, "/terms" on the vocabulary page, the
// term ID on a term's page, and anything else elsewhere.
func overviewNav(document *saga.Saga, vocabulary requirements.Document, active string) *navNodeView {
	overview := &navNodeView{Title: "Overview", Href: sagaHref(document.Section.Target), NodeID: "nav-overview", Expanded: true, Active: active == ""}
	name := &navNodeView{Title: "Name", Note: document.Manifest.Title, Href: sagaHref(document.Section.Target), NodeID: "nav-overview-name"}
	part := func(title, id, pkg string) *navNodeView {
		node := &navNodeView{Title: title, NodeID: id}
		if fragment := document.OverviewPart(pkg); fragment != nil {
			node.Href = sagaHref(fragment.Target)
		} else {
			node.Gap, node.Note = true, "not written yet"
		}
		return node
	}
	var terms []*navNodeView
	for _, term := range vocabulary.Terms {
		view := makeTermView(vocabulary, term, nil)
		node := &navNodeView{Title: view.Name, Href: view.Href, NodeID: "nav-" + domID(view.Target), Active: active == view.ID}
		if view.Retired {
			node.Note = "retired"
		}
		terms = append(terms, node)
	}
	vocabularyNode := &navNodeView{Title: "Terms and vocabulary", Href: "/terms", NodeID: "nav-terms", Children: terms, Active: active == "/terms"}
	if len(terms) == 0 {
		vocabularyNode.Gap, vocabularyNode.Note = true, "no terms yet"
	}
	// The vocabulary opens on the terms pages only. Opened on every other
	// page, its dozens of rows pushed the epics out of sight.
	vocabularyNode.Expanded = active == "/terms" || vocabulary.FindTerm(active) != nil
	overview.Children = []*navNodeView{
		name,
		part("Elevator pitch", "nav-overview-pitch", applayout.OverviewPitch),
		part("Description", "nav-overview-description", applayout.OverviewDescription),
		vocabularyNode,
	}
	return overview
}

// fileTermView is one term whose code lies in a file, with the lines it names.
type fileTermView struct {
	Name     string
	Href     string
	Target   string
	Location string
}

// fileTerms returns the terms whose code, viewed at any of commits, lies in
// path: the code view's way back to the vocabulary a file defines.
func (a *app) fileTerms(ctx context.Context, sagaID string, commits []string, path string) []fileTermView {
	vocabulary, err := requirements.Load(a.root, sagaID)
	if err != nil || len(vocabulary.Terms) == 0 {
		return nil
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		return nil
	}
	defer resolver.Close()
	var result []fileTermView
	for _, term := range vocabulary.Terms {
		if term.CurrentRevision == nil {
			continue
		}
		view := makeTermView(vocabulary, term, nil)
	references:
		for _, reference := range term.CurrentRevision.Code {
			for _, commit := range commits {
				if commit == "" {
					continue
				}
				if at := resolver.Resolve(ctx, reference, commit); at.Current() && at.Location.Path == path {
					result = append(result, fileTermView{Name: view.Name, Href: view.Href, Target: view.Target, Location: at.Location.String()})
					break references
				}
			}
		}
	}
	return result
}
