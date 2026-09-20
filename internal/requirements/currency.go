package requirements

import (
	"sort"
	"strconv"
	"strings"
)

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
	// ReasonCriterionStatementChanged is the criterion a relation points at
	// being reworded under it: the one revision change that can invalidate
	// what the relation asserts.
	ReasonCriterionStatementChanged = "criterion_statement_changed"
	// ReasonStoryObligationChanged is the story a relation points at changing
	// what it obliges: its title, its statement, or any acceptance criterion.
	ReasonStoryObligationChanged = "story_obligation_changed"
)

// Carry-forward codes. Every carried-forward endpoint has exactly one.
const (
	// CarriedCriterionUnchanged is a story revised around a criterion whose id
	// and statement did not move.
	CarriedCriterionUnchanged = "criterion_unchanged"
	// CarriedStoryObligationUnchanged is a story revised without changing what
	// it obliges.
	CarriedStoryObligationUnchanged = "story_obligation_unchanged"
)

// CarriedForward records one endpoint whose pin the tool advanced on its own
// because what the relation points at did not change. Confirmed stays the
// revision a person pinned, so a reader can always tell an affirmed relation
// from one carried past a revision nobody read.
type CarriedForward struct {
	Endpoint  string `json:"endpoint"`
	Code      string `json:"code"`
	Confirmed string `json:"confirmed"`
	Current   string `json:"current"`
	Message   string `json:"message"`
}

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
	// CarriedForward names every endpoint whose pin the tool advanced without
	// a judgment call. It is empty on a relation nothing moved under.
	CarriedForward []CarriedForward `json:"carried_forward,omitempty"`
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
	// statements is what each criterion said in each revision of its story,
	// keyed criterion URN then revision URN. It is how a relation to a
	// criterion is judged by the criterion rather than by the story around it.
	statements map[string]map[string]string
	// obligations is what each story required in each of its revisions, keyed
	// story URN then revision URN.
	obligations map[string]map[string]string
}

func newCurrencyHeads(document Document, inputs StaleInputs) currencyHeads {
	heads := currencyHeads{sagaID: document.SagaID, inputs: inputs, revisions: map[string]string{}, conflicted: map[string][]string{}, removed: map[string]bool{}, statements: map[string]map[string]string{}, obligations: map[string]map[string]string{}}
	for key, value := range inputs.CurrentRevisions {
		heads.revisions[key] = value
	}
	for key, value := range inputs.ConflictedRevisions {
		heads.conflicted[key] = value
	}
	for _, story := range document.Stories {
		storyID, _ := storyURN(document.SagaID, story.Identity.ID)
		for _, revision := range story.Revisions {
			revisionID, err := revisionURN(document.SagaID, story.Identity.ID, revision.ID)
			if err != nil {
				continue
			}
			for _, criterion := range revision.AcceptanceCriteria {
				id, err := criterionURN(document.SagaID, story.Identity.ID, criterion.ID)
				if err != nil {
					continue
				}
				if heads.statements[id] == nil {
					heads.statements[id] = map[string]string{}
				}
				heads.statements[id][revisionID] = criterion.Statement
			}
			if heads.obligations[storyID] == nil {
				heads.obligations[storyID] = map[string]string{}
			}
			heads.obligations[storyID][revisionID] = obligationOf(revision)
		}
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
func (heads currencyHeads) evaluate(record Relation) RelationCurrency {
	// A relation is judged against what it is confirmed against, which is its
	// own pins as its appended repins have moved them.
	relation := record.Confirmed()
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
			switch code := heads.revisionChange(pin.endpoint, pin.revision, current); code {
			case CarriedCriterionUnchanged:
				result.CarriedForward = append(result.CarriedForward, CarriedForward{
					Endpoint: pin.name, Code: code, Confirmed: pin.revision, Current: current,
					Message: pin.name + " criterion statement is unchanged since the confirmed revision",
				})
			case CarriedStoryObligationUnchanged:
				result.CarriedForward = append(result.CarriedForward, CarriedForward{
					Endpoint: pin.name, Code: code, Confirmed: pin.revision, Current: current,
					Message: pin.name + " story obliges what it did at the confirmed revision",
				})
			case ReasonCriterionStatementChanged:
				add(code, "criterion statement changed", pin.revision, current)
			case ReasonStoryObligationChanged:
				add(code, "story obligation changed", pin.revision, current)
			default:
				add(ReasonRevisionChanged, "revision changed", pin.revision, current)
			}
		}
		if current, known := heads.inputs.CurrentContentDigests[pin.endpoint]; known && pin.digest != "" && current != pin.digest {
			add(ReasonContentDigestChanged, "content digest changed", pin.digest, current)
		}
	}
	result.Status = currencyStatus(result.Reasons)
	return result
}

// revisionChange says what a moved revision pin means for one endpoint. A
// relation to a criterion asserts something about that criterion alone, so a
// revision that leaves the criterion's id and statement alone carries the pin
// forward; a reworded criterion is stale, and so is anything the criterion
// index cannot speak for. The comparison is byte-for-byte on purpose:
// whitespace and typo fixes are judgment calls by definition, and staleness is
// worth something only when it fires on a call somebody has to make.
func (heads currencyHeads) revisionChange(endpoint, pinned, current string) string {
	if byRevision := heads.statements[endpoint]; byRevision != nil {
		return compareAcross(byRevision, pinned, current, CarriedCriterionUnchanged, ReasonCriterionStatementChanged)
	}
	if byRevision := heads.obligations[endpoint]; byRevision != nil {
		return compareAcross(byRevision, pinned, current, CarriedStoryObligationUnchanged, ReasonStoryObligationChanged)
	}
	return ReasonRevisionChanged
}

func compareAcross(byRevision map[string]string, pinned, current, unchanged, changed string) string {
	was, knewPinned := byRevision[pinned]
	now, knewCurrent := byRevision[current]
	switch {
	case !knewPinned || !knewCurrent:
		return ReasonRevisionChanged
	case was == now:
		return unchanged
	default:
		return changed
	}
}

// obligationOf renders what one story revision requires: its title, its
// statement, and its acceptance criteria in order. Priority, personas, and
// citations say how urgent the story is, who it serves, and where it came
// from; none of them changes what a design must address, a slide must explain,
// or a test must verify, so none of them ages a relation.
func obligationOf(revision Revision) string {
	parts := []string{strconv.Quote(revision.Title), strconv.Quote(revision.Statement)}
	for _, criterion := range revision.AcceptanceCriteria {
		parts = append(parts, strconv.Quote(criterion.ID), strconv.Quote(criterion.Statement))
	}
	return strings.Join(parts, " ")
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
