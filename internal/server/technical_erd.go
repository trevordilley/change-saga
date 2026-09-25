package server

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The data model: canonical data entities, the relationships they own, and
// authored ERD views over them.
//
// An ERD is an author's picture, not a generated schema. The renderer inlines
// the ERD revision's own offline SVG exactly as authored, and makes only the
// elements its bindings name selectable. It never reads the drawing's
// geometry to understand the model: the directory, relationships, meaning and
// cardinality all come from the records. A binding and the directory row for
// the same entity open the same pinned revision.
//
// Relationships keep their two meanings apart. An association is drawn with
// crow's-foot endpoints and states each endpoint's declared cardinality in
// text, including an explicit "unknown". A production is a labelled
// directional flow; it has no cardinality and is not a foreign key.

// ----- One data entity -----

type entityFieldView struct {
	Name, Type, Note string
	Keys             []string
}

type entityHolderView struct {
	Name, Role, Href, Status string
	Pin                      saga.DocumentationLink
}

// relationshipView is one relationship as a reader sees it from either end.
type relationshipView struct {
	ID, Meaning, Label, Explanation, Intent string
	// Direction is "outgoing" for a relationship the shown revision owns and
	// "incoming" for one another entity's current revision declares toward it.
	Direction                   string
	OwnerName, OwnerHref        string
	OwnerPin                    saga.DocumentationLink
	DestinationName             string
	DestinationHref             string
	DestinationStatus           string
	OwnerCardinality            string
	DestinationCardinality      string
	Flow                        string
	Notation                    string
	Glyph                       template.HTML
	Evidence                    int
	OwnerStatus, OwnerIntentRef string
}

// entityName names a pinned revision, or its target while it is missing.
func entityName(inventory requirements.Inventory, pin saga.DocumentationLink) string {
	if revision := inventory.Pinned(pin); revision != nil {
		return revision.Name
	}
	return pin.Target
}

// makeRelationshipView reads one owned relationship. owner is the pin of the
// revision that declares it.
func makeRelationshipView(inventory requirements.Inventory, owner saga.DocumentationLink, edge requirements.Relationship, direction string) relationshipView {
	view := relationshipView{
		ID: edge.ID, Meaning: edge.Meaning, Label: edge.Label, Explanation: edge.Explanation, Intent: edge.Intent, Direction: direction,
		OwnerPin: owner, OwnerName: entityName(inventory, owner), OwnerHref: technicalPinHref(owner), OwnerStatus: inventory.LinkStatus(owner),
		DestinationName: entityName(inventory, edge.Destination), DestinationHref: technicalPinHref(edge.Destination), DestinationStatus: inventory.LinkStatus(edge.Destination),
		Flow: edge.Flow, Evidence: len(edge.Code),
	}
	if view.Intent == "" {
		view.Intent = requirements.IntentUnspecified
	}
	switch edge.Meaning {
	case "association":
		if edge.Cardinality != nil {
			view.OwnerCardinality, view.DestinationCardinality = edge.Cardinality.Owner, edge.Cardinality.Destination
		}
		view.Notation = fmt.Sprintf("%s (%s) — (%s) %s", view.OwnerName, view.OwnerCardinality, view.DestinationCardinality, view.DestinationName)
	case "production":
		from, to := view.OwnerName, view.DestinationName
		if edge.Flow == "destination_to_owner" {
			from, to = to, from
		}
		view.Notation = fmt.Sprintf("%s → %s → %s", from, edge.Label, to)
	}
	view.Glyph = relationshipGlyph(edge)
	return view
}

