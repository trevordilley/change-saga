package server

import (
	"context"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/plancheckpoint"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A review that covers a Plan's checkpoint shows the checkpoint beside its deck:
// the Plan with what was done before, what this checkpoint did, and what is to
// come; the stories it addresses and the evidence behind each criterion; how big it
// was to explain; and what would make passing it surprising.

type checkpointReviewView struct {
	*plancheckpoint.Report
	PlanTitle, PlanHref string
	Columns             []checkpointColumn
}

// checkpointColumn is one phase of the Plan as the checkpoint sees it.
type checkpointColumn struct {
	Title      string
	Checkpoint bool
	Current    bool
	Cells      []checkpointCell
}

type checkpointCell struct {
	Title, Href, Status string
	// Class is before, now, or later: done in an earlier stretch, in this one, or to come.
	Class string
}

func (a *app) checkpointReview(ctx context.Context, document *saga.Saga, plan *saga.Plan, checkpointID string) *checkpointReviewView {
	events, _ := a.planActivityEvents(ctx, plan.ID)
	report, err := plancheckpoint.Build(document, plan, checkpointID, plancheckpoint.Stories(document.Root, document.Manifest.ID), planactivity.Summarize(events))
	if err != nil {
		return nil
	}
	view := &checkpointReviewView{Report: report, PlanTitle: plan.Title, PlanHref: planHref(plan.ID)}
	inScope := map[string]bool{}
	for _, phase := range report.Phases {
		inScope[phase] = true
	}
	ready := planReady(plan)
	place := "before"
	for _, phase := range plan.Phases {
		if phase.IsCheckpoint() {
			current := phase.ID == checkpointID
			view.Columns = append(view.Columns, checkpointColumn{Title: phase.Title, Checkpoint: true, Current: current})
			if current {
				place = "later"
			}
			continue
		}
		column := checkpointColumn{Title: phase.Title}
		for _, task := range plan.Tasks {
			if task.Phase != phase.ID {
				continue
			}
			class := place
			if inScope[phase.ID] {
				class = "now"
			}
			column.Cells = append(column.Cells, checkpointCell{Title: task.Title, Href: taskHref(plan.ID, task.ID), Status: statusOf(task, ready).Word, Class: class})
		}
		if len(column.Cells) > 0 {
			view.Columns = append(view.Columns, column)
		}
	}
	return view
}

const checkpointReviewTemplates = `{{define "checkpoint-review-link"}}<p class="review-deck-pr" data-review-checkpoint>Checkpoint <strong>{{.Title}}</strong> of <a href="{{.PlanHref}}">{{.PlanTitle}}</a>: {{.State}}. <button type="button" class="plan-slide-tasks-open" data-plan-drawer="review-checkpoint" data-plan-drawer-title="Checkpoint" aria-label="Open checkpoint {{.Title}}">Open checkpoint</button></p><template id="review-checkpoint">{{template "checkpoint-review" .}}</template>{{end}}
{{define "checkpoint-review"}}<section class="plan-drawer checkpoint-review" data-checkpoint-review="{{.Checkpoint}}"><h2>Checkpoint {{.Title}}</h2><p class="plan-note">{{.TasksDone}} of its {{.Tasks}} tasks done in <a href="{{.PlanHref}}">{{.PlanTitle}}</a>. This review is the checkpoint's work as a change into the Plan's feature branch{{if .WidenedTo}}, <span data-checkpoint-widened>widened to every change since checkpoint {{.WidenedTo}}</span>{{end}}.</p>{{with .Passed}}<p class="checkpoint-passed" data-checkpoint-passed>Passed by {{.By}}{{if .Agent}}, an agent,{{end}} on {{.At.Format "2006-01-02"}}: {{.Reason}}</p>{{else}}<p data-checkpoint-next>{{.Next}}</p>{{end}}<h3>The Plan at this checkpoint</h3><p class="checkpoint-legend"><span><i class="checkpoint-cell before"></i>Done before</span><span><i class="checkpoint-cell now"></i>In this checkpoint</span><span><i class="checkpoint-cell later"></i>To come</span></p><div class="checkpoint-grid" data-checkpoint-grid>{{range .Columns}}{{if .Checkpoint}}<div class="checkpoint-gate{{if .Current}} current{{end}}" title="Checkpoint: {{.Title}}"><span>{{.Title}}</span></div>{{else}}<div class="checkpoint-phase"><h4>{{.Title}}</h4><div class="checkpoint-cells">{{range .Cells}}<a class="checkpoint-cell {{.Class}}" href="{{.Href}}" title="{{.Title}}: {{.Status}}" aria-label="{{.Title}}: {{.Status}}"></a>{{end}}</div></div>{{end}}{{end}}</div><h3>Size</h3><p data-checkpoint-size>About {{.Estimate}} {{if eq .Estimate 1}}slide{{else}}slides{{end}} estimated, one per story{{if .Slides}}; this review has {{.Slides}}{{end}}.</p><h3>Stories and the proof the work works</h3>{{range .Stories}}<div class="checkpoint-story" data-checkpoint-story><strong>{{.Title}}</strong><ul>{{range .Criteria}}<li>{{.Statement}} {{if .Proven}}{{range .Tests}}{{if .Evidence}}<span class="plan-state done" title="{{.Title}}">{{.Evidence}} evidence</span> {{end}}{{end}}{{else}}<span class="plan-state warn" data-checkpoint-unproven>No test evidence</span>{{end}}</li>{{else}}<li class="plan-note">No acceptance criteria yet.</li>{{end}}</ul></div>{{else}}<p class="plan-note">Its Tasks lead to no story yet.</p>{{end}}{{if .Changes}}<h3>How the Plan changed since checkpoint {{.ChangedSince}} passed</h3><table class="plan-table" data-checkpoint-changes><tbody>{{range .Changes}}<tr><td><span class="plan-state {{if eq .Kind "dropped"}}warn{{else}}working{{end}}">{{.Kind}}</span></td><td>{{if .Title}}{{.Title}}{{else}}{{.Task}}{{end}}</td><td class="plan-note">{{if .From}}{{.From}}{{end}}{{if and .From .To}} → {{end}}{{if .To}}{{.To}}{{end}}</td></tr>{{end}}</tbody></table>{{end}}{{if and .Surprises (not .Passed)}}<h3>Surprising</h3><ul data-checkpoint-surprises>{{range .Surprises}}<li>{{.}}</li>{{end}}</ul>{{end}}</section>{{end}}
`

const checkpointReviewStyles = `
.checkpoint-review h3{margin:16px 0 6px}
.checkpoint-passed{color:var(--review-approved-ink);background:var(--review-approved-bg);border-radius:7px;padding:7px 10px}
.checkpoint-legend{display:flex;gap:12px;flex-wrap:wrap;font-size:12px;color:var(--muted);margin:0 0 6px}
.checkpoint-legend .checkpoint-cell{display:inline-block;margin-right:4px;vertical-align:-1px}
.checkpoint-grid{display:flex;gap:6px;overflow-x:auto;padding-bottom:4px}
.checkpoint-phase{flex:0 0 auto;max-width:150px;background:var(--bg-subtle);border-radius:7px;padding:5px}
.checkpoint-phase h4{margin:0 0 4px;font-size:11px;color:var(--muted);font-weight:600}
.checkpoint-cells{display:grid;grid-template-columns:repeat(6,13px);gap:3px}
.checkpoint-cell{display:block;width:13px;height:13px;border-radius:3px}
.checkpoint-cell.before{background:var(--bg-inset);outline:1px solid var(--line)}
.checkpoint-cell.now{background:var(--accent)}
.checkpoint-cell.later{outline:1px dashed var(--line)}
.checkpoint-gate{flex:0 0 auto;display:flex;align-items:center;writing-mode:vertical-rl;font-size:11px;font-weight:600;color:var(--muted);border-left:2px dashed var(--line);padding:0 3px}
.checkpoint-gate.current{color:var(--accent);border-left-color:var(--accent)}
.checkpoint-story{margin:8px 0}
.checkpoint-story ul{margin:3px 0 0;padding-left:18px;font-size:13px}
.checkpoint-story li{margin:3px 0}
`
