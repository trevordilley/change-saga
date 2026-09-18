package requirements

import "sort"

// Currency is the derived standing of one relation against current heads.
// Only CurrencyCurrent may ever count as coverage.
type Currency string

const (
	CurrencyCurrent    Currency = "current"
	CurrencyStale      Currency = "stale"
	CurrencyConflicted Currency = "conflicted"
	CurrencyInvalid    Currency = "invalid"
	CurrencySuperseded Currency = "superseded"
)

// Currency reason codes. Every non-current relation has at least one.
const (
	ReasonRevisionChanged       = "revision_changed"
	ReasonContentDigestChanged  = "content_digest_changed"
	ReasonMultipleRevisionHeads = "multiple_revision_heads"
	ReasonEndpointMissing       = "endpoint_missing"
	ReasonCriterionRemoved      = "criterion_removed"
	ReasonHeadUnresolved        = "head_unresolved"
)

// CurrencyReason explains one way a relation's pins disagree with current
// heads. Message is the legacy Relation.StaleReasons text.
type CurrencyReason struct {
	Endpoint string   `json:"endpoint"`
	Code     string   `json:"code"`
	Pinned   string   `json:"pinned,omitempty"`
	Current  []string `json:"current,omitempty"`
	Message  string   `json:"message"`
}

// RelationCurrency reports one relation's currency. Staleness is derived only
// from the relation's persisted pins and the supplied current heads, never from
// repository history.
type RelationCurrency struct {
	Relation string           `json:"relation"`
	Type     RelationType     `json:"type"`
	From     string           `json:"from"`
	To       string           `json:"to"`
	Scope    RelationScope    `json:"scope"`
	Version  int              `json:"version"`
	Status   Currency         `json:"status"`
	Reasons  []CurrencyReason `json:"reasons"`
}

// Current reports whether the relation may count as coverage.
func (currency RelationCurrency) Current() bool { return currency.Status == CurrencyCurrent }

// SetTestCaseHeads supplies the revision heads of every test case in the Saga,
// keyed by test-case ID; values are full test-case revision URNs such as
// quality.TestCase.RevisionHeads. A test-case endpoint absent from heads then
// evaluates as missing, and more than one head as conflicted.
func (inputs *StaleInputs) SetTestCaseHeads(sagaID string, heads map[string][]string) {
	if inputs.CurrentRevisions == nil {
		inputs.CurrentRevisions = map[string]string{}
	}
	if inputs.ConflictedRevisions == nil {
		inputs.ConflictedRevisions = map[string][]string{}
	}
	for id, revisions := range heads {
		urn, err := testCaseURN(sagaID, id)
		if err != nil {
			continue
		}
		if len(revisions) == 1 {
			inputs.CurrentRevisions[urn] = revisions[0]
		} else {
			inputs.ConflictedRevisions[urn] = append([]string{}, revisions...)
		}
	}
	inputs.TestCasesSupplied = true
}

// EvaluateRelations reports the currency of every relation in document order.
// Story and criterion heads come from document; everything owned outside the
// requirements domain comes from inputs.
func EvaluateRelations(document Document, inputs StaleInputs) []RelationCurrency {
	heads := newCurrencyHeads(document, inputs)
	result := make([]RelationCurrency, 0, len(document.Relations))
	for _, relation := range document.Relations {
		result = append(result, heads.evaluate(relation))
	}
	return result
}

// EvaluateRelation reports the currency of one relation.
func EvaluateRelation(document Document, relation Relation, inputs StaleInputs) RelationCurrency {
	return newCurrencyHeads(document, inputs).evaluate(relation)
}

// currencyHeads is the merged view of current heads a relation is judged by.
type currencyHeads struct {
	sagaID     string
	inputs     StaleInputs
	revisions  map[string]string
	conflicted map[string][]string
	removed    map[string]bool
}

