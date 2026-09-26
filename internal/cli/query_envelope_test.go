package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// Every query operation writes one envelope: the same schema, and the
// snapshot that identifies what it read.
func TestEveryQueryEnvelopeCarriesTheSchemaAndASnapshot(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Explain the change.", root, "implementation"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "flow"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"terms", "--saga", root, "--repo", repo},
		{"layers", "--saga", root, "--repo", repo, "--against", "main"},
		{"history", "--saga", root, "--node", "urn:change-saga:batch:persona:" + testPersona},
		{"slide", "--saga", root, "--repo", repo, "--target", "urn:change-saga:batch:slide:flow"},
		{"overview", "--saga", root, "--repo", repo},
	} {
		var out bytes.Buffer
		if err := Query(context.Background(), args, &out); err != nil {
			t.Fatalf("query %s: %v\n%s", args[0], err, out.String())
		}
		var envelope struct {
			Schema   string `json:"schema"`
			Snapshot string `json:"snapshot"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Schema != querySchema || envelope.Snapshot == "" {
			t.Errorf("query %s envelope schema=%q snapshot=%q", args[0], envelope.Schema, envelope.Snapshot)
		}
	}
}
