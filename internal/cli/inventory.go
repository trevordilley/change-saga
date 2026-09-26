package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func Technical(ctx context.Context, kind string, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	err := technicalOperation(ctx, kind, args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, kind, err)
	}
	return err
}

// TechnicalKinds are the public inventory authoring commands.
var TechnicalKinds = []string{"component", "system", requirements.KindDataEntity, requirements.KindERD, requirements.KindERDOverlay}

func technicalOperation(ctx context.Context, kind string, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp(kind, []string{"add", "revise", "set-state"}, out)
	}
	op := args[0]
	name := kind + " " + op
	flags := commandFlags(name, technicalUsage(kind, op), out)
	id := flags.String("id", "", "stable record ID (never a display name)")
	revision := flags.String("revision", "r1", "immutable revision ID")
	from := flags.String("from", "", "complete definition JSON; code commits may name HEAD and digests may be omitted")
	repo := flags.String("repo", "", "source checkout")
	delivery := flags.String("delivery", "", "delivery commit for an implemented revision (resolved once to a full OID)")
	visual := flags.String("visual", "", "offline SVG for an ERD or overlay revision")
	event := flags.String("event", "", "new lifecycle event ID")
	state := flags.String("state", "", "active or retired")
	reason := flags.String("reason", "", "reason for lifecycle change")
	jsonOutput := flags.Bool("json", false, "machine-readable mutation result")
	var parents stringList
	flags.Var(&parents, "parent", "observed revision or lifecycle head URN; repeat for reconciliation")
	if err := flags.Parse(normalizeLivingArgs(args[1:])); err != nil {
		return err
	}
	if flags.NArg() != 1 || *id == "" {
		return fmt.Errorf("requires --id and one Saga path")
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if manifest.Version != 5 {
		return fmt.Errorf("technical inventory documentation requires a v5 Saga")
	}
	var result requirements.MutationResult
	switch op {
	case "add", "revise":
		if *from == "" {
			return fmt.Errorf("requires --from FILE|-")
		}
		data, e := readBoundedInput(*from, 1<<20, "definition")
		if e != nil {
			return e
		}
		var def requirements.TechnicalDefinition
		if e := decodeStrictInventory(data, &def); e != nil {
			return e
		}
		checkout := firstNonEmpty(*repo, root)
		if _, e := gitdiff.ReadCatalogRange(ctx, checkout, manifest.Source.Repository, gitdiff.Range{Head: "HEAD"}, gitdiff.ReadOptions{}); e != nil {
			return e
		}
		resolver, e := coderesolve.New(ctx, checkout)
		if e != nil {
			return e
		}
		defer resolver.Close()
		if e := authorTechnicalEvidence(ctx, checkout, resolver, &def); e != nil {
			return e
		}
		if e := resolveDelivery(ctx, checkout, manifest.Source.Repository, *delivery, &def); e != nil {
			return e
		}
		var svg []byte
		if *visual != "" {
			if svg, e = readBoundedInput(*visual, requirements.MaxVisualBytes, "visual"); e != nil {
				return e
			}
			if e := bindVisual(svg, &def); e != nil {
				return e
			}
		}
		result, err = requirements.WriteTechnicalRevision(ctx, root, manifest.ID, requirements.TechnicalWrite{
			Kind: kind, ID: *id, RevisionID: *revision, Parents: parents, Definition: def, Create: op == "add",
			Repository: manifest.Source.Repository, Resolver: resolver, Visual: svg,
		})
	case "set-state":
		result, err = requirements.SetTechnicalState(root, manifest.ID, kind, *id, *event, *state, *reason, parents)
	default:
		return fmt.Errorf("unknown %s operation %s", kind, op)
	}
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func readBoundedInput(path string, limit int64, label string) ([]byte, error) {
	var reader io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		reader = f
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return data, nil
}

// authorTechnicalEvidence pins symbolic commits and computes (or verifies)
// digests from source bytes for every owner's references, keeping evidence IDs.
func authorTechnicalEvidence(ctx context.Context, checkout string, resolver *coderesolve.Resolver, def *requirements.TechnicalDefinition) error {
	author := func(refs []requirements.Evidence) error {
		for i, r := range refs {
			loc, e := resolveLocation(ctx, checkout, r.Location().String())
			if e != nil {
				return e
			}
			pinned, e := resolver.Author(ctx, loc, r.Note)
			if e != nil {
				return e
			}
			if r.Digest != "" && r.Digest != pinned.Digest {
				return fmt.Errorf("code digest does not match %s", loc)
			}
			refs[i].Reference = pinned
		}
		return nil
	}
	if e := author(def.Code); e != nil {
		return e
	}
	for i := range def.Interactions {
		if e := author(def.Interactions[i].Code); e != nil {
			return e
		}
	}
	for i := range def.Relationships {
		if e := author(def.Relationships[i].Code); e != nil {
			return e
		}
	}
	return nil
}

// resolveDelivery resolves the delivery commit once at the command boundary
// and binds it to the Saga's canonical source repository.
func resolveDelivery(ctx context.Context, checkout, repository, flag string, def *requirements.TechnicalDefinition) error {
	supplied := ""
	if def.Delivery != nil {
		supplied = def.Delivery.Commit
		if def.Delivery.Repository != "" && def.Delivery.Repository != repository {
			return fmt.Errorf("delivery repository %q is not the Saga's canonical source %q", def.Delivery.Repository, repository)
		}
	}
	if flag == "" && supplied == "" {
		if def.Intent == requirements.IntentImplemented {
			return fmt.Errorf("an implemented revision requires --delivery REV (or delivery.commit)")
		}
		return nil
	}
	if def.Intent != requirements.IntentImplemented {
		return fmt.Errorf("delivery applies only to an implemented revision")
	}
	commit := ""
	for _, revision := range []string{flag, supplied} {
		if revision == "" {
			continue
		}
		oid, err := resolveCommit(ctx, checkout, revision)
		if err != nil {
			return err
		}
		if commit != "" && commit != oid {
			return fmt.Errorf("--delivery and delivery.commit name different commits")
		}
		commit = oid
	}
	def.Delivery = &requirements.Delivery{Repository: repository, Commit: commit}
	return nil
}

func bindVisual(svg []byte, def *requirements.TechnicalDefinition) error {
	digest := coderef.DigestBytes(svg)
	hexDigest := strings.TrimPrefix(digest, coderef.DigestPrefix)
	visual := requirements.Visual{Path: "assets/" + hexDigest + ".svg", MediaType: "image/svg+xml", Digest: digest}
	if def.Visual != nil && *def.Visual != visual {
		return fmt.Errorf("definition visual does not match the supplied SVG")
	}
	def.Visual = &visual
	return nil
}

// Inventory runs inventory-wide operations; today only explicit format adoption.
func Inventory(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	err := inventoryOperation(args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "inventory", err)
	}
	return err
}

