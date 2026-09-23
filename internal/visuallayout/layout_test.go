package visuallayout

import (
	"strings"
	"testing"
)

func TestLayoutsPreserveStableItemIDs(t *testing.T) {
	tests := []struct {
		name string
		run  func() ([]byte, error)
		ids  []string
	}{
		{"sequence", func() ([]byte, error) {
			return Sequence("Request", []Node{{ID: "client", Label: "Client"}, {ID: "api", Label: "API"}}, []Link{{ID: "submit", From: "client", To: "api", Label: "submit"}})
		}, []string{"client", "api", "submit"}},
		{"state", func() ([]byte, error) {
			return State("Lifecycle", []Node{{ID: "draft", Label: "Draft"}, {ID: "ready", Label: "Ready"}}, []Link{{ID: "approve", From: "draft", To: "ready"}})
		}, []string{"draft", "ready", "approve"}},
		{"ownership", func() ([]byte, error) {
			return Ownership("Owners", []Lane{{ID: "platform", Label: "Platform", Nodes: []Node{{ID: "renderer", Label: "Renderer"}}}})
		}, []string{"platform", "renderer"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.run()
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range test.ids {
				if !strings.Contains(string(value), `id="`+id+`" data-item-id="`+id+`"`) {
					t.Errorf("SVG omitted stable Item mapping for %q", id)
				}
			}
			if !strings.Contains(string(value), `viewBox="0 0 1280 720"`) {
				t.Error("SVG omitted the standard canvas")
			}
		})
	}
}

func TestLayoutsRejectAmbiguousOrUnknownIDs(t *testing.T) {
	if _, err := State("bad", []Node{{ID: "same"}, {ID: "same"}}, nil); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate id error = %v", err)
	}
	if _, err := Sequence("bad", []Node{{ID: "left"}}, []Link{{ID: "move", From: "left", To: "missing"}}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown endpoint error = %v", err)
	}
	if _, err := Ownership("bad", []Lane{{ID: "Not Stable"}}); err == nil || !strings.Contains(err.Error(), "kebab-case") {
		t.Fatalf("unstable id error = %v", err)
	}
}

func TestLayoutEscapesAuthoredText(t *testing.T) {
	value, err := Ownership(`<Owners & systems>`, []Lane{{ID: "team", Label: `<Team>`, Nodes: []Node{{ID: "api", Label: `A&B`}}}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(value)
	if strings.Contains(text, "<Owners & systems>") || !strings.Contains(text, "&lt;Owners &amp; systems&gt;") || !strings.Contains(text, "A&amp;B") {
		t.Fatalf("authored text was not escaped: %s", text)
	}
}
