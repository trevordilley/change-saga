package diagram

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Reveal lets a diagram unfold in its reading order. The renderer writes the
// animation as CSS keyframes keyed to :target of a renderer-owned anchor, so a
// drawing opened without that fragment, as a thumbnail, an export, or a
// landmark link, shows its finished state, and nothing runs script.
const (
	// RevealAnchor and RevealReplayAnchor are the fragments that play the
	// reveal. Navigating to the fragment already shown does not restart a CSS
	// animation, so a viewer alternates them to replay it.
	RevealAnchor       = "diagram-reveal"
	RevealReplayAnchor = "diagram-reveal-replay"
	// MaxRevealStep bounds an authored step; steps are ranked, so gaps between
	// them cost no time.
	MaxRevealStep = 1000

	revealFadeMS     = 500
	revealIntervalMS = 350
	// revealSpanMS caps when the last step starts, so a long diagram still
	// finishes promptly: its steps start closer together instead.
	revealSpanMS = 5000
)

var revealModes = map[string]bool{"fade": true}

// RevealModes lists the supported document reveal values for discovery.
func RevealModes() []string {
	modes := []string{}
	for mode := range revealModes {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	return modes
}

// validateReveal reports every reveal problem in d.
func validateReveal(d Document, add func(format string, args ...any)) {
	if d.Reveal != "" && !revealModes[d.Reveal] {
		add("reveal %q is not supported; use %s, or omit reveal for a static drawing", d.Reveal, strings.Join(RevealModes(), ", "))
	}
	for _, e := range d.Elements {
		if e.Step == 0 {
			continue
		}
		if e.Step < 1 || e.Step > MaxRevealStep {
			add("%s: step must be 1-%d", e.ID, MaxRevealStep)
		}
		if d.Reveal == "" {
			add("%s: step orders the reveal, but the diagram does not set one; set it with the canvas operation {\"reveal\":\"fade\"} or remove the step", e.ID)
		}
	}
}

// revealPlan is each element's resolved entrance. Step is the ranked step an
// element becomes visible at, 0 for one present from the start; an element
// animates itself only when it appears later than its parent group.
type revealPlan struct {
	step    map[string]int
	animate map[string]bool
	steps   int
}

// planReveal resolves the reveal order. A semantic element enters at its
// explicit step or else at its position among semantic elements; a decorative
// one without a step enters with its parent group, or is present from the
// start. No element appears before its parent group, and elements with the
// same step appear together.
func planReveal(d Document) revealPlan {
	plan := revealPlan{step: map[string]int{}, animate: map[string]bool{}}
	if d.Reveal == "" {
		return plan
	}
	own := map[string]int{}
	byID := map[string]Element{}
	ordinal := 0
	for _, e := range d.Elements {
		byID[e.ID] = e
		if !e.Decorative {
			ordinal++
		}
		switch {
		case e.Step > 0:
			own[e.ID] = e.Step
		case !e.Decorative:
			own[e.ID] = ordinal
		}
	}
	raw := map[string]int{}
	var resolve func(id string, depth int) int
	resolve = func(id string, depth int) int {
		if value, ok := raw[id]; ok {
			return value
		}
		e, ok := byID[id]
		if !ok || depth > len(d.Elements) {
			return 0
		}
		value := max(own[id], resolve(e.Parent, depth+1))
		raw[id] = value
		return value
	}
	distinct := map[int]bool{}
	for _, e := range d.Elements {
		if value := resolve(e.ID, 0); value > 0 {
			distinct[value] = true
		}
	}
	ranked := make([]int, 0, len(distinct))
	for value := range distinct {
		ranked = append(ranked, value)
	}
	sort.Ints(ranked)
	rank := map[int]int{}
	for index, value := range ranked {
		rank[value] = index + 1
	}
	for _, e := range d.Elements {
		plan.step[e.ID] = rank[raw[e.ID]]
		plan.animate[e.ID] = raw[e.ID] > resolve(e.Parent, 0)
	}
	plan.steps = len(ranked)
	return plan
}

// revealDefs adds the reveal's stylesheet and anchors to a rendering.
func (r *renderer) revealDefs(svg *node) {
	if r.reveal.steps == 0 {
		return
	}
	interval := min(revealIntervalMS, revealSpanMS/r.reveal.steps)
	var css strings.Builder
	css.WriteString("@media (prefers-reduced-motion:no-preference){")
	for _, anchor := range []string{RevealAnchor, RevealReplayAnchor} {
		fmt.Fprintf(&css, "#%[1]s:target~* [data-reveal-step],#%[1]s:target~[data-reveal-step]{animation-name:%[1]s-fade;animation-duration:%[2]dms;animation-timing-function:ease-out;animation-fill-mode:backwards}", anchor, revealFadeMS)
	}
	for step := 2; step <= r.reveal.steps; step++ {
		fmt.Fprintf(&css, `[data-reveal-step="%d"]{animation-delay:%dms}`, step, (step-1)*interval)
	}
	css.WriteString("}")
	for _, anchor := range []string{RevealAnchor, RevealReplayAnchor} {
		fmt.Fprintf(&css, "@keyframes %s-fade{from{opacity:0}}", anchor)
	}
	r.defs.add("style").text = css.String()
	svg.add("g", "id", RevealAnchor)
	svg.add("g", "id", RevealReplayAnchor)
}

// revealStep marks an element that enters on its own with its ranked step.
func (r *renderer) revealStep(e Element, g *node) {
	if r.reveal.animate[e.ID] {
		g.set("data-reveal-step", strconv.Itoa(r.reveal.step[e.ID]))
	}
}
