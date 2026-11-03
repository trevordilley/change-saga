package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Task activity is what agents say while they work: a live view shared by every
// worktree of a repository, never part of the Plan. Nothing here writes to the
// Saga, and no failure here fails a command: the Plan's records stay the fact.

// taskActivityResult is what task start and task note report with --json.
type taskActivityResult struct {
	OK        bool     `json:"ok"`
	Operation string   `json:"operation"`
	Task      string   `json:"task"`
	Agent     string   `json:"agent"`
	Recorded  bool     `json:"recorded"`
	Reason    string   `json:"reason,omitempty"`
	AlsoOnIt  []string `json:"also_on_it,omitempty"`
}

// activityFlags are the flags every command reporting activity takes.
type activityFlags struct {
	agent, repo *string
}

func addActivityFlags(flags interface {
	String(name, value, usage string) *string
}) activityFlags {
	return activityFlags{
		agent: flags.String("agent", "", "who is reporting; defaults to the DevSwarm workspace, else the Git branch"),
		repo:  flags.String("repo", "", "code checkout when the Saga lives in a companion repository"),
	}
}

// reportActivity appends one event and says whether it landed. reason is empty
// when it did.
func reportActivity(ctx context.Context, root, repo, agentOverride, planID, taskID, kind, message string) (agent string, reason string) {
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return "", err.Error()
	}
	log, reason := planactivity.Open(ctx, root, repo, manifest.ID)
	branch := planactivity.Branch(ctx, root)
	if branch == "" && repo != "" {
		branch = planactivity.Branch(ctx, repo)
	}
	agent = planactivity.Identity(agentOverride, os.Getenv, branch)
	if log == nil {
		return agent, reason
	}
	if err := log.Append(planID, planactivity.Event{At: time.Now().UTC(), Agent: agent, Task: taskID, Kind: kind, Message: message, Branch: branch}); err != nil {
		return agent, "Could not write the activity log: " + err.Error()
	}
	return agent, ""
}

// checkLiveTask refuses a Task that is not there, or of a delivered Plan, reading
// only the two records involved: reporting activity must stay quick.
func checkLiveTask(root, planID, taskID string) error {
	plan, tasks, err := saga.ReadPlanRecords(root, planID)
	if err != nil {
		return fmt.Errorf("plan %q does not exist", planID)
	}
	if plan.DeliveredAt != nil {
		return fmt.Errorf("plan %q was delivered on %s and no longer changes; write a new Plan for further work", planID, plan.DeliveredAt.Format("2006-01-02"))
	}
	for _, task := range tasks {
		if task.ID == taskID {
			return nil
		}
	}
	return fmt.Errorf("task %q is not in plan %q", taskID, planID)
}

func taskStart(ctx context.Context, args []string, out io.Writer) error {
	name := "task start"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN; required unless --task is a task URN")
	task := flags.String("task", "", "task id or URN")
	activity := addActivityFlags(flags)
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *task); err != nil {
		return err
	}
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	if err := checkLiveTask(root, planID, taskID); err != nil {
		return err
	}
	holders := liveTaskActivity(ctx, root, *activity.repo, planID)[taskID]
	agent, reason := reportActivity(ctx, root, *activity.repo, *activity.agent, planID, taskID, planactivity.KindStart, "")
	also := []string{}
	if holders != nil {
		now := time.Now()
		for _, holder := range holders.Holders {
			if holder.Agent != agent {
				also = append(also, holder.Agent+" ("+ago(now, holder.At)+")")
			}
		}
	}
	return writeActivityResult(out, name, root, planID, taskID, agent, reason, also, *jsonOutput, "Started")
}

func taskNote(ctx context.Context, args []string, out io.Writer) error {
	name := "task note"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN; required unless --task is a task URN")
	task := flags.String("task", "", "task id or URN")
	blocked := flags.Bool("blocked", false, "the Task cannot go on until someone acts; a later note clears it")
	activity := addActivityFlags(flags)
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	// A note's words come before the Saga, so the flags may fall on either side.
	positional := []string{}
	remaining := args
	for {
		if err := flags.Parse(remaining); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		remaining = flags.Args()[1:]
	}
	if len(positional) != 2 || strings.TrimSpace(positional[0]) == "" || strings.TrimSpace(*task) == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	root := positional[1]
	if err := checkLiveTask(root, planID, taskID); err != nil {
		return err
	}
	kind, verb := planactivity.KindNote, "Noted"
	if *blocked {
		kind, verb = planactivity.KindBlocked, "Marked blocked"
	}
	agent, reason := reportActivity(ctx, root, *activity.repo, *activity.agent, planID, taskID, kind, positional[0])
	return writeActivityResult(out, name, root, planID, taskID, agent, reason, nil, *jsonOutput, verb)
}

