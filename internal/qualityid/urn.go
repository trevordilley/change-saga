// Package qualityid builds and parses canonical v5 quality-domain URNs.
// It is deliberately independent of loaders and mutation packages so it can
// be reused by composition code without creating dependency cycles.
package qualityid

import (
	"fmt"
	"regexp"
	"strings"
)

const prefix = "urn:change-saga:"

// Kind identifies a quality-domain resource class.
type Kind string

const (
	KindTestCase          Kind = "test-case"
	KindRevision          Kind = "revision"
	KindStep              Kind = "step"
	KindEvent             Kind = "event"
	KindEvidence          Kind = "evidence"
	KindRun               Kind = "run"
	KindQualityPolicy     Kind = "quality-policy"
	KindCoverageException Kind = "coverage-exception"
)

// Reference is the structured form of a quality-domain URN. TestCaseID is set
// for resources nested beneath a test case and empty for top-level resources.
type Reference struct {
	SagaID     string
	Kind       Kind
	ID         string
	TestCaseID string
}

var stableID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidID reports whether value follows the stable resource-ID grammar.
func ValidID(value string) bool { return stableID.MatchString(value) }

// Build returns the canonical URN for reference.
func Build(reference Reference) (string, error) {
	if err := validateID("saga", reference.SagaID); err != nil {
		return "", err
	}
	if err := validateID(string(reference.Kind), reference.ID); err != nil {
		return "", err
	}
	base := prefix + reference.SagaID + ":"
	switch reference.Kind {
	case KindTestCase:
		if reference.TestCaseID != "" {
			return "", fmt.Errorf("test-case reference cannot have a parent")
		}
		return base + "test-case:" + reference.ID, nil
	case KindQualityPolicy, KindCoverageException:
		if reference.TestCaseID != "" {
			return "", fmt.Errorf("%s reference cannot have a parent", reference.Kind)
		}
		return base + string(reference.Kind) + ":" + reference.ID, nil
	case KindRevision, KindStep, KindEvent, KindEvidence, KindRun:
		if err := validateID("test-case", reference.TestCaseID); err != nil {
			return "", err
		}
		return base + "test-case:" + reference.TestCaseID + ":" + string(reference.Kind) + ":" + reference.ID, nil
	default:
		return "", fmt.Errorf("unsupported quality resource kind %q", reference.Kind)
	}
}

// Parse returns the structured form of a canonical quality-domain URN.
func Parse(value string) (Reference, error) {
	if !strings.HasPrefix(value, prefix) {
		return Reference{}, fmt.Errorf("URN must begin with %q", prefix)
	}
	parts := strings.Split(value, ":")
	var reference Reference
	switch len(parts) {
	case 5:
		reference = Reference{SagaID: parts[2], Kind: Kind(parts[3]), ID: parts[4]}
		if reference.Kind != KindTestCase && reference.Kind != KindQualityPolicy && reference.Kind != KindCoverageException {
			return Reference{}, fmt.Errorf("URN has an unsupported quality resource shape")
		}
	case 7:
		if parts[3] != string(KindTestCase) {
			return Reference{}, fmt.Errorf("nested quality resource must belong to a test case")
		}
		reference = Reference{SagaID: parts[2], TestCaseID: parts[4], Kind: Kind(parts[5]), ID: parts[6]}
		switch reference.Kind {
		case KindRevision, KindStep, KindEvent, KindEvidence, KindRun:
		default:
			return Reference{}, fmt.Errorf("URN has an unsupported nested quality resource kind")
		}
	default:
		return Reference{}, fmt.Errorf("URN has an unsupported quality resource shape")
	}
	canonical, err := Build(reference)
	if err != nil {
		return Reference{}, fmt.Errorf("invalid quality URN: %w", err)
	}
	if canonical != value {
		return Reference{}, fmt.Errorf("URN is not canonical; canonical form is %s", canonical)
	}
	return reference, nil
}

func validateID(name, value string) error {
	if !ValidID(value) {
		return fmt.Errorf("%s ID must match %s", name, stableID.String())
	}
	return nil
}

func topLevel(sagaID string, kind Kind, id string) (string, error) {
	return Build(Reference{SagaID: sagaID, Kind: kind, ID: id})
}

func nested(sagaID, testCaseID string, kind Kind, id string) (string, error) {
	return Build(Reference{SagaID: sagaID, Kind: kind, TestCaseID: testCaseID, ID: id})
}

func TestCase(sagaID, testCaseID string) (string, error) {
	return topLevel(sagaID, KindTestCase, testCaseID)
}

func Revision(sagaID, testCaseID, revisionID string) (string, error) {
	return nested(sagaID, testCaseID, KindRevision, revisionID)
}

func Step(sagaID, testCaseID, stepID string) (string, error) {
	return nested(sagaID, testCaseID, KindStep, stepID)
}

func Event(sagaID, testCaseID, eventID string) (string, error) {
	return nested(sagaID, testCaseID, KindEvent, eventID)
}

func Evidence(sagaID, testCaseID, evidenceID string) (string, error) {
	return nested(sagaID, testCaseID, KindEvidence, evidenceID)
}

func Run(sagaID, testCaseID, runID string) (string, error) {
	return nested(sagaID, testCaseID, KindRun, runID)
}

func QualityPolicy(sagaID, policyID string) (string, error) {
	return topLevel(sagaID, KindQualityPolicy, policyID)
}

func CoverageException(sagaID, exceptionID string) (string, error) {
	return topLevel(sagaID, KindCoverageException, exceptionID)
}
