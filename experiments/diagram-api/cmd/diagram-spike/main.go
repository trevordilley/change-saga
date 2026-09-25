// diagram-spike is an isolated experiment. It is not registered in change-saga.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/experiments/diagram-api/assets"
	"github.com/twentyideas/changesaga/experiments/diagram-api/draft"
)

func emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		die(err)
	}
	fmt.Println(string(b))
}
func die(err error) {
	v := map[string]any{"ok": false, "error": err.Error()}
	if p := draft.PublicationState(err); p != nil {
		v["publication"] = p
	}
	b, _ := json.Marshal(v)
	fmt.Fprintln(os.Stderr, string(b))
	os.Exit(1)
}
func read(path string) []byte {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(io.LimitReader(os.Stdin, 16<<20))
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		die(err)
	}
	return b
}
func jsonFile[T any](path string) T {
	var v T
	if err := draft.Decode(read(path), &v); err != nil {
		die(err)
	}
	return v
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		die(err)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use schema, init, apply, add, update, move, remove, describe, get, source, render, check, rebuild, merge, or assets")
	}
	cmd, args := args[0], args[1:]
	if cmd == "schema" {
		emit(map[string]any{"version": 1, "experimental": true, "commands": map[string]string{
			"init":          "--store DIR --id ID --title TEXT --request ID",
			"apply":         "--store DIR --from FILE|- [--dry-run]; request={version:1,request_id,expected_snapshot,operations:[...]}",
			"add":           "--store DIR --expected HASH --request ID --from element.json|-",
			"update":        "--store DIR --expected HASH --request ID --id ID --set JSON",
			"move":          "--store DIR --expected HASH --request ID --id ID --dx N --dy N (edges unchanged)",
			"remove":        "--store DIR --expected HASH --request ID --id ID [--cascade]; linked Items refused",
			"describe":      "--store DIR [--offset N --limit N] [--format text|json]; compact semantic text by default, non-reconstructable",
			"get":           "--store DIR --id ID; complete targeted element and snapshot",
			"source":        "--store DIR; complete source including asset pins",
			"render":        "--store DIR > drawing.svg; checks hash and selectors",
			"check/rebuild": "--store DIR; rebuild preserves divergent SVG under recovered/",
			"merge":         "--base FILE --ours FILE --theirs FILE; pure three-way source merge",
			"assets":        "[search TEXT]; bounded local curated icon names",
		}, "element": map[string]any{"required": []string{"id", "kind", "style"}, "kind": []string{"node", "edge", "text", "group", "graphic"}, "node_shapes": []string{"service", "datastore", "decision", "rect", "ellipse", "boundary"}, "properties": []string{"label", "detail", "description", "x", "y", "width", "height", "z", "parent", "icon", "icon_size", "from", "to", "points:[{x,y}]", "path", "head:arrow|none", "head_size", "label_at:{x,y}", "wrap", "fragment", "decorative", "evidence", "criterion_links"}}, "operations": []string{"style:id,style (complete reusable style)", "add:element", "update:id,set (identity/evidence immutable)", "move:id,dx,dy", "remove:id,cascade", "align:ids,axis,value", "distribute:ids,axis,value (gap, supplied order)"}, "rules": []string{"all coordinates explicit", "edge geometry independent of connections", "updates never move neighboring objects", "pinned font; overflow errors, no auto-resizing", "source+SVG hash published atomically", "SVG fragment subset, no arbitrary import"}})
		return nil
	}
	if cmd == "assets" {
		q := ""
		if len(args) > 0 && args[0] == "search" {
			args = args[1:]
		}
		if len(args) > 0 {
			q = args[0]
		}
		emit(map[string]any{"pack": "lucide@" + assets.LucideRevision, "names": assets.Names(q), "total_pack": 10, "runtime_network": false})
		return nil
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("store", "", "isolated diagram directory")
	id := fs.String("id", "", "stable ID")
	title := fs.String("title", "", "title")
	from := fs.String("from", "-", "JSON file or stdin")
	expected := fs.String("expected", "", "snapshot")
	request := fs.String("request", "", "request ID")
	set := fs.String("set", "{}", "patch JSON")
	dx := fs.Float64("dx", 0, "delta x")
	dy := fs.Float64("dy", 0, "delta y")
	cascade := fs.Bool("cascade", false, "explicit dependencies")
	dry := fs.Bool("dry-run", false, "validate only")
	offset := fs.Int("offset", 0, "offset")
	limit := fs.Int("limit", 30, "limit")
	format := fs.String("format", "text", "describe output: text or json")
	base := fs.String("base", "", "base source")
	ours := fs.String("ours", "", "ours source")
	theirs := fs.String("theirs", "", "theirs source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if cmd == "merge" {
		d, conflicts, err := draft.Merge(jsonFile[draft.Document](*base), jsonFile[draft.Document](*ours), jsonFile[draft.Document](*theirs))
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			emit(map[string]any{"ok": false, "conflicts": conflicts})
			return fmt.Errorf("merge conflict")
		}
		emit(d)
		return nil
	}
	if *root == "" {
		return fmt.Errorf("--store required")
	}
	s := draft.Store{Root: *root}
	switch cmd {
	case "init", "apply", "add", "update", "move", "remove":
		req := draft.Request{Version: 1, RequestID: *request, Expected: *expected}
		switch cmd {
		case "init":
			d := draft.New(*id, *title)
			req.Source = &d
			req.Expected = "absent"
		case "apply":
			req = jsonFile[draft.Request](*from)
		case "add":
			e := jsonFile[draft.Element](*from)
			req.Operations = []draft.Operation{{Op: "add", Element: &e}}
		case "update":
			req.Operations = []draft.Operation{{Op: "update", ID: *id, Set: json.RawMessage(*set)}}
		case "move":
			req.Operations = []draft.Operation{{Op: "move", ID: *id, DX: *dx, DY: *dy}}
		case "remove":
			req.Operations = []draft.Operation{{Op: "remove", ID: *id, Cascade: *cascade}}
		}
		res, err := s.Apply(req, *dry)
		if err != nil {
			return err
		}
		emit(res)
	case "check":
		v, err := s.Check()
		if err != nil {
			return err
		}
		emit(v)
		if v["visual_ok"] != true || v["rebuildable"] != true {
			return fmt.Errorf("integrity check failed")
		}
	case "rebuild":
		v, err := s.Rebuild()
		if err != nil {
			return err
		}
		emit(v)
	case "describe", "get", "source", "render":
		r, err := s.Load()
		if err != nil {
			return err
		}
		switch cmd {
		case "describe":
			if *offset < 0 || *limit < 1 || *limit > 100 {
				return fmt.Errorf("offset >=0 and limit 1..100 required")
			}
			v := draft.Describe(r.Source, *offset, *limit)
			switch *format {
			case "text":
				fmt.Print(v.Text())
			case "json":
				emit(v)
			default:
				return fmt.Errorf("--format must be text or json")
			}
		case "get":
			e, ok := r.Source.Elements[*id]
			if !ok {
				return fmt.Errorf("unknown element %q", *id)
			}
			emit(map[string]any{"snapshot": r.Snapshot, "element": e, "selector": "#" + e.ID})
		case "source":
			emit(r.Source)
		case "render":
			b, err := s.Visual(r)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(b)
			return err
		}
	default:
		return fmt.Errorf("unknown command %s", strconv.Quote(strings.TrimSpace(cmd)))
	}
	return nil
}