func inventoryOperation(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("inventory", []string{"adopt-format"}, out)
	}
	if args[0] != "adopt-format" {
		return fmt.Errorf("unknown inventory operation %s", args[0])
	}
	flags := commandFlags("inventory adopt-format", commandUsage["inventory adopt-format"], out)
	format := flags.Int("format", 0, "inventory format to adopt (2)")
	jsonOutput := flags.Bool("json", false, "machine-readable mutation result")
	if err := flags.Parse(normalizeLivingArgs(args[1:])); err != nil {
		return err
	}
	if flags.NArg() != 1 || *format == 0 {
		return fmt.Errorf("requires --format 2 and one Saga path")
	}
	manifest, err := saga.ReadManifest(flags.Arg(0))
	if err != nil {
		return err
	}
	result, err := requirements.AdoptInventoryFormat(flags.Arg(0), manifest.ID, *format)
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, "inventory adopt-format", result, nil, *jsonOutput)
}

func technicalUsage(kind, op string) string {
	usage := "change-saga " + kind + " " + op + " --id ID [--from FILE|- --revision ID | --event ID --state STATE --reason TEXT] [--parent URN]"
	switch kind {
	case requirements.KindERD, requirements.KindERDOverlay:
		usage += " [--visual SVG]"
	default:
		usage += " [--delivery REV]"
	}
	return usage + " [--repo PATH] [--json] <saga>"
}

