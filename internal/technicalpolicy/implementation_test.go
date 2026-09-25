package technicalpolicy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
)

var delivery = strings.Repeat("b", 40)

func reference() coderef.Reference {
	return coderef.Reference{Commit: strings.Repeat("a", 40), Path: "code.go", Start: 2, End: 3, Digest: coderef.DigestBytes([]byte("two\nthree\n")), Note: "Explains entity behavior"}
}
func candidate() Candidate {
	return Candidate{Pin: Pin{"entity", "r2"}, Intent: Implemented, DeliveryOID: delivery, Evidence: []coderef.Reference{reference()}}
}
func edge(id string) Edge {
	return Edge{ID: id, Intent: Implemented, Destination: Endpoint{Pin{"other", "r1"}, true, Implemented}, Evidence: []coderef.Reference{reference()}}
}
func clone(c Candidate) Candidate {
	c.Evidence = slices.Clone(c.Evidence)
	c.Edges = slices.Clone(c.Edges)
	for i := range c.Edges {
		c.Edges[i].Evidence = slices.Clone(c.Edges[i].Evidence)
	}
	return c
}

type fakeResolver struct {
	calls         []string
	exists        bool
	commitErr     error
	authorErr     error
	wrongOriginal bool
	state         coderesolve.State
	wrongView     bool
}

func goodResolver() *fakeResolver { return &fakeResolver{exists: true, state: coderesolve.Current} }
func (f *fakeResolver) CommitExists(_ context.Context, oid string) (bool, error) {
	f.calls = append(f.calls, "commit:"+oid)
	return f.exists, f.commitErr
}
func (f *fakeResolver) Author(_ context.Context, loc coderef.Location, note string) (coderef.Reference, error) {
	f.calls = append(f.calls, "author:"+loc.String())
	r := reference()
	r.Commit = loc.Commit
	r.Path = loc.Path
	r.Start = loc.Start
	r.End = loc.End
	r.Note = note
	if f.wrongOriginal {
		r.Digest = coderef.DigestBytes(nil)
	}
	return r, f.authorErr
}
func (f *fakeResolver) Resolve(_ context.Context, r coderef.Reference, oid string) coderesolve.Resolution {
	f.calls = append(f.calls, "resolve:"+r.Location().String()+":"+oid)
	loc := r.Location()
	loc.Commit = oid
	if f.wrongView {
		loc.Commit = r.Commit
	}
	return coderesolve.Resolution{State: f.state, Location: loc}
}

