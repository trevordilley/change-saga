package qualityid

import "testing"

func TestQualityURNRoundTrips(t *testing.T) {
	tests := []struct {
		name string
		ref  Reference
		want string
	}{
		{"test case", Reference{SagaID: "checkout", Kind: KindTestCase, ID: "deadline"}, "urn:change-saga:checkout:test-case:deadline"},
		{"revision", Reference{SagaID: "checkout", Kind: KindRevision, TestCaseID: "deadline", ID: "r1"}, "urn:change-saga:checkout:test-case:deadline:revision:r1"},
		{"step", Reference{SagaID: "checkout", Kind: KindStep, TestCaseID: "deadline", ID: "submit"}, "urn:change-saga:checkout:test-case:deadline:step:submit"},
		{"event", Reference{SagaID: "checkout", Kind: KindEvent, TestCaseID: "deadline", ID: "active"}, "urn:change-saga:checkout:test-case:deadline:event:active"},
		{"evidence", Reference{SagaID: "checkout", Kind: KindEvidence, TestCaseID: "deadline", ID: "test-code"}, "urn:change-saga:checkout:test-case:deadline:evidence:test-code"},
		{"run", Reference{SagaID: "checkout", Kind: KindRun, TestCaseID: "deadline", ID: "ci-1"}, "urn:change-saga:checkout:test-case:deadline:run:ci-1"},
		{"policy", Reference{SagaID: "checkout", Kind: KindQualityPolicy, ID: "cutoff"}, "urn:change-saga:checkout:quality-policy:cutoff"},
		{"exception", Reference{SagaID: "checkout", Kind: KindCoverageException, ID: "manual"}, "urn:change-saga:checkout:coverage-exception:manual"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Build(test.ref)
			if err != nil || got != test.want {
				t.Fatalf("Build() = %q, %v; want %q", got, err, test.want)
			}
			parsed, err := Parse(got)
			if err != nil || parsed != test.ref {
				t.Fatalf("Parse() = %#v, %v; want %#v", parsed, err, test.ref)
			}
		})
	}
}

func TestQualityURNRejectsMalformedAndNoncanonicalValues(t *testing.T) {
	values := []string{
		"urn:change-saga:checkout:test-case:deadline:unknown:x",
		"urn:change-saga:checkout:story:deadline:event:active",
		"urn:change-saga:checkout:test-case:bad/id",
		"urn:change-saga:checkout:test-case:deadline:revision:",
		"URN:change-saga:checkout:test-case:deadline",
	}
	for _, value := range values {
		if _, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", value)
		}
	}
	if _, err := Build(Reference{SagaID: "checkout", Kind: KindTestCase, ID: "deadline", TestCaseID: "parent"}); err == nil {
		t.Fatal("top-level resource with parent unexpectedly succeeded")
	}
}