// relationshipGlyph draws the declared notation for one relationship: crow's
// foot endpoints for an association, a dashed arrow for a production. It is a
// legend for the text beside it, drawn from the record alone.
func relationshipGlyph(edge requirements.Relationship) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg class="relationship-glyph" viewBox="0 0 120 24" width="120" height="24" aria-hidden="true" focusable="false">`)
	switch edge.Meaning {
	case "association":
		b.WriteString(`<line x1="4" y1="12" x2="116" y2="12" stroke="currentColor" stroke-width="1.5"/>`)
		owner, destination := "unknown", "unknown"
		if edge.Cardinality != nil {
			owner, destination = edge.Cardinality.Owner, edge.Cardinality.Destination
		}
		b.WriteString(crowsFoot(owner, 4, 1))
		b.WriteString(crowsFoot(destination, 116, -1))
	case "production":
		x1, x2 := 4, 110
		if edge.Flow == "destination_to_owner" {
			x1, x2 = 116, 10
		}
		fmt.Fprintf(&b, `<line x1="%d" y1="12" x2="%d" y2="12" stroke="currentColor" stroke-width="1.5" stroke-dasharray="5 3"/>`, x1, x2)
		if x2 > x1 {
			b.WriteString(`<path d="M 108 6 L 118 12 L 108 18 z" fill="currentColor"/>`)
		} else {
			b.WriteString(`<path d="M 12 6 L 2 12 L 12 18 z" fill="currentColor"/>`)
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// crowsFoot draws one association endpoint at x, facing into the line along
// dir: a bar for one, a circle for zero, a foot for many, "?" for unknown.
func crowsFoot(cardinality string, x, dir int) string {
	at := func(offset int) int { return x + dir*offset }
	bar := func(offset int) string {
		return fmt.Sprintf(`<line x1="%d" y1="5" x2="%d" y2="19" stroke="currentColor" stroke-width="1.5"/>`, at(offset), at(offset))
	}
	circle := func(offset int) string {
		return fmt.Sprintf(`<circle cx="%d" cy="12" r="4" fill="var(--bg,white)" stroke="currentColor" stroke-width="1.5"/>`, at(offset))
	}
	foot := fmt.Sprintf(`<path d="M %d 5 L %d 12 L %d 19" fill="none" stroke="currentColor" stroke-width="1.5"/>`, at(0), at(10), at(0))
	switch cardinality {
	case "1":
		return bar(6) + bar(10)
	case "0..1":
		return bar(6) + circle(16)
	case "1..many":
		return foot + bar(14)
	case "0..many":
		return foot + circle(18)
	}
	return fmt.Sprintf(`<text x="%d" y="16" font-size="12" text-anchor="middle" fill="currentColor">?</text>`, at(8))
}

// dataEntityDetail fills the entity-specific parts of a definition view:
// curated fields, holding resources, owned and incoming relationships, and
// the ERDs whose directory names it.
func dataEntityDetail(inventory requirements.Inventory, view *documentationView, revision *requirements.TechnicalRevision, pin saga.DocumentationLink) {
	for _, field := range revision.Fields {
		view.Fields = append(view.Fields, entityFieldView{Name: field.Name, Type: field.Type, Keys: field.Keys, Note: field.Note})
	}
	for _, holder := range revision.Holders {
		view.Holders = append(view.Holders, entityHolderView{Name: entityName(inventory, holder.Component), Role: holder.Role, Href: technicalPinHref(holder.Component), Status: inventory.LinkStatus(holder.Component), Pin: holder.Component})
	}
	for _, edge := range revision.Relationships {
		view.Relationships = append(view.Relationships, makeRelationshipView(inventory, pin, edge, "outgoing"))
	}
	// Incoming relationships are derived, never stored twice: read them from
	// every other entity's current revision.
	for index := range inventory.Records {
		record := &inventory.Records[index]
		if record.Kind != requirements.KindDataEntity || record.CurrentRevision == nil || record.Target == pin.Target {
			continue
		}
		owner := saga.DocumentationLink{Target: record.Target, Revision: record.Target + ":revision:" + record.CurrentRevision.ID}
		for _, edge := range record.CurrentRevision.Relationships {
			if edge.Destination.Target == pin.Target {
				incoming := makeRelationshipView(inventory, owner, edge, "incoming")
				if edge.Destination.Revision != pin.Revision {
					incoming.OwnerIntentRef = strings.TrimPrefix(edge.Destination.Revision, pin.Target+":revision:")
				}
				view.Incoming = append(view.Incoming, incoming)
			}
		}
	}
	for index := range inventory.Records {
		record := &inventory.Records[index]
		if record.Kind != requirements.KindERD || record.CurrentRevision == nil {
			continue
		}
		for _, member := range record.CurrentRevision.Directory {
			if member.Target == pin.Target {
				view.ERDs = append(view.ERDs, erdMembershipView{Name: record.CurrentRevision.Name, Href: technicalHref(record.Kind, record.Identity.ID, ""), Pinned: member.Revision == pin.Revision, Revision: strings.TrimPrefix(member.Revision, pin.Target+":revision:")})
			}
		}
	}
}

type erdMembershipView struct {
	Name, Href, Revision string
	Pinned               bool
}

// ----- An ERD view -----

type erdBindingView struct {
	ID, Element, Label  string
	Pin                 saga.DocumentationLink
	Relationship        string
	Intent, Status      string
	DirectoryRow        string
	RelationshipMeaning string
}

type erdDirectoryRow struct {
	Name, Href, Status, Intent, Explanation string
	Pin                                     saga.DocumentationLink
	// Elements are the diagram elements bound to this pin; none means the
	// entity is in the directory but not drawn.
	Elements []string
	Holders  []string
	// Change is an overlay's effect on this row: "replaces", "adds" or
	// "removes"; empty for a baseline row.
	Change, ChangeNote string
	Missing            bool
}

type erdView struct {
	Kind, ID, Target, Name, Explanation, Revision string
	Visual                                        template.HTML
	VisualNote                                    string
	Bindings                                      []erdBindingView
	Directory                                     []erdDirectoryRow
	Relationships                                 []relationshipView
	Drawn, Omitted                                int
	// Overlay fields: the named baseline ERD revision and feature scope.
	BaselineName, BaselineHref, Feature, FeatureHref string
	Removals                                         []requirements.Removal
}

func (view *erdView) DOMID() string { return "erd-" + domID(view.Target) }

// elementPrefix namespaces the drawing's ids within the page.
func (view *erdView) elementPrefix() string { return view.DOMID() + "-" }

// makeERDView renders one ERD or overlay revision. Overlays compose their
// directory over the exact baseline ERD revision they name; the canonical ERD
// is read, never rewritten.
func (a *app) makeERDView(inventory requirements.Inventory, record *requirements.TechnicalRecord, revision *requirements.TechnicalRevision) *erdView {
	view := &erdView{Kind: record.Kind, ID: record.Identity.ID, Target: record.Target, Name: revision.Name, Explanation: revision.Explanation, Revision: revision.ID}
	directory := revision.Directory
	visualRecord, visual, bindings := record, revision.Visual, revision.Bindings
	baselinePins := map[string]saga.DocumentationLink{}
	if record.Kind == requirements.KindERDOverlay && revision.ERD != nil {
		view.BaselineName, view.BaselineHref = entityName(inventory, *revision.ERD), technicalPinHref(*revision.ERD)
		if revision.Feature != "" {
			parts := strings.Split(revision.Feature, ":")
			view.Feature, view.FeatureHref = parts[len(parts)-1], featureHref(parts[len(parts)-1])
		}
		view.Removals = revision.Removals
		composed, err := inventory.ComposeOverlay(revision)
		if err != nil {
			view.VisualNote = "The overlay could not be composed with its baseline: " + err.Error()
		}
		directory = composed
		if baseline := inventory.Pinned(*revision.ERD); baseline != nil {
			for _, pin := range baseline.Directory {
				baselinePins[pin.Target] = pin
			}
			if visual == nil {
				// No overlay drawing: show the baseline's, and say so.
				visualRecord, visual, bindings = inventory.Find(revision.ERD.Target), baseline.Visual, baseline.Bindings
				view.VisualNote = "This overlay has no drawing of its own. The diagram is the baseline ERD revision; the overlay's changes are listed in the directory."
			}
		}
	}
	if visual != nil && visualRecord != nil {
		path := filepath.Join(a.root, filepath.FromSlash(requirements.TechnicalPath(visualRecord.Kind, visualRecord.Identity.ID)), filepath.FromSlash(visual.Path))
		data, err := os.ReadFile(path)
		if err == nil {
			err = requirements.ValidateVisual(data, visual.Digest, bindings)
		}
		if err != nil {
			view.VisualNote = "The authored diagram is unavailable: " + err.Error() + ". The directory below still lists every entity."
		} else if markup, refused := sanitizeSVG(data, view.elementPrefix()); len(refused) > 0 {
			view.VisualNote = "The authored diagram is unavailable: this reviewer draws only static shapes, text and same-document links, and the drawing contains " + strings.Join(refused, "; ") + ". The directory below still lists every entity."
		} else {
			// Re-serialized from its tokens, never the author's bytes.
			view.Visual = template.HTML(markup)
		}
	}
	elements := map[string][]string{}
	for _, binding := range bindings {
		b := erdBindingView{ID: binding.ID, Element: view.elementPrefix() + binding.Element}
		switch {
		case binding.Entity != nil:
			b.Pin = *binding.Entity
			b.Label = entityName(inventory, b.Pin)
			if pinned := inventory.Pinned(b.Pin); pinned != nil {
				b.Intent = pinned.EffectiveIntent()
			}
			elements[b.Pin.Revision] = append(elements[b.Pin.Revision], binding.Element)
		case binding.Relationship != nil:
			b.Pin, b.Relationship = binding.Relationship.Owner, binding.Relationship.ID
			b.Label = entityName(inventory, b.Pin) + " relationship " + b.Relationship
			if owner := inventory.Pinned(b.Pin); owner != nil {
				for _, edge := range owner.Relationships {
					if edge.ID == b.Relationship {
						rel := makeRelationshipView(inventory, b.Pin, edge, "outgoing")
						b.Label, b.Intent, b.RelationshipMeaning = rel.Notation, rel.Intent, edge.Meaning
					}
				}
			}
		}
		b.Status = inventory.LinkStatus(b.Pin)
		view.Bindings = append(view.Bindings, b)
	}
	removed := map[string]string{}
	for _, removal := range view.Removals {
		removed[removal.Target] = removal.Explanation
	}
	for _, pin := range directory {
		row := erdDirectoryRow{Pin: pin, Name: entityName(inventory, pin), Href: technicalPinHref(pin), Status: inventory.LinkStatus(pin), Elements: elements[pin.Revision]}
		pinned := inventory.Pinned(pin)
		if pinned == nil {
			row.Missing, row.Intent = true, "unknown"
		} else {
			row.Intent, row.Explanation = pinned.EffectiveIntent(), summarise(pinned.Explanation, 140)
			for _, holder := range pinned.Holders {
				row.Holders = append(row.Holders, entityName(inventory, holder.Component)+" ("+holder.Role+")")
			}
			for _, edge := range pinned.Relationships {
				view.Relationships = append(view.Relationships, makeRelationshipView(inventory, pin, edge, "outgoing"))
			}
		}
		if record.Kind == requirements.KindERDOverlay {
			if base, ok := baselinePins[pin.Target]; !ok {
				row.Change = "adds"
			} else if base.Revision != pin.Revision {
				row.Change, row.ChangeNote = "replaces", strings.TrimPrefix(base.Revision, pin.Target+":revision:")
				if len(row.Elements) == 0 {
					// The baseline drawing binds the baseline pin; the overlay's
					// replacement is not what is drawn.
					row.Elements = elements[base.Revision]
					if len(row.Elements) > 0 {
						row.ChangeNote += "; the diagram shows the baseline revision"
					}
				}
			}
		}
		if len(row.Elements) > 0 {
			view.Drawn++
		} else {
			view.Omitted++
		}
		view.Directory = append(view.Directory, row)
	}
	for _, removal := range view.Removals {
		row := erdDirectoryRow{Pin: saga.DocumentationLink{Target: removal.Target}, Change: "removes", ChangeNote: removal.Explanation, Status: "removed by overlay"}
		row.Name = removal.Target
		if base, ok := baselinePins[removal.Target]; ok {
			row.Pin, row.Name, row.Href = base, entityName(inventory, base), technicalPinHref(base)
			row.Elements = elements[base.Revision]
		}
		view.Directory = append(view.Directory, row)
	}
	return view
}

// ----- The ERD page's data model -----

type technicalDataModelView struct {
	// Primary is the application's ERD, when one is authored: the one named
	// "application", else the first.
	Primary  *erdView
	Views    []traceLink
	Entities *directoryView
}

func (a *app) technicalDataModel(inventory requirements.Inventory, usages technicalUsageIndex, newness *technicalNewness, query string) *technicalDataModelView {
	view := &technicalDataModelView{Entities: &directoryView{
		ID: "technical-entities", Title: "Data entities", Action: technicalPath + "/erd",
		Label: "Filter data entities", Noun: "data entity", Nouns: "data entities",
		Columns: []directoryColumn{{Title: "Data entity"}, {Title: "Purpose", Wide: true}, {Title: "Intent"}, {Title: "Held by"}, {Title: "Relationships", Numeric: true}, {Title: "Used by Items", Numeric: true}},
		Empty:   "No data entities yet.",
	}}
	var primary *requirements.TechnicalRecord
	for index := range inventory.Records {
		record := &inventory.Records[index]
		switch record.Kind {
		case requirements.KindERD, requirements.KindERDOverlay:
			link := traceLink{Kind: "ERD", Href: technicalHref(record.Kind, record.Identity.ID, ""), Target: record.Target, Title: record.Identity.ID}
			if record.Kind == requirements.KindERDOverlay {
				link.Kind = "ERD overlay"
			}
			if record.CurrentRevision != nil {
				link.Title = record.CurrentRevision.Name
				if record.Kind == requirements.KindERDOverlay && record.CurrentRevision.Feature != "" {
					link.Note = "proposed changes for " + record.CurrentRevision.Feature
				}
			} else {
				link.Note = "competing revisions"
			}
			view.Views = append(view.Views, link)
			if record.Kind == requirements.KindERD && record.CurrentRevision != nil && (primary == nil || record.Identity.ID == "application") {
				if primary == nil || primary.Identity.ID != "application" {
					primary = record
				}
			}
		case requirements.KindDataEntity:
			name, purpose, intent, holders, relationships := record.Identity.ID, "", gapCell("unknown — competing revisions"), []string{}, 0
			if current := record.CurrentRevision; current != nil {
				name, purpose, relationships = current.Name, current.Explanation, len(current.Relationships)
				intent = textCell(current.EffectiveIntent())
				if intent.Text == requirements.IntentUnspecified {
					intent = gapCell(intent.Text)
				}
				for _, holder := range current.Holders {
					holders = append(holders, entityName(inventory, holder.Component))
				}
			}
			items, _ := usages.itemCount(record.Target)
			cells := []directoryCell{
				{Text: name, Href: technicalHref(record.Kind, record.Identity.ID, ""), Note: record.Identity.ID, Target: record.Target},
				textCell(summarise(purpose, 140)), intent, listCell(holders, "no holding resource named"), countCell(relationships), countCell(items),
			}
			if newness != nil {
				cells = append(cells, newnessCell(newness, record))
			}
			view.Entities.addRow(directoryRow{Key: record.Identity.ID, Cells: cells})
		}
	}
	if newness != nil {
		view.Entities.Columns = append(view.Entities.Columns, directoryColumn{Title: "Since " + newness.Against})
	}
	if primary != nil {
		view.Primary = a.makeERDView(inventory, primary, primary.CurrentRevision)
	}
	view.Entities.apply(query)
	return view
}

const technicalERDTemplates = `
{{define "relationship-row"}}<li class="relationship relationship-{{.Meaning}}" data-relationship="{{.ID}}" data-relationship-meaning="{{.Meaning}}" data-relationship-intent="{{.Intent}}">{{.Glyph}}<span class="relationship-notation">{{if eq .Meaning "association"}}<a href="{{.OwnerHref}}">{{.OwnerName}}</a> <span class="cardinality" data-cardinality-owner>{{.OwnerCardinality}}</span> — <span class="cardinality" data-cardinality-destination>{{.DestinationCardinality}}</span> <a href="{{.DestinationHref}}">{{.DestinationName}}</a>{{else}}{{if eq .Flow "destination_to_owner"}}<a href="{{.DestinationHref}}">{{.DestinationName}}</a> → <em>{{.Label}}</em> → <a href="{{.OwnerHref}}">{{.OwnerName}}</a>{{else}}<a href="{{.OwnerHref}}">{{.OwnerName}}</a> → <em>{{.Label}}</em> → <a href="{{.DestinationHref}}">{{.DestinationName}}</a>{{end}}{{end}}</span> <small class="technical-intent intent-{{.Intent}}">{{.Intent}}</small> <small class="trace-kind">{{if eq .Meaning "association"}}association{{else}}production, not a foreign key{{end}}</small>{{if ne .DestinationStatus "current"}} <small class="gap">destination pin {{.DestinationStatus}}</small>{{end}}{{if .OwnerIntentRef}} <small class="gap">declared against revision {{.OwnerIntentRef}}</small>{{end}}<p class="trace-rationale">{{.Explanation}}{{if eq .Meaning "association"}}{{if or (eq .OwnerCardinality "unknown") (eq .DestinationCardinality "unknown")}} Cardinality is declared unknown, not guessed.{{end}}{{end}}{{if .Evidence}} {{.Evidence}} exact code {{if eq .Evidence 1}}reference{{else}}references{{end}}.{{else}} No code yet.{{end}}</p></li>{{end}}