func TestCandidateMatrix(t *testing.T) {
	tests := []struct {
		name     string
		edit     func(*Candidate, *fakeResolver)
		codes    []Code
		proposed []string
	}{
		{"implemented", func(c *Candidate, f *fakeResolver) {}, nil, nil},
		{"proposal no code", func(c *Candidate, f *fakeResolver) { c.Intent = Proposed; c.Evidence = nil; c.DeliveryOID = "" }, nil, nil},
		{"mixed intent", func(c *Candidate, f *fakeResolver) {
			c.Edges = []Edge{{ID: "later", Intent: Proposed}, edge("now"), {ID: "also-later", Intent: Proposed}}
		}, nil, []string{"later", "also-later"}},
		{"proposed owner implemented edge", func(c *Candidate, f *fakeResolver) { c.Intent = Proposed; c.Edges = []Edge{edge("bad")} }, []Code{EndpointNotImplemented}, nil},
		{"proposed endpoint", func(c *Candidate, f *fakeResolver) {
			e := edge("bad")
			e.Destination.Intent = Proposed
			c.Edges = []Edge{e}
		}, []Code{EndpointNotImplemented}, nil},
		{"legacy endpoint", func(c *Candidate, f *fakeResolver) {
			e := edge("bad")
			e.Destination.Intent = Unspecified
			c.Edges = []Edge{e}
		}, []Code{EndpointNotImplemented}, nil},
		{"unresolved endpoint", func(c *Candidate, f *fakeResolver) {
			e := edge("bad")
			e.Destination.Resolved = false
			c.Edges = []Edge{e}
		}, []Code{EndpointNotImplemented}, nil},
		{"missing endpoint pin", func(c *Candidate, f *fakeResolver) { e := edge("bad"); e.Destination.Pin = Pin{}; c.Edges = []Edge{e} }, []Code{EndpointNotImplemented}, nil},
		{"self post transition", func(c *Candidate, f *fakeResolver) {
			e := edge("self")
			e.Destination = Endpoint{Pin: c.Pin, Intent: Proposed}
			c.Edges = []Edge{e}
		}, nil, nil},
		{"baseline not candidate", func(c *Candidate, f *fakeResolver) {
			e := edge("baseline")
			e.Destination = Endpoint{Pin: Pin{c.Pin.Target, "r1"}, Resolved: true, Intent: Unspecified}
			c.Edges = []Edge{e}
		}, []Code{EndpointNotImplemented}, nil},
		{"edge own evidence", func(c *Candidate, f *fakeResolver) { e := edge("missing"); e.Evidence = nil; c.Edges = []Edge{e} }, []Code{MissingEvidence}, nil},
		{"owner evidence", func(c *Candidate, f *fakeResolver) { c.Evidence = nil }, []Code{MissingEvidence}, nil},
		{"blank owner", func(c *Candidate, f *fakeResolver) { c.Pin.Target = " " }, []Code{InvalidOwner}, nil},
		{"missing intent", func(c *Candidate, f *fakeResolver) { c.Intent = "" }, []Code{InvalidIntent}, nil},
		{"legacy candidate", func(c *Candidate, f *fakeResolver) { c.Intent = Unspecified }, []Code{InvalidIntent}, nil},
		{"unknown edge intent", func(c *Candidate, f *fakeResolver) { e := edge("unknown"); e.Intent = "future"; c.Edges = []Edge{e} }, []Code{InvalidIntent}, nil},
		{"blank edge", func(c *Candidate, f *fakeResolver) { c.Edges = []Edge{{ID: " ", Intent: Proposed}} }, []Code{InvalidEdgeID}, []string{" "}},
		{"duplicate edge", func(c *Candidate, f *fakeResolver) { c.Edges = []Edge{edge("same"), edge("same")} }, []Code{DuplicateEdgeID}, nil},
		{"missing delivery", func(c *Candidate, f *fakeResolver) { c.DeliveryOID = "" }, []Code{InvalidDelivery}, nil},
		{"symbolic delivery", func(c *Candidate, f *fakeResolver) { c.DeliveryOID = "HEAD" }, []Code{InvalidDelivery}, nil},
		{"short delivery", func(c *Candidate, f *fakeResolver) { c.DeliveryOID = "abcdef" }, []Code{InvalidDelivery}, nil},
		{"sha256 delivery", func(c *Candidate, f *fakeResolver) { c.DeliveryOID = strings.Repeat("c", 64) }, nil, nil},
		{"missing commit", func(c *Candidate, f *fakeResolver) { f.exists = false }, []Code{DeliveryUnavailable}, nil},
		{"commit error", func(c *Candidate, f *fakeResolver) { f.commitErr = errors.New("variable external text") }, []Code{DeliveryUnavailable}, nil},
		{"original unavailable", func(c *Candidate, f *fakeResolver) { f.authorErr = errors.New("missing") }, []Code{InvalidOriginal}, nil},
		{"wrong digest", func(c *Candidate, f *fakeResolver) { f.wrongOriginal = true }, []Code{InvalidOriginal}, nil},
		{"stale delivery", func(c *Candidate, f *fakeResolver) { f.state = coderesolve.Stale }, []Code{StaleDelivery}, nil},
		{"unknown resolution", func(c *Candidate, f *fakeResolver) { f.state = "" }, []Code{StaleDelivery}, nil},
		{"wrong resolved view", func(c *Candidate, f *fakeResolver) { f.wrongView = true }, []Code{StaleDelivery}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := candidate()
			f := goodResolver()
			tt.edit(&c, f)
			before := clone(c)
			got := ValidateCandidate(context.Background(), c, f)
			var codes []Code
			for _, d := range got.Diagnostics {
				codes = append(codes, d.Code)
			}
			if !reflect.DeepEqual(codes, tt.codes) || !reflect.DeepEqual(got.ProposedEdges, tt.proposed) {
				t.Fatalf("got %+v want codes %v proposed %v", got, tt.codes, tt.proposed)
			}
			if got.Valid() != (len(tt.codes) == 0) {
				t.Fatal("invalid result validity")
			}
			if !reflect.DeepEqual(c, before) {
				t.Fatal("mutated candidate")
			}
			again := ValidateCandidate(context.Background(), c, f)
			if !reflect.DeepEqual(got, again) {
				t.Fatal("nondeterministic diagnostics")
			}
			if slices.Contains(codes, InvalidOriginal) {
				for _, call := range f.calls {
					if strings.HasPrefix(call, "resolve:") {
						t.Fatal("delivery checked before original verification passed")
					}
				}
			}
		})
	}
}

