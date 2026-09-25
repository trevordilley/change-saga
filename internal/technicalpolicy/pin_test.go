package technicalpolicy

import (
	"reflect"
	"strings"
	"testing"
)

func TestPinAdmission(t *testing.T) {
	pin := Pin{"entity", "r1"}
	current := GlobalHealth{pin, DefinitionUnique, Active}
	saved := SavedView{"snapshot:fixture", strings.Repeat("a", 40), true, pin, DefinitionUnique, Active}
	tests := []struct {
		name   string
		edit   func(*PinRequest)
		reason PinReason
		basis  AdmissionBasis
	}{
		{"current", func(r *PinRequest) {}, "", Current},
		{"old without view", func(r *PinRequest) { r.Global.Current.Revision = "r2" }, PinNotCurrent, ""},
		{"global unknown", func(r *PinRequest) { r.Global = GlobalHealth{} }, PinNotCurrent, ""},
		{"global conflict", func(r *PinRequest) { r.Global.Definition = DefinitionConflicted }, PinNotCurrent, ""},
		{"global retired", func(r *PinRequest) { r.Global.Lifecycle = Retired }, PinNotCurrent, ""},
		{"empty pin", func(r *PinRequest) { r.Requested = Pin{} }, PinInvalid, ""},
		{"valid saved current", func(r *PinRequest) { v := saved; r.View = &v }, "", Saved},
		{"saved old global retired conflict", func(r *PinRequest) {
			v := saved
			r.View = &v
			r.Global = GlobalHealth{Pin{"entity", "r2"}, DefinitionConflicted, Retired}
		}, "", Saved},
		{"saved global unknown", func(r *PinRequest) { v := saved; r.View = &v; r.Global = GlobalHealth{} }, "", Saved},
		{"missing selected view no fallback", func(r *PinRequest) { r.View = &SavedView{} }, ViewMissing, ""},
		{"unresolved label", func(r *PinRequest) { v := saved; v.Resolved = false; r.View = &v }, ViewMissing, ""},
		{"missing artifact", func(r *PinRequest) { v := saved; v.Artifact = " "; r.View = &v }, ViewMissing, ""},
		{"symbolic source", func(r *PinRequest) { v := saved; v.SourceCommit = "HEAD"; r.View = &v }, ViewMissing, ""},
		{"missing binding", func(r *PinRequest) { v := saved; v.Binding = Pin{}; r.View = &v }, ViewBindingMissing, ""},
		{"different revision", func(r *PinRequest) { v := saved; v.Binding.Revision = "r2"; r.View = &v }, ViewBindingMismatch, ""},
		{"different target", func(r *PinRequest) { v := saved; v.Binding.Target = "other"; r.View = &v }, ViewBindingMismatch, ""},
		{"missing definition", func(r *PinRequest) { v := saved; v.Definition = DefinitionMissing; r.View = &v }, ViewDefinitionMissing, ""},
		{"ambiguous definition", func(r *PinRequest) { v := saved; v.Definition = DefinitionConflicted; r.View = &v }, ViewAmbiguous, ""},
		{"ambiguous lifecycle", func(r *PinRequest) { v := saved; v.Lifecycle = LifecycleConflicted; r.View = &v }, ViewAmbiguous, ""},
		{"inactive saved", func(r *PinRequest) { v := saved; v.Lifecycle = Retired; r.View = &v }, ViewInactive, ""},
		{"unknown definition", func(r *PinRequest) { v := saved; v.Definition = "future"; r.View = &v }, ViewUnknown, ""},
		{"unknown lifecycle", func(r *PinRequest) { v := saved; v.Lifecycle = ""; r.View = &v }, ViewUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := PinRequest{Requested: pin, Global: current}
			tt.edit(&request)
			before := request
			if request.View != nil {
				v := *request.View
				before.View = &v
			}
			got := AdmitPin(request)
			if got.Admitted != (tt.reason == "") || got.Reason != tt.reason || got.Basis != tt.basis || got.Global != request.Global {
				t.Fatalf("got %+v", got)
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("mutated input")
			}
		})
	}
}
