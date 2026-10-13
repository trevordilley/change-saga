package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func whiteboardRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--whiteboard" || strings.HasPrefix(arg, "--whiteboard=") {
			return true
		}
	}
	return false
}

// Whiteboard manages complete technical-design documents using the shared
// diagram grammar, source resolver and immutable revision store.
func Whiteboard(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("whiteboard", []string{"publish", "get", "describe", "check"}, out)
	}
	return whiteboardCommand(ctx, "whiteboard", args[0], args[1:], out)
}

// diagramWhiteboard retains the original diagram ... --whiteboard spelling.
func diagramWhiteboard(ctx context.Context, op string, args []string, out io.Writer) error {
	return whiteboardCommand(ctx, "diagram", op, args, out)
}

func whiteboardCommand(ctx context.Context, family, op string, args []string, out io.Writer) error {
	if op != "publish" && op != "get" && op != "describe" && op != "check" {
		return fmt.Errorf("whiteboards support publish, get, describe and check; usage: %s", commandUsage["whiteboard"])
	}
	name := family + " " + op
	kindFlag := "kind"
	if family == "diagram" {
		kindFlag = "whiteboard"
	}
	flags := commandFlags(name, commandUsage[name], out)
	kind := flags.String(kindFlag, "", "whiteboard kind (erd)")
	revision := flags.String("revision", "", "exact board revision; reads default to the unique current head")
	focus := flags.String("focus", "", "frame or element instance path")
	id := flags.String("id", "", "element instance path (alias for --focus on get)")
	from := flags.String("from", "", "complete diagram.Document JSON file, or - for stdin")
	parent := flags.String("parent", "", "observed current board revision ID, required on updates")
	title := flags.String("name", "ERD whiteboard", "whiteboard title")
	format := flags.String("format", "text", "text or json (describe)")
	jsonOutput := flags.Bool("json", false, "machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *kind != "erd" {
		return fmt.Errorf("requires --%s erd and one Saga path", kindFlag)
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("--format must be text or json")
	}
	if *focus != "" && *id != "" {
		return fmt.Errorf("use either --focus or --id")
	}
	if *id != "" {
		if op != "get" {
			return fmt.Errorf("--id is supported only for get")
		}
		*focus = *id
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if manifest.Version != 5 {
		return fmt.Errorf("whiteboards require a v5 Saga")
	}
	if op == "publish" {
		if *from == "" || *revision == "" {
			return fmt.Errorf("publishing a whiteboard requires --from and --revision")
		}
		if *focus != "" {
			return fmt.Errorf("publishing replaces the complete board; --focus is read-only")
		}
		data, err := readBoundedInput(*from, requirements.MaxRecordBytes, "whiteboard diagram")
		if err != nil {
			return err
		}
		var doc diagram.Document
		if err := requirements.DecodeInventoryJSON(data, &doc); err != nil {
			return err
		}
		value, err := requirements.PublishWhiteboard(ctx, root, manifest.ID, *kind, *revision, *parent, *title, doc)
		if err != nil {
			return err
		}
		return writeJSON(out, value)
	}
	if *from != "" || *parent != "" {
		return fmt.Errorf("--from and --parent require whiteboard publish")
	}
	if op == "check" && *revision == "" {
		if err := requirements.ValidateWhiteboards(root, manifest.ID); err != nil {
			return err
		}
	}
	value, err := requirements.LoadResolvedWhiteboard(root, manifest.ID, *kind, *revision)
	if err != nil {
		return err
	}
	if value == nil {
		return fmt.Errorf("ERD whiteboard does not exist; publish it with whiteboard publish --kind erd")
	}
	if op == "get" && *focus == "" {
		return writeJSON(out, value.Revision.Document)
	}
	description, err := value.Composition.Describe(*focus)
	if err != nil {
		return err
	}
	if op == "check" {
		if *jsonOutput || *format == "json" {
			return writeJSON(out, map[string]any{"whiteboard": *kind, "revision": value.Revision.ID, "valid": true})
		}
		_, err = fmt.Fprintf(out, "Whiteboard %s revision %s: valid\n", *kind, value.Revision.ID)
		return err
	}
	if op == "get" || *jsonOutput || *format == "json" {
		return writeJSON(out, map[string]any{"whiteboard": *kind, "revision": value.Revision.ID, "name": value.Revision.Name, "diagram": description})
	}
	fmt.Fprintf(out, "Whiteboard %s: %s (revision %s)\n", *kind, value.Revision.Name, value.Revision.ID)
	if description.Focus != "" {
		fmt.Fprintf(out, "Focus: %s\n", description.Focus)
	}
	seen := map[string]bool{}
	for _, element := range description.Elements {
		writeWhiteboardInstance(out, element)
		seen[element.Path] = true
	}
	for _, connection := range description.Connections {
		if !seen[connection.Path] {
			writeWhiteboardInstance(out, connection)
		}
	}
	if len(description.Context) > 0 {
		fmt.Fprintln(out, "Context:")
		for _, element := range description.Context {
			writeWhiteboardInstance(out, element)
		}
	}
	return nil
}

func writeWhiteboardInstance(out io.Writer, instance diagram.Instance) {
	element := instance.Element
	fmt.Fprintf(out, "  %s [%s]", instance.Path, element.Kind)
	if element.Label != "" {
		fmt.Fprintf(out, " %q", element.Label)
	}
	if instance.Opaque {
		fmt.Fprint(out, " (SVG; structured detail unavailable)")
	}
	if instance.From != "" || instance.To != "" {
		fmt.Fprintf(out, " %s -> %s", instance.From, instance.To)
	}
	if instance.About != "" {
		fmt.Fprintf(out, " about=%s", instance.About)
	}
	fmt.Fprintln(out)
	for _, text := range []string{element.Summary, element.Description, element.Detail, element.Note} {
		if text != "" {
			fmt.Fprintf(out, "    %s\n", text)
		}
	}
	if source := instance.Source; source != nil {
		fmt.Fprintf(out, "    source: %s @ %s", source.Target, source.Revision)
		if source.Element != "" {
			fmt.Fprintf(out, " element=%s", source.Element)
		}
		if source.Relationship != "" {
			fmt.Fprintf(out, " relationship=%s", source.Relationship)
		}
		fmt.Fprintln(out)
	}
}

func appendWhiteboardIssues(root string, document *saga.Saga, validation *saga.Validation) {
	if document == nil || document.Manifest.Version != 5 {
		return
	}
	if err := requirements.ValidateWhiteboards(root, document.Manifest.ID); err != nil {
		appendLoadIssues(validation, requirements.WhiteboardDir, err)
	}
}
