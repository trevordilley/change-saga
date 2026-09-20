package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
)

var prototypeOperations = []string{"add-html", "add-external", "revise", "annotate"}

// Prototype exposes the existing v3 prototype model. Prototypes are revisioned
// interactive HTML experiences or explicitly allowed external embeds; a
// prototype may stay unlinked while exploration continues, so authoring never
// requires an annotation.
func Prototype(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("prototype", prototypeOperations, out)
	}
	operation := "prototype " + args[0]
	var err error
	switch args[0] {
	case "add-html":
		err = prototypeAddHTML(ctx, args[1:], out)
	case "add-external":
		err = prototypeAddExternal(ctx, args[1:], out)
	case "revise":
		err = prototypeRevise(ctx, args[1:], out)
	case "annotate":
		err = prototypeAnnotate(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["prototype"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

// prototypeSagaID reads the identity through the prototype loader so the
// command family stays independent of the requirements domain.
func prototypeSagaID(root string) (string, error) {
	document, err := prototypes.Load(root, "")
	if err != nil {
		return "", err
	}
	return document.SagaID, nil
}

func prototypeAddHTML(_ context.Context, args []string, out io.Writer) error {
	name := "prototype add-html"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable prototype id")
	revision := flags.String("revision", "", "stable initial revision id")
	title := flags.String("title", "", "prototype title")
	source := flags.String("source", "", "single .html file or real directory containing index.html")
	state := flags.String("state", string(prototypes.StateDraft), "draft, ready, or retired")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *revision, *title, *source); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := prototypeSagaID(root)
	if err != nil {
		return err
	}
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	result, err := prototypes.AddHTML(root, sagaID, prototypes.AddHTMLInput{
		Feature: target.ID, ID: *id, RevisionID: *revision, Title: *title, State: prototypes.State(*state),
		SourcePath: *source, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

func prototypeAddExternal(_ context.Context, args []string, out io.Writer) error {
	name := "prototype add-external"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable prototype id")
	revision := flags.String("revision", "", "stable initial revision id")
	title := flags.String("title", "", "prototype title")
	state := flags.String("state", string(prototypes.StateDraft), "draft, ready, or retired")
	external := registerExternalPrototypeFlags(flags)
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *revision, *title, *external.url); err != nil {
		return err
	}
	allowlist, err := external.allowlist()
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := prototypeSagaID(root)
	if err != nil {
		return err
	}
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	result, err := prototypes.AddExternal(root, sagaID, prototypes.AddExternalInput{
		Feature: target.ID, ID: *id, RevisionID: *revision, Title: *title, State: prototypes.State(*state),
		URL: *external.url, EmbedURL: *external.embedURL, FallbackURL: *external.fallbackURL,
		Allowlist: allowlist, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

func prototypeRevise(_ context.Context, args []string, out io.Writer) error {
	name := "prototype revise"
	flags := commandFlags(name, commandUsage[name], out)
	prototype := flags.String("prototype", "", "canonical prototype URN")
	revision := flags.String("revision", "", "stable revision id")
	title := flags.String("title", "", "complete revised title")
	state := flags.String("state", string(prototypes.StateDraft), "draft, ready, or retired")
	source := flags.String("source", "", "fresh .html file or real directory for an html revision")
	external := registerExternalPrototypeFlags(flags)
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	var parents stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *prototype, *revision, *title); err != nil {
		return err
	}
	htmlRevision, externalRevision := *source != "", *external.url != "" || *external.embedURL != ""
	if htmlRevision == externalRevision {
		return fmt.Errorf("provide exactly one revised source: --source for an html revision, or --url for an external or embedded revision")
	}
	if externalRevision && *external.url == "" {
		return fmt.Errorf("--url is required for an external or embedded revision")
	}
	allowlist, err := external.allowlist()
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := prototypeSagaID(root)
	if err != nil {
		return err
	}
	input := prototypes.ReviseInput{
		Prototype: *prototype, ID: *revision, Parents: parents, Title: *title,
		State: prototypes.State(*state), RequestID: *requestID,
	}
	if htmlRevision {
		input.HTMLSourcePath = *source
	} else {
		input.Source = externalPrototypeSource(*external.url, *external.embedURL, *external.fallbackURL, allowlist)
	}
	if err := assertRecordFeature(root, *feature, *prototype); err != nil {
		return err
	}
	result, err := prototypes.Revise(root, sagaID, input)
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

func prototypeAnnotate(_ context.Context, args []string, out io.Writer) error {
	name := "prototype annotate"
	flags := commandFlags(name, commandUsage[name], out)
	prototype := flags.String("prototype", "", "canonical prototype URN")
	id := flags.String("id", "", "stable annotation id")
	target := flags.String("target", "", "canonical story or criterion URN the prototype is pinned to")
	rationale := flags.String("rationale", "", "why this part of the prototype means that requirement")
	prototypeRevision := flags.String("prototype-revision", "", "exact prototype revision URN pin")
	contentDigest := flags.String("prototype-content-digest", "", "exact prototype content digest pin")
	storyRevision := flags.String("story-revision", "", "exact story revision URN the target is pinned to")
	elementID := flags.String("element-id", "", "element selector: stable id inside the prototype")
	text := flags.String("text", "", "text selector: exact text inside the prototype")
	region := flags.String("region", "", "region selector: normalized x,y,width,height")
	providerID := flags.String("provider-id", "", "provider selector: provider-native node id")
	deepLink := flags.String("deep-link", "", "provider selector: absolute deep link")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *prototype, *id, *target, *rationale, *storyRevision); err != nil {
		return err
	}
	if (*prototypeRevision == "") == (*contentDigest == "") {
		return fmt.Errorf("provide exactly one prototype pin: --prototype-revision or --prototype-content-digest")
	}
	selector, err := parsePrototypeSelector(*elementID, *text, *region, *providerID, *deepLink)
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := prototypeSagaID(root)
	if err != nil {
		return err
	}
	featureID := ""
	if *feature != "" {
		resolved, err := requireFeature(root, *feature)
		if err != nil {
			return err
		}
		featureID = resolved.ID
	}
	result, err := prototypes.AddAnnotation(root, sagaID, prototypes.AddAnnotationInput{
		Feature: featureID, ID: *id, Prototype: *prototype, Target: *target, Rationale: *rationale,
		PrototypeRevision: *prototypeRevision, PrototypeContentDigest: *contentDigest,
		StoryRevision: *storyRevision, Selector: selector, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

// externalPrototypeFlags is shared by add-external and revise so both spell an
// explicitly allowed embed the same way.
type externalPrototypeFlags struct {
	url         *string
	embedURL    *string
	fallbackURL *string
	provider    *string
	embedOrigin *string
	sandbox     stringList
	permission  stringList
}

func registerExternalPrototypeFlags(flags *flag.FlagSet) *externalPrototypeFlags {
	value := &externalPrototypeFlags{}
	value.url = flags.String("url", "", "canonical external prototype URL")
	value.embedURL = flags.String("embed-url", "", "explicitly allowed https embed URL")
	value.fallbackURL = flags.String("fallback-url", "", "link shown when the embed cannot render; defaults to --url")
	value.provider = flags.String("provider", "", "allowlisted embed provider id")
	value.embedOrigin = flags.String("embed-origin", "", "allowlisted embed origin; must equal the --embed-url origin")
	flags.Var(&value.sandbox, "sandbox", "allowed iframe sandbox token; repeatable")
	flags.Var(&value.permission, "permission", "allowed iframe permission token; repeatable")
	return value
}

// allowlist returns nil unless the author declared embed permission, so the
// domain refuses an embed whose hostname merely looks recognizable.
func (f *externalPrototypeFlags) allowlist() (*prototypes.ProviderAllowlist, error) {
	declared := *f.provider != "" || *f.embedOrigin != "" || len(f.sandbox) > 0 || len(f.permission) > 0
	if *f.embedURL == "" {
		if declared || *f.fallbackURL != "" {
			return nil, fmt.Errorf("--provider, --embed-origin, --sandbox, --permission, and --fallback-url require --embed-url")
		}
		return nil, nil
	}
	if !declared {
		return nil, nil
	}
	return &prototypes.ProviderAllowlist{
		Provider: *f.provider, EmbedOrigin: *f.embedOrigin,
		Sandbox: f.sandbox, Permissions: f.permission,
	}, nil
}

func externalPrototypeSource(url, embedURL, fallbackURL string, allowlist *prototypes.ProviderAllowlist) prototypes.Source {
	if embedURL == "" {
		return prototypes.Source{Kind: prototypes.SourceExternal, URL: url}
	}
	fallback := fallbackURL
	if fallback == "" {
		fallback = url
	}
	return prototypes.Source{Kind: prototypes.SourceEmbed, EmbedURL: embedURL, FallbackURL: fallback, Allowlist: allowlist}
}

func parsePrototypeSelector(elementID, text, region, providerID, deepLink string) (prototypes.Selector, error) {
	provided := 0
	for _, declared := range []bool{
		strings.TrimSpace(elementID) != "", strings.TrimSpace(text) != "",
		strings.TrimSpace(region) != "", strings.TrimSpace(providerID) != "" || strings.TrimSpace(deepLink) != "",
	} {
		if declared {
			provided++
		}
	}
	if provided != 1 {
		return prototypes.Selector{}, fmt.Errorf("provide exactly one selector: --element-id, --text, --region, or --provider-id/--deep-link")
	}
	switch {
	case elementID != "":
		return prototypes.Selector{Kind: prototypes.SelectorElement, ElementID: elementID}, nil
	case text != "":
		return prototypes.Selector{Kind: prototypes.SelectorText, ExactText: text}, nil
	case region != "":
		parsed, err := parseLandmarkRegion(region)
		if err != nil {
			return prototypes.Selector{}, fmt.Errorf("invalid --region: %w", err)
		}
		return prototypes.Selector{Kind: prototypes.SelectorRegion, Region: &prototypes.Region{
			X: parsed.X, Y: parsed.Y, Width: parsed.Width, Height: parsed.Height,
		}}, nil
	default:
		return prototypes.Selector{Kind: prototypes.SelectorProvider, ProviderID: providerID, DeepLink: deepLink}, nil
	}
}
