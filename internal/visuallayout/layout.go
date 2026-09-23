// Package visuallayout renders small, deterministic SVG starting points for
// Change Saga slides. The caller owns the meaning of every node and edge; the
// helpers only arrange authored facts and preserve stable Item IDs in the SVG.
package visuallayout

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

const (
	Width  = 1280
	Height = 720
)

var stableID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Node is one addressable visual element. ID is emitted unchanged as both the
// SVG id and data-item-id so an Item selector can point to it exactly.
type Node struct {
	ID     string
	Label  string
	Detail string
}

// Link is an authored relationship. Rendering a link does not verify that its
// direction or meaning is semantically correct.
type Link struct {
	ID    string
	From  string
	To    string
	Label string
}

// Lane groups owned nodes under one stable owner ID.
type Lane struct {
	ID    string
	Label string
	Nodes []Node
}

// Sequence renders participants and authored messages from left to right.
func Sequence(title string, participants []Node, messages []Link) ([]byte, error) {
	if len(participants) == 0 || len(participants) > 8 {
		return nil, fmt.Errorf("sequence requires 1 to 8 participants")
	}
	if len(messages) > 12 {
		return nil, fmt.Errorf("sequence supports at most 12 messages")
	}
	if err := validateGraph(participants, messages); err != nil {
		return nil, err
	}
	var body strings.Builder
	centers := map[string]int{}
	gap := 1080 / len(participants)
	for i, node := range participants {
		x := 100 + i*gap
		centers[node.ID] = x + gap/2 - 12
		fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><rect x="%d" y="92" width="%d" height="72" rx="14" class="node"/><text x="%d" y="124" class="label">%s</text><text x="%d" y="148" class="detail">%s</text><path d="M%d 164V660" class="lifeline"/></g>`, attr(node.ID), attr(node.ID), x, gap-24, x+16, text(node.Label), x+16, text(node.Detail), centers[node.ID])
	}
	for i, message := range messages {
		y := 205 + i*36
		from, to := centers[message.From], centers[message.To]
		anchor := "start"
		labelX := from + 8
		if to < from {
			anchor, labelX = "end", from-8
		}
		fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><path d="M%d %dH%d" class="link" marker-end="url(#arrow)"/><text x="%d" y="%d" text-anchor="%s" class="edge-label">%s</text></g>`, attr(message.ID), attr(message.ID), from, y, to, labelX, y-7, anchor, text(message.Label))
	}
	return svg(title, body.String()), nil
}

// State renders states in a bounded grid and the authored transitions between
// them. Geometry is deterministic; it is not a state-machine validator.
func State(title string, states []Node, transitions []Link) ([]byte, error) {
	if len(states) == 0 || len(states) > 12 {
		return nil, fmt.Errorf("state layout requires 1 to 12 states")
	}
	if len(transitions) > 18 {
		return nil, fmt.Errorf("state layout supports at most 18 transitions")
	}
	if err := validateGraph(states, transitions); err != nil {
		return nil, err
	}
	type point struct{ x, y int }
	points := map[string]point{}
	var body strings.Builder
	for i, node := range states {
		col, row := i%4, i/4
		x, y := 70+col*300, 116+row*180
		points[node.ID] = point{x + 120, y + 55}
		fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><rect x="%d" y="%d" width="240" height="110" rx="18" class="node"/><text x="%d" y="%d" class="label">%s</text><text x="%d" y="%d" class="detail">%s</text></g>`, attr(node.ID), attr(node.ID), x, y, x+18, y+46, text(node.Label), x+18, y+74, text(node.Detail))
	}
	for _, transition := range transitions {
		from, to := points[transition.From], points[transition.To]
		fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><path d="M%d %dL%d %d" class="link" marker-end="url(#arrow)"/><text x="%d" y="%d" class="edge-label">%s</text></g>`, attr(transition.ID), attr(transition.ID), from.x, from.y, to.x, to.y, (from.x+to.x)/2+8, (from.y+to.y)/2-8, text(transition.Label))
	}
	return svg(title, body.String()), nil
}

// Ownership renders owner lanes with addressable work or subsystem cards.
func Ownership(title string, lanes []Lane) ([]byte, error) {
	if len(lanes) == 0 || len(lanes) > 5 {
		return nil, fmt.Errorf("ownership layout requires 1 to 5 lanes")
	}
	seen := map[string]bool{}
	var body strings.Builder
	laneWidth := 1160 / len(lanes)
	for i, lane := range lanes {
		if err := takeID(lane.ID, seen); err != nil {
			return nil, err
		}
		if len(lane.Nodes) > 5 {
			return nil, fmt.Errorf("ownership lane %q supports at most 5 nodes", lane.ID)
		}
		x := 60 + i*laneWidth
		fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><rect x="%d" y="92" width="%d" height="568" rx="16" class="lane"/><text x="%d" y="130" class="lane-label">%s</text></g>`, attr(lane.ID), attr(lane.ID), x, laneWidth-16, x+16, text(lane.Label))
		for j, node := range lane.Nodes {
			if err := takeID(node.ID, seen); err != nil {
				return nil, err
			}
			y := 154 + j*96
			fmt.Fprintf(&body, `<g id="%s" data-item-id="%s"><rect x="%d" y="%d" width="%d" height="78" rx="12" class="node"/><text x="%d" y="%d" class="label">%s</text><text x="%d" y="%d" class="detail">%s</text></g>`, attr(node.ID), attr(node.ID), x+14, y, laneWidth-44, x+28, y+32, text(node.Label), x+28, y+56, text(node.Detail))
		}
	}
	return svg(title, body.String()), nil
}

func validateGraph(nodes []Node, links []Link) error {
	seen := map[string]bool{}
	nodesByID := map[string]bool{}
	for _, node := range nodes {
		if err := takeID(node.ID, seen); err != nil {
			return err
		}
		nodesByID[node.ID] = true
	}
	for _, link := range links {
		if err := takeID(link.ID, seen); err != nil {
			return err
		}
		if !nodesByID[link.From] || !nodesByID[link.To] {
			return fmt.Errorf("link %q references an unknown node", link.ID)
		}
	}
	return nil
}

func takeID(id string, seen map[string]bool) error {
	if !stableID.MatchString(id) {
		return fmt.Errorf("visual item id %q must be stable lowercase kebab-case", id)
	}
	if seen[id] {
		return fmt.Errorf("visual item id %q is duplicated", id)
	}
	seen[id] = true
	return nil
}

func svg(title, body string) []byte {
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-labelledby="layout-title"><title id="layout-title">%s</title><defs><marker id="arrow" markerWidth="10" markerHeight="10" refX="9" refY="4" orient="auto"><path d="M0 0L0 8L9 4Z" fill="#46607a"/></marker><style>.canvas{fill:#f7f9fc}.node{fill:#fff;stroke:#5c7187;stroke-width:2}.lane{fill:#edf2f7;stroke:#c5d0dc}.label,.lane-label{font:600 18px system-ui,sans-serif;fill:#172635}.detail,.edge-label{font:14px system-ui,sans-serif;fill:#46607a}.link{fill:none;stroke:#46607a;stroke-width:2}.lifeline{stroke:#a7b5c4;stroke-dasharray:6 7}</style></defs><rect class="canvas" width="1280" height="720"/><text x="60" y="58" style="font:700 28px system-ui,sans-serif;fill:#172635">%s</text>%s</svg>`, text(title), text(title), body))
}

func attr(value string) string { return html.EscapeString(value) }
func text(value string) string { return html.EscapeString(strings.TrimSpace(value)) }
