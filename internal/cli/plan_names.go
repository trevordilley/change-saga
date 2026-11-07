package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// nameLinks reads --link TO[=LABEL] values.
func nameLinks(values []string) ([]saga.NameLink, error) {
	links := make([]saga.NameLink, 0, len(values))
	for _, value := range values {
		to, label, _ := strings.Cut(value, "=")
		to, label = strings.TrimSpace(to), strings.TrimSpace(label)
		if to == "" {
			return nil, fmt.Errorf("--link must be NAME or NAME=LABEL")
		}
		links = append(links, saga.NameLink{To: to, Label: label})
	}
	return links, nil
}

// requireNameRecord refuses a pointer to a Term, Component, or System that does
// not exist.
func requireNameRecord(root, record string) error {
	if record == "" {
		return nil
	}
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if id, err := requirements.ParseTermURN(sagaManifest.ID, record); err == nil {
		document, err := requirements.Load(root, sagaManifest.ID)
		if err != nil {
			return err
		}
		if document.FindTerm(id) == nil {
			return fmt.Errorf("term %q does not exist: %w", record, errRecordMissing)
		}
		return nil
	}
	prefix := "urn:change-saga:" + sagaManifest.ID + ":"
	if strings.HasPrefix(record, prefix+"component:") || strings.HasPrefix(record, prefix+"system:") {
		inventory, err := requirements.LoadInventory(root, sagaManifest.ID)
		if err != nil {
			return err
		}
		if inventory.Find(record) == nil {
			return fmt.Errorf("%q does not exist: %w", record, errRecordMissing)
		}
		return nil
	}
	return fmt.Errorf("--record must be a term, component, or system URN of this Saga: urn:change-saga:%s:term:<id>", sagaManifest.ID)
}

func planAddName(_ context.Context, args []string, out io.Writer) error {
	name := "plan add-name"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "stable name id")
	title := flags.String("title", "", "the name")
	summary := flags.String("summary", "", "what the name hides")
	parent := flags.String("parent", "", "the name this one details; omitted for a top-level name")
	record := flags.String("record", "", "URN of a term, component, or system the name means")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var links stringList
	flags.Var(&links, "link", "NAME[=LABEL] this name links to; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id, *title); err != nil {
		return err
	}
	root := flags.Arg(0)
	if err := requireNameRecord(root, *record); err != nil {
		return err
	}
	value := saga.NameManifest{ID: *id, Title: strings.TrimSpace(*title), Summary: strings.TrimSpace(*summary), Parent: *parent, Record: *record}
	var err error
	if value.Links, err = nameLinks(links); err != nil {
		return err
	}
	urn, err := planstore.AddName(root, planIDOf(*plan), value)
	if err != nil {
		return err
	}
	path := saga.PlansDir + "/" + planIDOf(*plan) + saga.PlanSuffix + "/" + saga.PlanNamesDir + "/" + *id + saga.NameSuffix
	return writeLivingMutation(out, name, urn, path, []string{urn}, nil, false, *jsonOutput)
}

func planSetName(_ context.Context, args []string, out io.Writer) error {
	name := "plan set-name"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "name id")
	title := flags.String("title", "", "the name")
	summary := flags.String("summary", "", "what the name hides; empty clears it")
	parent := flags.String("parent", "", "the name this one details; empty makes it top-level")
	record := flags.String("record", "", "URN of a term, component, or system; empty clears it")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var links stringList
	flags.Var(&links, "link", "NAME[=LABEL]; repeatable; replaces the name's links, and one empty value clears them")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id); err != nil {
		return err
	}
	given := map[string]bool{}
	flags.Visit(func(value *flag.Flag) { given[value.Name] = true })
	root := flags.Arg(0)
	if given["record"] {
		if err := requireNameRecord(root, *record); err != nil {
			return err
		}
	}
	var replacement []saga.NameLink
	if given["link"] && !(len(links) == 1 && strings.TrimSpace(links[0]) == "") {
		var err error
		if replacement, err = nameLinks(links); err != nil {
			return err
		}
	}
	err := planstore.UpdateName(root, planIDOf(*plan), *id, func(value *saga.NameManifest) error {
		if given["title"] {
			value.Title = strings.TrimSpace(*title)
		}
		if given["summary"] {
			value.Summary = strings.TrimSpace(*summary)
		}
		if given["parent"] {
			value.Parent = *parent
		}
		if given["record"] {
			value.Record = *record
		}
		if given["link"] {
			value.Links = replacement
		}
		return nil
	})
	if err != nil {
		return err
	}
	return writeNameChange(out, name, root, planIDOf(*plan), *id, *jsonOutput)
}

