package saga

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
)

// Feature is the report view of one durable product domain: its report content,
// technical design, and implementation deck. Its nodes are the same pointers
// the app-wide Section tree holds, so target indexes see one tree while
// readers keep the grouping.
type Feature struct {
	Plan   applayout.FeaturePlan `json:"plan"`
	ID     string                `json:"id"`
	Title  string                `json:"title"`
	Path   string                `json:"path"`
	Target string                `json:"target"`
	// Report holds the feature's own chapters and fragments.
	Report *Section `json:"report"`
	// Design holds the chapters and fragments loaded from its ___design.
	Design *Section `json:"design"`
	Decks  []*Deck  `json:"decks,omitempty"`
}

// FeatureTheme owns a product overview and a rationale deck, independently of
// feature ownership. The deck uses the ordinary slide and Item model.
type FeatureTheme struct {
	applayout.FeatureTheme
	Target string  `json:"target"`
	Path   string  `json:"path"`
	Decks  []*Deck `json:"decks,omitempty"`
}

func (document *Saga) ThemeDecks() []*Deck {
	var decks []*Deck
	for _, theme := range document.FeatureThemes {
		decks = append(decks, theme.Decks...)
	}
	return decks
}

func (document *Saga) DocumentationDecks() []*Deck {
	decks := append([]*Deck{}, document.Decks...)
	decks = append(decks, document.Onboarding...)
	return append(decks, document.ThemeDecks()...)
}

func (document *Saga) FindFeatureTheme(value string) *FeatureTheme {
	id, ok := applayout.FeatureThemeID(document.Manifest.ID, value)
	if !ok {
		return nil
	}
	for _, theme := range document.FeatureThemes {
		if theme.ID == id {
			return theme
		}
	}
	return nil
}

type appContent struct {
	classifications applayout.Classifications
	overview        *Section
	designSystem    *Section
	onboarding      []*Deck
	features        []*Feature
	themes          []*FeatureTheme
}

// DeckRoleFeatureTheme explains product rationale without implementation gates.
// DeckRoleChange is a feature's implementation deck; DeckRoleOnboarding is the
// app's onboarding deck, whose Items reference records instead of code;
// DeckRoleReview is a pull request review's deck, whose Items reference the
// code the change touched and the records it revised.
const (
	DeckRoleFeatureTheme = "feature-theme"
	DeckRoleChange       = "change"
	DeckRoleOnboarding   = "onboarding"
	DeckRoleReview       = "review"
)