{{define "data-entity-detail"}}{{if .Holders}}<h3>Held by</h3><p class="term-empty">A holding resource carries or stores this data; it is a separate Component, not the data itself.</p><ul class="trace-links entity-holders">{{range .Holders}}<li><a href="{{.Href}}">{{.Name}}</a> <small class="trace-note">{{.Role}}</small>{{if ne .Status "current"}} <small class="gap">{{.Status}}</small>{{end}}</li>{{end}}</ul>{{end}}
<h3>Fields</h3>{{if .Fields}}<table class="entity-fields"><caption>Author-selected fields; this is not an exhaustive schema.</caption><thead><tr><th scope="col">Field</th><th scope="col">Type</th><th scope="col">Keys</th><th scope="col">Note</th></tr></thead><tbody>{{range .Fields}}<tr><th scope="row"><code>{{.Name}}</code></th><td>{{.Type}}</td><td>{{range $i, $k := .Keys}}{{if $i}}, {{end}}{{$k}}{{end}}</td><td>{{.Note}}</td></tr>{{end}}</tbody></table>{{else}}<p class="term-empty">No fields are described. Omitted fields do not imply missing implementation.</p>{{end}}
<h3>Relationships</h3>{{if or .Relationships .Incoming}}<ul class="relationship-list">{{range .Relationships}}{{template "relationship-row" .}}{{end}}{{range .Incoming}}{{template "relationship-row" .}}{{end}}</ul>{{else}}<p class="term-empty">No relationships are declared.</p>{{end}}
{{if .ERDs}}<h3>Shown in</h3><ul class="trace-links">{{range .ERDs}}<li><a href="{{.Href}}">{{.Name}}</a>{{if not .Pinned}} <small class="gap">at revision {{.Revision}}</small>{{end}}</li>{{end}}</ul>{{end}}{{end}}

