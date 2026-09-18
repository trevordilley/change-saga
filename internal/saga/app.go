package saga

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
)

// Epic is the report view of one durable product domain: its report content,
// technical design, and implementation deck. Its nodes are the same pointers
// the app-wide Section tree holds, so target indexes see one tree while
// readers keep the grouping.
type Epic struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Path   string `json:"path"`
	Target string `json:"target"`
	// Report holds the epic's own chapters and fragments.
	Report *Section `json:"report"`
	// Design holds the chapters and fragments loaded from its ___design.
	Design *Section `json:"design"`
	Decks  []*Deck  `json:"decks,omitempty"`
}

type appContent struct {
	overview     *Section
	designSystem *Section
	onboarding   []*Deck
	epics        []*Epic
}

// DeckRoleChange is an epic's implementation deck; DeckRoleOnboarding is the
// app's onboarding deck, whose Items reference records instead of code.
const (
	DeckRoleChange     = "change"
	DeckRoleOnboarding = "onboarding"
)

// loadAppContent loads the app-level report roots, every epic, and the
// onboarding deck, joining their nodes into root so each target stays
// addressable through the one Section tree.
func loadAppContent(root string, manifest Manifest, section *Section, options loadOptions, validation *Validation) (appContent, []*Deck, error) {
	var app appContent
	var decks []*Deck
	join := func(part *Section) {
		if part == nil {
			return
		}
		section.Fragments = append(section.Fragments, part.Fragments...)
		section.Children = append(section.Children, part.Children...)
	}
	var err error
	if app.overview, err = loadReportRoot(root, filepath.Join(root, applayout.OverviewDir), manifest, "overview", "Overview", options, validation); err != nil {
		return app, nil, err
	}
	join(app.overview)
	if app.designSystem, err = loadReportRoot(root, filepath.Join(root, applayout.DesignSystemDir), manifest, "designsystem", "Design system", options, validation); err != nil {
		return app, nil, err
	}
	join(app.designSystem)

	epics, epicErr := applayout.Epics(root)
	if epicErr != nil {
		addIssue(validation, "error", applayout.EpicsDir, epicErr.Error())
	}
	for _, value := range epics {
		epic := &Epic{ID: value.ID, Title: value.Title, Path: value.Rel, Target: applayout.EpicURN(manifest.ID, value.ID)}
		report, err := loadSection(root, value.Dir, manifest, epicHierarchy, options, validation)
		if err != nil {
			return app, nil, err
		}
		report.Title = value.Title
		epic.Report = report
		join(report)
		if epic.Design, err = loadReportRoot(root, filepath.Join(value.Dir, applayout.DesignDir), manifest, "design", "Technical design", options, validation); err != nil {
			return app, nil, err
		}
		join(epic.Design)
		if metadataDirectorySafe(root, value.Dir, EmbeddedSlidesDir, validation) {
			epic.Decks, err = loadEmbeddedDecks(root, filepath.Join(value.Dir, EmbeddedSlidesDir), DeckRoleChange, manifest, options, validation)
			if err != nil {
				return app, nil, err
			}
		}
		decks = append(decks, epic.Decks...)
		app.epics = append(app.epics, epic)
	}
	if metadataDirectorySafe(root, root, applayout.OnboardingDir, validation) {
		app.onboarding, err = loadEmbeddedDecks(root, filepath.Join(root, applayout.OnboardingDir), DeckRoleOnboarding, manifest, options, validation)
		if err != nil {
			return app, nil, err
		}
		if len(app.onboarding) > 1 {
			addIssue(validation, "error", applayout.OnboardingDir, "an app has one onboarding deck")
		}
	}
	// The query and coverage applications understand deck/slide/Item nodes
	// through the section projection.
	if all := append(append([]*Deck{}, decks...), app.onboarding...); len(all) > 0 {
		section.Children = append(section.Children, projectDecks(manifest, all).Children...)
	}
	sortSectionContents(section)
	return app, decks, nil
}

// loadReportRoot loads one authored report root that is not an epic, or nil
// when it is absent.
func loadReportRoot(root, dir string, manifest Manifest, kind, title string, options loadOptions, validation *Validation) (*Section, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			addIssue(validation, "error", relativePath(root, dir), "reserved metadata path must be a real directory")
		}
		return nil, nil
	}
	section, err := loadSection(root, dir, manifest, designHierarchy, options, validation)
	if err != nil {
		return nil, err
	}
	section.Kind, section.Title = kind, title
	return section, nil
}

// IsDesignPath reports whether an app-relative node path was loaded from an
// epic's ___design root.
func IsDesignPath(path string) bool {
	return strings.Contains("/"+path+"/", "/"+applayout.DesignDir+"/")
}

// IsOverviewPath and IsDesignSystemPath report whether an app-relative node
// path belongs to the app's ___overview or ___designsystem roots.
func IsOverviewPath(path string) bool {
	return path == applayout.OverviewDir || strings.HasPrefix(path, applayout.OverviewDir+"/")
}

func IsDesignSystemPath(path string) bool {
	return path == applayout.DesignSystemDir || strings.HasPrefix(path, applayout.DesignSystemDir+"/")
}

// EpicOf returns the epic that holds an app-relative node path, or "".
func EpicOf(path string) string { return applayout.EpicOfPath(path) }

// FindEpic returns the loaded epic with id.
func (document *Saga) FindEpic(id string) *Epic {
	for _, epic := range document.Epics {
		if epic.ID == id {
			return epic
		}
	}
	return nil
}