// loadAppContent loads the app-level report roots, every feature, and the
// onboarding deck, joining their nodes into root so each target stays
// addressable through the one Section tree.
func loadAppContent(root string, manifest Manifest, section *Section, options loadOptions, validation *Validation) (appContent, []*Deck, error) {
	var app appContent
	var decks []*Deck
	// The app root holds no report content of its own, so the joined tree is
	// ordered by place rather than by path: the overview, the design system,
	// then every feature's report and design in feature order. Each part keeps the
	// authored order its own loader gave it.
	join := func(part *Section) {
		if part == nil {
			return
		}
		section.Fragments = append(section.Fragments, part.Fragments...)
		section.Children = append(section.Children, part.Children...)
	}
	var err error
	if app.overview, err = loadReportRootAs(root, filepath.Join(root, applayout.OverviewDir), manifest, overviewHierarchy, "overview", "Overview", options, validation); err != nil {
		return app, nil, err
	}
	join(app.overview)
	if app.designSystem, err = loadReportRoot(root, filepath.Join(root, applayout.DesignSystemDir), manifest, "designsystem", "Design system", options, validation); err != nil {
		return app, nil, err
	}
	join(app.designSystem)

	app.classifications, err = applayout.ReadClassifications(root)
	if err != nil {
		addIssue(validation, "error", "classifications", err.Error())
	}
	features, featureErr := applayout.Features(root)
	if featureErr != nil {
		addIssue(validation, "error", applayout.FeaturesDir, featureErr.Error())
	}
	for _, value := range applayout.InCreationOrder(features) {
		if err := app.classifications.ValidatePlan(manifest.ID, value.Plan); err != nil {
			addIssue(validation, "error", value.Rel+"/plan.json", err.Error())
		}
		feature := &Feature{Plan: value.Plan, ID: value.ID, Title: value.Title, Path: value.Rel, Target: applayout.FeatureURN(manifest.ID, value.ID)}
		report, err := loadSection(root, value.Dir, manifest, featureHierarchy, options, validation)
		if err != nil {
			return app, nil, err
		}
		report.Title = value.Title
		feature.Report = report
		join(report)
		if feature.Design, err = loadReportRoot(root, filepath.Join(value.Dir, applayout.DesignDir), manifest, "design", "Technical design", options, validation); err != nil {
			return app, nil, err
		}
		join(feature.Design)
		if metadataDirectorySafe(root, value.Dir, EmbeddedSlidesDir, validation) {
			feature.Decks, err = loadEmbeddedDecks(root, filepath.Join(value.Dir, EmbeddedSlidesDir), DeckRoleChange, manifest, options, validation)
			if err != nil {
				return app, nil, err
			}
		}
		decks = append(decks, feature.Decks...)
		app.features = append(app.features, feature)
	}
	themes, themeErr := applayout.FeatureThemes(root, manifest.ID)
	if themeErr != nil {
		addIssue(validation, "error", applayout.FeatureThemesDir, themeErr.Error())
	}
	var themeDecks []*Deck
	for _, value := range themes {
		theme := &FeatureTheme{FeatureTheme: value, Target: applayout.FeatureThemeURN(manifest.ID, value.ID), Path: applayout.FeatureThemeRel(value.ID)}
		dir := filepath.Join(root, theme.Path)
		if metadataDirectorySafe(root, dir, EmbeddedSlidesDir, validation) {
			theme.Decks, err = loadEmbeddedDecks(root, filepath.Join(dir, EmbeddedSlidesDir), DeckRoleFeatureTheme, manifest, options, validation)
			if err != nil {
				return app, nil, err
			}
			if len(theme.Decks) > 1 {
				addIssue(validation, "error", theme.Path, "a feature theme has one rationale deck")
			}
		}
		themeDecks = append(themeDecks, theme.Decks...)
		app.themes = append(app.themes, theme)
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
	if all := append(append(append([]*Deck{}, decks...), app.onboarding...), themeDecks...); len(all) > 0 {
		section.Children = append(section.Children, projectDecks(manifest, all).Children...)
	}
	return app, decks, nil
}

// loadReportRoot loads one authored report root that is not a feature, or nil
// when it is absent.
func loadReportRoot(root, dir string, manifest Manifest, kind, title string, options loadOptions, validation *Validation) (*Section, error) {
	return loadReportRootAs(root, dir, manifest, designHierarchy, kind, title, options, validation)
}

func loadReportRootAs(root, dir string, manifest Manifest, hierarchy hierarchyRoot, kind, title string, options loadOptions, validation *Validation) (*Section, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			addIssue(validation, "error", relativePath(root, dir), "reserved metadata path must be a real directory")
		}
		return nil, nil
	}
	section, err := loadSection(root, dir, manifest, hierarchy, options, validation)
	if err != nil {
		return nil, err
	}
	section.Kind, section.Title = kind, title
	return section, nil
}

// IsDesignPath reports whether an app-relative node path was loaded from an
// feature's ___design root.
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

// OverviewPart returns the overview's elevator pitch (applayout.OverviewPitch)
// description or data-model overview fragment, or nil when that
// part is not written yet. An absent part is a visible gap, never an error.
func (document *Saga) OverviewPart(name string) *Fragment {
	if document == nil || document.Overview == nil {
		return nil
	}
	want := applayout.OverviewDir + "/" + name
	for _, fragment := range document.Overview.Fragments {
		if fragment.Path == want {
			return fragment
		}
	}
	return nil
}

// FeatureOf returns the feature that holds an app-relative node path, or "".
func FeatureOf(path string) string { return applayout.FeatureOfPath(path) }

// FindFeature returns the loaded feature with id.
func (document *Saga) FindFeature(id string) *Feature {
	for _, feature := range document.Features {
		if feature.ID == id {
			return feature
		}
	}
	return nil
}

// Overview returns the optional authored overview in the ordinary feature report.
// Its identity, content, links and Git history are those of a normal fragment.
func (feature *Feature) Overview() *Fragment {
	if feature == nil || feature.Report == nil {
		return nil
	}
	for _, fragment := range feature.Report.Fragments {
		if fragment.Path == feature.Path+"/"+applayout.FeatureOverview {
			return fragment
		}
	}
	return nil
}
