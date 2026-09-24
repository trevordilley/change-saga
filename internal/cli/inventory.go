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
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func Technical(ctx context.Context, kind string, args []string, out io.Writer) error {
	err := technicalOperation(ctx, kind, args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, kind, err)
	}
	return err
}

func technicalOperation(ctx context.Context, kind string, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp(kind, []string{"add", "revise", "set-state"}, out)
	}
	op := args[0]
	name := kind + " " + op
	flags := commandFlags(name, "change-saga "+name+" --id ID [--from FILE|- --revision ID | --event ID --state STATE --reason TEXT] [--parent URN] [--repo PATH] [--json] <saga>", out)
	id := flags.String("id", "", "stable record ID (never a display name)")
	revision := flags.String("revision", "r1", "immutable revision ID")
	from := flags.String("from", "", "complete definition JSON; code commits may name HEAD and digests may be omitted")
	repo := flags.String("repo", "", "source checkout")
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
		return fmt.Errorf("Component/System documentation requires an app Saga (v5)")
	}
	var result requirements.MutationResult
	switch op {
	case "add", "revise":
		if *from == "" {
			return fmt.Errorf("requires --from FILE|-")
		}
		var reader io.Reader = os.Stdin
		if *from != "-" {
			f, e := os.Open(*from)
			if e != nil {
				return e
			}
			defer f.Close()
			reader = f
		}
		data, e := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if e != nil {
			return e
		}
		if len(data) > 1<<20 {
			return fmt.Errorf("definition exceeds one MiB")
		}
		var def requirements.TechnicalDefinition
		if e := decodeStrictInventory(data, &def); e != nil {
			return e
		}
		if _, e := gitdiff.ReadCatalogRange(ctx, firstNonEmpty(*repo, root), manifest.Source.Repository, gitdiff.Range{Head: "HEAD"}, gitdiff.ReadOptions{}); e != nil {
			return e
		}
		resolver, e := coderesolve.New(ctx, firstNonEmpty(*repo, root))
		if e != nil {
			return e
		}
		defer resolver.Close()
		author := func(refs []coderef.Reference) error {
			for i, r := range refs {
				loc, e := resolveLocation(ctx, firstNonEmpty(*repo, root), r.Location().String())
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
				refs[i] = pinned
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
		result, err = requirements.WriteTechnical(root, manifest.ID, kind, *id, *revision, parents, def, op == "add")
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

func decodeStrictInventory(data []byte, value any) error {
	return requirements.DecodeInventoryJSON(data, value)
}

func requireDocumentation(root, sagaID string, pin *saga.DocumentationLink) error {
	if pin == nil {
		return nil
	}
	if !saga.ValidDocumentationLink(sagaID, *pin) {
		return fmt.Errorf("documentation requires a canonical Component/System target and its revision URN")
	}
	d, err := requirements.LoadInventory(root, sagaID)
	if err != nil {
		return err
	}
	if status := d.LinkStatus(*pin); status != "current" {
		return fmt.Errorf("documentation target %s is %s", pin.Target, status)
	}
	return nil
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
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if item.Documentation != nil {
					report(item.Path, *item.Documentation)
				}
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
				if item.Documentation != nil {
					report(item.Path, *item.Documentation)
				}
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
	for _, kind := range []string{"component", "system"} {
		for _, op := range []string{"add", "revise", "set-state"} {
			name := kind + " " + op
			commandUsage[name] = "change-saga " + name + " --id ID [--from FILE|- --revision ID | --event ID --state STATE --reason TEXT] [--parent URN] [--repo PATH] [--json] <saga>"
		}
	}
}