{{define "erd-view"}}<div class="erd-view" id="{{.DOMID}}" data-erd-view="{{.Target}}" data-erd-revision="{{.Revision}}">
{{if .BaselineHref}}<p class="erd-overlay-note">Proposed changes over <a href="{{.BaselineHref}}">{{.BaselineName}}</a>{{if .Feature}} for <a href="{{.FeatureHref}}">{{.Feature}}</a>{{end}}. The baseline ERD is not rewritten.</p>{{end}}
{{if .VisualNote}}<p role="status" class="gap" data-erd-visual-note>{{.VisualNote}}</p>{{end}}
{{if .Visual}}<figure class="erd-visual" data-erd-visual aria-label="{{.Name}}: authored diagram; the directory below lists every entity.">{{.Visual}}<ul hidden data-erd-bindings>{{range .Bindings}}<li data-erd-element="{{.Element}}" data-erd-target="{{.Pin.Target}}" data-erd-pin="{{.Pin.Revision}}"{{if .Relationship}} data-erd-relationship="{{.Relationship}}"{{end}} data-erd-intent="{{.Intent}}" data-erd-status="{{.Status}}">{{.Label}}</li>{{end}}</ul><figcaption>Select a drawn entity or relationship to open its pinned definition. {{.Drawn}} of {{len .Directory}} directory {{if eq (len .Directory) 1}}entity is{{else}}entities are{{end}} drawn{{if .Omitted}}; {{.Omitted}} {{if eq .Omitted 1}}is{{else}}are{{end}} in the directory but not drawn{{end}}.</figcaption></figure>{{end}}
<h3>Entity directory</h3><table class="directory-table erd-directory" data-erd-directory><thead><tr><th scope="col">Data entity</th><th scope="col">Intent</th><th scope="col">In the diagram</th><th scope="col">Held by</th><th scope="col" class="wide">Purpose</th></tr></thead><tbody>{{range .Directory}}<tr data-erd-directory-row="{{.Pin.Target}}" data-erd-row-pin="{{.Pin.Revision}}"{{if .Change}} data-overlay-change="{{.Change}}"{{end}}><th scope="row">{{if .Href}}<a href="{{.Href}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}{{if .Pin.Revision}} <button type="button" class="icon-button" data-documentation-target="{{.Pin.Target}}" data-documentation-revision="{{.Pin.Revision}}" aria-label="Open {{.Name}} explanation" title="Open explanation"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-book"></use></svg></button>{{end}}{{if eq .Change "adds"}} <small class="technical-change">added by this overlay</small>{{else if eq .Change "replaces"}} <small class="technical-change">proposed revision replacing {{.ChangeNote}}</small>{{else if eq .Change "removes"}} <small class="technical-change">proposed removal: {{.ChangeNote}}</small>{{end}}{{if and (ne .Status "current") (ne .Change "removes")}} <small class="gap">{{.Status}}</small>{{end}}</th><td><span class="technical-intent intent-{{.Intent}}">{{.Intent}}</span></td><td>{{if .Elements}}drawn{{else}}<span class="directory-gap">not drawn</span>{{end}}</td><td>{{range $i, $h := .Holders}}{{if $i}}, {{end}}{{$h}}{{else}}<span class="directory-gap">none named</span>{{end}}</td><td>{{.Explanation}}</td></tr>{{end}}</tbody></table>
{{if .Relationships}}<h3>Relationships</h3><ul class="relationship-list">{{range .Relationships}}{{template "relationship-row" .}}{{end}}</ul>{{end}}
</div>{{end}}
`

const technicalERDStyles = `
.erd-visual{margin:12px 0;padding:8px;border:1px solid var(--line);border-radius:8px;background:var(--bg);overflow:auto}
.erd-visual>svg{display:block;width:100%;height:auto;max-width:1200px}
.erd-visual figcaption{margin-top:6px;color:var(--muted);font:12px var(--ui)}
.erd-visual [data-erd-bound]{cursor:pointer}
.erd-visual [data-erd-bound]:hover,.erd-visual [data-erd-bound]:focus-visible,.erd-visual [data-erd-bound].erd-highlight{outline:3px solid var(--accent,#2f6fdd);outline-offset:2px}
.erd-directory{min-width:560px}
.erd-view{overflow-x:auto}
.relationship-list{list-style:none;padding:0}
.relationship-list li{margin:8px 0}
.relationship-glyph{vertical-align:middle;margin-right:8px;color:var(--ink)}
.relationship-production .relationship-glyph{color:var(--amber,#9a5b00)}
.cardinality{font:600 12px var(--mono);padding:0 3px;border:1px solid var(--line);border-radius:3px}
.technical-intent{font:600 11px var(--ui);text-transform:uppercase;letter-spacing:.03em}
.technical-intent.intent-proposed{color:var(--amber,#9a5b00);border-bottom:1px dashed currentColor}
.technical-intent.intent-unspecified,.technical-intent.intent-unknown{color:var(--muted);font-style:italic}
.technical-change{color:var(--amber,#9a5b00);font:600 11px var(--ui)}
.entity-fields{border-collapse:collapse;font:13px var(--ui)}
.entity-fields caption{text-align:left;color:var(--muted);font-size:12px;padding-bottom:4px}
.entity-fields th,.entity-fields td{border-bottom:1px solid var(--line-soft,#eee);padding:4px 10px 4px 0;text-align:left;vertical-align:top}
`