func newCurrencyHeads(document Document, inputs StaleInputs) currencyHeads {
	heads := currencyHeads{sagaID: document.SagaID, inputs: inputs, revisions: map[string]string{}, conflicted: map[string][]string{}, removed: map[string]bool{}}
	for key, value := range inputs.CurrentRevisions {
		heads.revisions[key] = value
	}
	for key, value := range inputs.ConflictedRevisions {
		heads.conflicted[key] = value
	}
	for _, story := range document.Stories {
		storyID, _ := storyURN(document.SagaID, story.Identity.ID)
		if story.CurrentRevision == nil {
			heads.conflicted[storyID] = story.RevisionHeads
			for _, revision := range story.Revisions {
				for _, criterion := range revision.AcceptanceCriteria {
					id, _ := criterionURN(document.SagaID, story.Identity.ID, criterion.ID)
					heads.conflicted[id] = story.RevisionHeads
				}
			}
			continue
		}
		revisionID, _ := revisionURN(document.SagaID, story.Identity.ID, story.CurrentRevision.ID)
		heads.revisions[storyID] = revisionID
		currentCriteria := map[string]bool{}
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			id, _ := criterionURN(document.SagaID, story.Identity.ID, criterion.ID)
			heads.revisions[id] = revisionID
			currentCriteria[id] = true
		}
		for _, revision := range story.Revisions {
			for _, criterion := range revision.AcceptanceCriteria {
				id, _ := criterionURN(document.SagaID, story.Identity.ID, criterion.ID)
				if !currentCriteria[id] {
					heads.removed[id] = true
				}
			}
		}
	}
	return heads
}

// evaluate is the single place where relation pins are compared with heads.
func (heads currencyHeads) evaluate(relation Relation) RelationCurrency {
	urn, _ := relationURN(heads.sagaID, relation.ID)
	scope := relation.Scope
	if scope == "" {
		scope = ScopeSelf
	}
	result := RelationCurrency{
		Relation: urn, Type: relation.Type, From: relation.From, To: relation.To,
		Scope: scope, Version: relation.Version, Status: CurrencyCurrent, Reasons: []CurrencyReason{},
	}
	if relation.State != RelationActive {
		result.Status = CurrencySuperseded
		return result
	}
	for _, pin := range []struct {
		name, endpoint, revision, digest string
	}{
		{"from", relation.From, relation.FromRevision, relation.FromContentDigest},
		{"to", relation.To, relation.ToRevision, relation.ToContentDigest},
	} {
		add := func(code, message string, pinned string, current ...string) {
			result.Reasons = append(result.Reasons, CurrencyReason{Endpoint: pin.name, Code: code, Pinned: pinned, Current: current, Message: pin.name + " " + message})
		}
		if heads.inputs.Missing[pin.endpoint] {
			add(ReasonEndpointMissing, "endpoint is missing", "")
		} else if heads.removed[pin.endpoint] {
			add(ReasonCriterionRemoved, "criterion is absent from the current revision", pin.revision)
		} else if parsed, err := parseEndpoint(pin.endpoint); err == nil && parsed.Kind == endpointTestCase {
			// A test case lives outside this domain; its currency is unknown
			// rather than assumed until its heads are supplied.
			_, current := heads.revisions[pin.endpoint]
			_, conflicted := heads.conflicted[pin.endpoint]
			if !heads.inputs.TestCasesSupplied {
				add(ReasonHeadUnresolved, "test-case revision head was not supplied", pin.revision)
			} else if !current && !conflicted {
				add(ReasonEndpointMissing, "test case is missing", pin.revision)
			}
		}
		if conflicting, conflicted := heads.conflicted[pin.endpoint]; conflicted && pin.revision != "" {
			add(ReasonMultipleRevisionHeads, "endpoint has multiple revision heads", pin.revision, sortedCopy(conflicting)...)
		} else if current, known := heads.revisions[pin.endpoint]; known && pin.revision != "" && current != pin.revision {
			add(ReasonRevisionChanged, "revision changed", pin.revision, current)
		}
		if current, known := heads.inputs.CurrentContentDigests[pin.endpoint]; known && pin.digest != "" && current != pin.digest {
			add(ReasonContentDigestChanged, "content digest changed", pin.digest, current)
		}
	}
	result.Status = currencyStatus(result.Reasons)
	return result
}

// currencyStatus ranks reasons: invalid > conflicted > stale > current.
func currencyStatus(reasons []CurrencyReason) Currency {
	status := CurrencyCurrent
	for _, reason := range reasons {
		switch reason.Code {
		case ReasonEndpointMissing, ReasonCriterionRemoved, ReasonHeadUnresolved:
			return CurrencyInvalid
		case ReasonMultipleRevisionHeads:
			status = CurrencyConflicted
		default:
			if status == CurrencyCurrent {
				status = CurrencyStale
			}
		}
	}
	return status
}

func sortedCopy(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}