func planRemoveName(_ context.Context, args []string, out io.Writer) error {
	name := "plan remove-name"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "name id")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id); err != nil {
		return err
	}
	if err := planstore.RemoveName(flags.Arg(0), planIDOf(*plan), *id); err != nil {
		return err
	}
	return writeNameChange(out, name, flags.Arg(0), planIDOf(*plan), *id, *jsonOutput)
}

func writeNameChange(out io.Writer, operation, root, planID, nameID string, jsonOutput bool) error {
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.NameTarget(sagaManifest.ID, planID, nameID)
	if jsonOutput {
		path := saga.PlansDir + "/" + planID + saga.PlanSuffix + "/" + saga.PlanNamesDir + "/" + nameID + saga.NameSuffix
		return writeLivingMutation(out, operation, urn, path, []string{urn}, nil, false, true)
	}
	fmt.Fprintf(out, "Updated %s\n", urn)
	return nil
}

// planNameRow is one name as plan names --json returns it: the record, then
// what its Tasks say about it.
type planNameRow struct {
	saga.NameManifest
	Target   string   `json:"target"`
	State    string   `json:"state"`
	Done     int      `json:"done"`
	All      int      `json:"all"`
	Depth    int      `json:"depth"`
	Children []string `json:"children,omitempty"`
	Tasks    []string `json:"tasks,omitempty"`
}

// planNamesResult is what plan names --json returns.
type planNamesResult struct {
	Plan      string        `json:"plan"`
	Roots     []string      `json:"roots"`
	Names     []planNameRow `json:"names"`
	Unnamed   []string      `json:"unnamed"`
	Unreached []string      `json:"unreached"`
}

func namesResult(plan *saga.Plan) planNamesResult {
	result := planNamesResult{Plan: plan.Target, Roots: []string{}, Names: []planNameRow{}, Unnamed: []string{}, Unreached: []string{}}
	view := saga.PlanNames(plan.NameManifests(), plan.Manifests())
	if view == nil {
		return result
	}
	result.Roots = view.Roots
	for _, name := range plan.Names {
		progress := view.Progress[name.ID]
		result.Names = append(result.Names, planNameRow{NameManifest: name.NameManifest, Target: name.Target, State: progress.State,
			Done: progress.Done, All: progress.All, Depth: progress.Depth, Children: progress.Children, Tasks: progress.Tasks})
	}
	result.Unnamed = append(result.Unnamed, view.Unnamed...)
	result.Unreached = append(result.Unreached, view.Unreached...)
	return result
}

var nameStateMarks = map[string]string{saga.NameDone: "✓", saga.NameUnderWay: "◐", saga.NamePlanned: "○", saga.NameUnreached: "·"}

func planNames(_ context.Context, args []string, out io.Writer) error {
	name := "plan names"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	jsonOutput := flags.Bool("json", false, "emit the names, their progress, and the gaps as JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan); err != nil {
		return err
	}
	_, found, err := planOf(flags.Arg(0), *plan)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, namesResult(found))
	}
	view := saga.PlanNames(found.NameManifests(), found.Manifests())
	if view == nil {
		fmt.Fprintf(out, "Plan %s has no names yet. Add a few top-level names with plan add-name, then the names beneath them with --parent.\n", found.ID)
		return nil
	}
	var print func(id string, depth int)
	print = func(id string, depth int) {
		progress, record := view.Progress[id], found.FindName(id)
		fmt.Fprintf(out, "%s%s %s (%s)  %d/%d done\n", strings.Repeat("  ", depth), nameStateMarks[progress.State], record.Title, id, progress.Done, progress.All)
		for _, child := range progress.Children {
			print(child, depth+1)
		}
	}
	for _, root := range view.Roots {
		print(root, 0)
	}
	writeNameGaps(out, found, view)
	return nil
}

// writeNameGaps lists the Tasks on no name and the names no Task reaches.
func writeNameGaps(out io.Writer, plan *saga.Plan, view *saga.NamesView) {
	for _, id := range view.Unnamed {
		fmt.Fprintf(out, "  Unexpected work, on no name: %s (%s). Name what it works on with task set --name, or remove it.\n", id, plan.FindTask(id).Title)
	}
	for _, id := range view.Unreached {
		path := []string{}
		for _, ancestor := range view.Ancestry(plan.NameManifests(), id) {
			path = append(path, plan.FindName(ancestor).Title)
		}
		fmt.Fprintf(out, "  Possibly unneeded, no task reaches it: %s (%s). Add a Task that names it, or remove it.\n", strings.Join(path, " › "), id)
	}
}