func TestReferenceGuardsAndBounds(t *testing.T) {
	edits := []struct {
		name string
		edit func(*coderef.Reference)
	}{
		{"whole file", func(r *coderef.Reference) { r.Start = 0; r.End = 0 }},
		{"empty note", func(r *coderef.Reference) { r.Note = " \n" }},
		{"bad commit", func(r *coderef.Reference) { r.Commit = "HEAD" }},
		{"bad digest", func(r *coderef.Reference) { r.Digest = "sha256:bad" }},
		{"bad path", func(r *coderef.Reference) { r.Path = "../escape" }},
		{"bad range", func(r *coderef.Reference) { r.End = 1 }},
	}
	for _, tt := range edits {
		for _, intent := range []Intent{Proposed, Implemented} {
			t.Run(tt.name+string(intent), func(t *testing.T) {
				c := candidate()
				c.Intent = intent
				tt.edit(&c.Evidence[0])
				c.Edges = []Edge{edge("e")}
				c.Edges[0].Intent = intent
				c.Edges[0].Evidence = slices.Clone(c.Evidence)
				f := goodResolver()
				got := ValidateCandidate(context.Background(), c, f)
				if len(got.Diagnostics) != 2 || got.Diagnostics[0].Code != InvalidReference || got.Diagnostics[1].Code != InvalidReference {
					t.Fatalf("%+v", got)
				}
				if len(f.calls) != 1 {
					t.Fatalf("unexpected byte reads: %v", f.calls)
				}
			})
		}
	}
	for _, n := range []int{64, 65} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			c := candidate()
			c.Evidence = nil
			for i := 0; i < n; i++ {
				r := reference()
				r.Path = fmt.Sprintf("file%d.go", i)
				c.Evidence = append(c.Evidence, r)
			}
			c.Edges = []Edge{edge("e")}
			c.Edges[0].Evidence = slices.Clone(c.Evidence)
			f := goodResolver()
			got := ValidateCandidate(context.Background(), c, f)
			if n == 64 && !got.Valid() {
				t.Fatalf("%+v", got)
			}
			if n == 65 && (len(got.Diagnostics) != 2 || got.Diagnostics[0].Code != TooManyReferences || got.Diagnostics[1].Code != TooManyReferences || len(f.calls) != 1) {
				t.Fatalf("%+v %v", got, f.calls)
			}
			c = candidate()
			for i := 0; i < n; i++ {
				c.Edges = append(c.Edges, Edge{ID: fmt.Sprint(i), Intent: Proposed})
			}
			f = goodResolver()
			got = ValidateCandidate(context.Background(), c, f)
			if n == 64 && (!got.Valid() || len(got.ProposedEdges) != 64) {
				t.Fatalf("%+v", got)
			}
			if n == 65 && (len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != TooManyEdges || len(f.calls) != 0) {
				t.Fatalf("%+v %v", got, f.calls)
			}
		})
	}
	c := candidate()
	r := reference()
	r.Note = "another note"
	c.Evidence = append(c.Evidence, r)
	if got := ValidateCandidate(context.Background(), c, goodResolver()); len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != DuplicateReference {
		t.Fatalf("%+v", got)
	}
	if got := ValidateCandidate(context.Background(), candidate(), nil); len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != ResolverMissing {
		t.Fatalf("%+v", got)
	}
	c = candidate()
	c.Intent = Proposed
	c.DeliveryOID = ""
	c.Evidence = nil
	if got := ValidateCandidate(context.Background(), c, nil); !got.Valid() {
		t.Fatalf("%+v", got)
	}
}

func TestDiagnosticOrderAndOwnership(t *testing.T) {
	c := candidate()
	c.Evidence = nil
	c.Edges = []Edge{edge("z"), {ID: "later", Intent: Proposed}, edge("a")}
	c.Edges[0].Destination.Intent = Proposed
	c.Edges[0].Evidence = nil
	c.Edges[2].Evidence = append(c.Edges[2].Evidence, reference())
	c.Edges[2].Evidence[0].Note = ""
	f := goodResolver()
	f.state = coderesolve.Stale
	got := ValidateCandidate(context.Background(), c, f)
	want := []Diagnostic{{c.Pin, "", -1, -1, MissingEvidence}, {c.Pin, "z", 0, -1, EndpointNotImplemented}, {c.Pin, "z", 0, -1, MissingEvidence}, {c.Pin, "a", 2, 0, InvalidReference}, {c.Pin, "a", 2, 1, StaleDelivery}}
	if !reflect.DeepEqual(got.Diagnostics, want) {
		t.Fatalf("got %+v want %+v", got.Diagnostics, want)
	}
	// Altering returned slices cannot change any caller-owned slice.
	got.ProposedEdges[0] = "changed"
	got.Diagnostics[0].Owner.Target = "changed"
	if c.Edges[1].ID != "later" || c.Pin.Target != "entity" {
		t.Fatal("result aliases input")
	}
}