func writeActivityResult(out io.Writer, operation, root, planID, taskID, agent, reason string, also []string, jsonOutput bool, verb string) error {
	if jsonOutput {
		urn := taskID
		if manifest, err := saga.ReadManifest(root); err == nil {
			urn = saga.TaskTarget(manifest.ID, planID, taskID)
		}
		return writeJSON(out, taskActivityResult{OK: true, Operation: operation, Task: urn, Agent: agent, Recorded: reason == "", Reason: reason, AlsoOnIt: also})
	}
	if reason != "" {
		fmt.Fprintf(out, "%s Nothing was recorded, and the Plan is unchanged.\n", reason)
		return nil
	}
	fmt.Fprintf(out, "%s %s as %s. This is live activity: nothing to commit.\n", verb, taskID, agent)
	if len(also) > 0 {
		fmt.Fprintf(out, "Also on it: %s. Check with them before going on.\n", strings.Join(also, ", "))
	}
	return nil
}

// recordTaskStateActivity reports a durable state change as activity, so a reader
// on another worktree sees it. A failure is not reported: the state was written,
// and that is what the command was asked to do.
func recordTaskStateActivity(ctx context.Context, root, repo, agent, planID, taskID, state, message string) {
	kinds := map[string]string{saga.TaskDone: planactivity.KindDone, saga.TaskInProgress: planactivity.KindStart, saga.TaskPlanned: planactivity.KindReleased}
	if kind := kinds[state]; kind != "" {
		reportActivity(ctx, root, repo, agent, planID, taskID, kind, message)
	}
}

// liveTaskActivity is a Plan's activity by Task. It is empty when there is no log
// or it cannot be read, which never matters to whoever asks.
func liveTaskActivity(ctx context.Context, root, repo, planID string) map[string]*planactivity.TaskActivity {
	activity, _, _ := readTaskActivity(ctx, root, repo, planID)
	return activity
}

// readTaskActivity also returns the Plan's events, and why there is no log if not.
func readTaskActivity(ctx context.Context, root, repo, planID string) (map[string]*planactivity.TaskActivity, []planactivity.Event, string) {
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return nil, nil, ""
	}
	log, reason := planactivity.Open(ctx, root, repo, manifest.ID)
	if log == nil {
		return nil, nil, reason
	}
	events, err := log.Read(planID)
	if err != nil {
		return nil, nil, ""
	}
	return planactivity.Summarize(events), events, ""
}

// taskActivityView is a Task's latest activity, kept compact: who, what state, the
// latest message cut short. Full history is query task --history.
type taskActivityView struct {
	State   string    `json:"state"`
	Agents  []string  `json:"agents,omitempty"`
	Branch  string    `json:"branch,omitempty"`
	Quiet   bool      `json:"quiet,omitempty"`
	At      time.Time `json:"at"`
	Message string    `json:"message,omitempty"`
}

// activityMessageRunes bounds the message a compact view carries.
const activityMessageRunes = 140

// activityView reads a Task's activity against its durable state. A Task durably
// done is simply done, so it has no live state to report.
func activityView(activity *planactivity.TaskActivity, durableState string, now time.Time) *taskActivityView {
	if activity == nil || durableState == saga.TaskDone {
		return nil
	}
	view := &taskActivityView{Agents: activity.Agents(), Quiet: activity.Quiet(now), At: activity.Latest.At, Message: cutRunes(activity.Latest.Message, activityMessageRunes)}
	switch activity.State() {
	case planactivity.StateWorking:
		view.State, view.Branch = "working", activity.Holders[0].Branch
	case planactivity.StateBlocked:
		view.State, view.Branch = "blocked", activity.Holders[0].Branch
	case planactivity.StateFinished:
		view.State, view.Branch = "done_on_branch", activity.Finished.Branch
	default:
		return nil
	}
	return view
}

func cutRunes(text string, limit int) string {
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return text
}

// ago says how long since at, in the largest whole unit.
func ago(now, at time.Time) string {
	switch elapsed := now.Sub(at); {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	}
}

// activityWords says a Task's live state for task list.
func activityWords(view *taskActivityView, now time.Time) string {
	who := strings.Join(view.Agents, ", ")
	switch view.State {
	case "working":
		if view.Quiet {
			return "started by " + who + ", quiet since " + ago(now, view.At)
		}
		return "started by " + who + " " + ago(now, view.At)
	case "blocked":
		return "blocked: " + who + ", " + ago(now, view.At)
	case "done_on_branch":
		return "done on " + firstNonEmpty(view.Branch, "another branch")
	}
	return ""
}