func decodeStrictInventory(data []byte, value any) error {
	return requirements.DecodeInventoryJSON(data, value)
}

func appendInventoryIssues(root string, document *saga.Saga, validation *saga.Validation) {
	if document == nil {
		return
	}
	d, err := requirements.LoadInventory(root, document.Manifest.ID)
	if err != nil {
		appendLoadIssues(validation, requirements.InventoryDir, err)
		return
	}
	report := func(path string, pin saga.DocumentationLink) {
		status := d.LinkStatus(pin)
		if status != "current" {
			severity := "warning"
			if status == "missing" {
				severity = "error"
				validation.Valid = false
			}
			validation.Issues = append(validation.Issues, saga.Issue{Severity: severity, Path: path, Message: "documentation reference " + pin.Target + " is " + status})
		}
	}
	for _, r := range d.Records {
		if r.CurrentRevision == nil || r.CurrentLifecycle == nil {
			validation.Issues = append(validation.Issues, saga.Issue{Severity: "warning", Path: requirements.TechnicalPath(r.Kind, r.Identity.ID), Message: "documentation has competing heads"})
		}
		for _, rev := range r.Revisions {
			for _, pin := range rev.Components {
				if d.LinkStatus(pin) == "missing" {
					report(r.Target+":revision:"+rev.ID, pin)
				}
			}
		}
		if r.CurrentRevision != nil {
			for _, pin := range r.CurrentRevision.Components {
				if d.LinkStatus(pin) != "missing" {
					report(r.Target, pin)
				}
			}
		}
	}
	checkItem := func(item *saga.Item) {
		if item.Documentation == nil {
			return
		}
		if item.DocumentationView == "" {
			report(item.Path, *item.Documentation)
		}
		formatTwo := item.DocumentationView != "" || len(item.Selections) > 0 || strings.Contains(item.Documentation.Target, ":"+requirements.KindDataEntity+":")
		if formatTwo && d.Format < 2 {
			validation.Valid = false
			validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: item.Path, Message: "Item uses inventory format 2 content without ___inventory/format.json"})
		}
		// Saved revisions are append-only, so a selection that no longer
		// resolves structurally is damaged metadata, not ordinary drift.
		for _, selection := range item.Selections {
			if _, err := d.ResolveSelection(*item.Documentation, selection); err != nil {
				validation.Valid = false
				validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: item.Path, Message: "selection " + selection.ID + " does not resolve: " + err.Error()})
			}
		}
	}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				checkItem(item)
			}
		}
	}
	for _, review := range document.Reviews {
		if review.Deck == nil {
			// Loading already reports the missing deck; keep validation diagnostic.
			continue
		}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				checkItem(item)
			}
		}
	}
}

// Keep selectors explicit: ID is accepted only with an explicit kind.
func inventoryTarget(sagaID, kind, id string) (string, error) {
	if strings.HasPrefix(id, "urn:") {
		if !saga.ValidDocumentationLink(sagaID, saga.DocumentationLink{Target: id, Revision: id + ":revision:probe"}) {
			return "", fmt.Errorf("invalid inventory target")
		}
		return id, nil
	}
	return requirements.TechnicalURN(sagaID, kind, id)
}

func init() {
	for _, kind := range TechnicalKinds {
		for _, op := range []string{"add", "revise", "set-state"} {
			commandUsage[kind+" "+op] = technicalUsage(kind, op)
		}
	}
	commandUsage["inventory adopt-format"] = "change-saga inventory adopt-format --format 2 [--json] <saga>"
}
