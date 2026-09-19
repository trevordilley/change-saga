package livingapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagaref"
	"github.com/twentyideas/changesaga/internal/workplan"
)

type session struct {
	snapshot           string
	sourceHeadIdentity string
	sourceHeadCommit   string
	requirements       requirements.Document
	plan               workplan.Plan
	saga               *saga.Saga
	adopted            bool
}

func Open(_ context.Context, options OpenOptions) (Session, error) {
	if strings.TrimSpace(options.SagaRoot) == "" {
		return nil, appError(CodeInvalidArgument, "saga_root is required", false, nil, nil)
	}
	root, err := filepath.Abs(options.SagaRoot)
	if err != nil {
		return nil, appError(CodeInvalidArgument, "saga_root could not be resolved", false, nil, err)
	}
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, appError(CodeNotFound, "saga was not found", false, map[string]any{"kind": "saga"}, err)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, appError(CodeInvalidSaga, "the saga root could not be resolved safely", false, nil, err)
	}

	doc, sagaValidation, err := saga.Load(root)
	if err != nil {
		return nil, appError(CodeInvalidSaga, "the saga could not be loaded", false, nil, err)
	}
	if !sagaValidation.Valid {
		return nil, appError(CodeInvalidSaga, "the saga is invalid", false, map[string]any{"issues": sanitizeSagaIssues(sagaValidation.Issues)}, nil)
	}
	snapshot := options.Snapshot
	if snapshot == "" {
		snapshot, err = snapshotTree(root)
		if err != nil {
			return nil, appError(CodeInternal, "the session snapshot could not be created", false, nil, err)
		}
	}
	adopted := requirementsAdopted(root)
	testCases, err := testCaseHeads(root)
	if err != nil {
		return nil, appError(CodeInvalidSaga, "the quality records could not be loaded", false, nil, err)
	}
	graph, err := loadLivingGraph(root, doc, testCases)
	if err != nil {
		return nil, err
	}
	return &session{snapshot: snapshot, sourceHeadIdentity: options.SourceHeadIdentity, sourceHeadCommit: options.SourceHeadCommit, requirements: graph.requirements, plan: graph.plan, saga: doc, adopted: adopted}, nil
}

func livingRootPresent(root, name string) bool {
	info, err := os.Lstat(filepath.Join(root, name))
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

// crossDomainInputs adds the facts requirements cannot derive without
// importing the work-plan and design domains: which relation endpoints no
// longer exist, and which work items have competing revision heads. They are
// inputs to requirements.EvaluateRelations, the one place relation pins and
// endpoints are judged; nothing here compares a pin.
func crossDomainInputs(document requirements.Document, plan workplan.Plan, doc *saga.Saga, designDigests map[string]string, inputs *requirements.StaleInputs) {
	targets := saga.MutationIndexFromDocument(doc).Targets
	claims := map[string]bool{}
	for _, claim := range doc.Claims {
		claims["urn:change-saga:"+document.SagaID+":claim:"+claim.ID] = true
	}
	verifications := map[string]bool{}
	for _, verification := range doc.Verifications {
		verifications["urn:change-saga:"+document.SagaID+":verification:"+verification.ID] = true
	}
	if inputs.Missing == nil {
		inputs.Missing = map[string]bool{}
	}
	if inputs.ConflictedRevisions == nil {
		inputs.ConflictedRevisions = map[string][]string{}
	}
	for _, relation := range document.Relations {
		if relation.State != requirements.RelationActive {
			continue
		}
		for _, endpoint := range []string{relation.From, relation.To} {
			if ref, err := livingid.Parse(endpoint); err == nil {
				switch ref.Kind {
				case livingid.KindWorkItem:
					item := plan.WorkItems[ref.ID]
					if item == nil {
						inputs.Missing[endpoint] = true
					} else if len(item.Heads) > 1 {
						inputs.ConflictedRevisions[endpoint] = copyStrings(item.Heads)
					}
				case livingid.KindDesign:
					if _, exists := designDigests[endpoint]; !exists {
						inputs.Missing[endpoint] = true
					}
				}
				continue
			}
			if target, err := sagaref.ParseTarget(endpoint); err == nil {
				switch target.Kind {
				case sagaref.TargetDeck, sagaref.TargetSlide, sagaref.TargetItem:
					if _, exists := targets[endpoint]; !exists {
						inputs.Missing[endpoint] = true
					}
					continue
				}
			}
			if strings.Contains(endpoint, ":claim:") && !claims[endpoint] {
				inputs.Missing[endpoint] = true
			}
			if strings.Contains(endpoint, ":verification:") && !verifications[endpoint] {
				inputs.Missing[endpoint] = true
			}
		}
	}
}

func (s *session) Snapshot() string { return s.snapshot }

func (s *session) Query(_ context.Context, query Query) (Result, error) {
	var rows any
	var key string
	switch query.Operation {
	case "requirements":
		values := s.requirementRows(query.Filters)
		rows, key = values, "requirements"
	case "requirement-history":
		values, err := s.historyRows(query.Filters)
		if err != nil {
			return Result{}, err
		}
		rows, key = values, "events"
	case "citations":
		values := s.citationRows(query.Filters)
		rows, key = values, "citations"
	case "relations":
		values := s.relationRows(query.Filters)
		rows, key = values, "relations"
	case "waves":
		values := s.waveRows(query.Filters)
		rows, key = values, "waves"
	case "work-items":
		values := s.workItemRows(query.Filters)
		rows, key = values, "items"
	case "work-events":
		values := s.workEventRows(query.Filters)
		rows, key = values, "events"
	case "work-conflicts":
		values := s.conflictRows(query.Filters)
		rows, key = values, "conflicts"
	case "traceability":
		return s.traceabilityResult(query)
	case "readiness":
		return s.readinessResult(query)
	default:
		return Result{}, appError(CodeInvalidArgument, "unknown living query operation", false, nil, nil)
	}
	return s.pageRows(query, key, rows)
}

func (s *session) pageRows(query Query, key string, rows any) (Result, error) {
	total := sliceLength(rows)
	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, total)
	if err != nil {
		return Result{}, err
	}
	sliced := sliceRange(rows, start, end)
	var data any
	switch key {
	case "requirements":
		data = RequirementPage{Requirements: sliced.([]Requirement)}
	case "events":
		if query.Operation == "requirement-history" {
			data = HistoryPage{Events: sliced.([]HistoryEvent)}
		} else {
			data = WorkEventPage{Events: sliced.([]WorkEvent)}
		}
	case "citations":
		data = CitationPage{Citations: sliced.([]Citation)}
	case "relations":
		data = RelationPage{Relations: sliced.([]Relation)}
	case "waves":
		data = WavePage{Waves: sliced.([]Wave)}
	case "items":
		data = WorkItemPage{Items: sliced.([]WorkItem)}
	case "conflicts":
		data = ConflictPage{Conflicts: sliced.([]Conflict)}
	}
	return Result{Data: data, Page: page}, nil
}

func (s *session) readinessResult(query Query) (Result, error) {
	pageData, all := s.readinessRows(query.Filters)
	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, len(all))
	if err != nil {
		return Result{}, err
	}
	pageData.Requirements = all[start:end]
	return Result{Data: pageData, Page: page}, nil
}

func (s *session) traceabilityResult(query Query) (Result, error) {
	if query.Filters.Commit != "" && s.sourceHeadCommit == "" {
		return Result{}, appError(CodeInvalidArgument, "commit lookup requires a committed source comparison", false, nil, nil)
	}
	rows, _ := s.traceRows(query.Filters)
	unlinked := s.unlinkedReviewEvidence(query.Filters)
	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, len(rows))
	if err != nil {
		return Result{}, err
	}
	return Result{Data: TraceabilityPage{Criteria: rows[start:end], UnlinkedCodeEvidence: unlinked}, Page: page}, nil
}

type cursorToken struct {
	Version   int    `json:"v"`
	Operation string `json:"op"`
	Key       string `json:"key"`
	Snapshot  string `json:"snapshot"`
	Offset    int    `json:"offset"`
	Checksum  string `json:"checksum"`
}

func (s *session) page(operation, key, cursor string, limit, total int) (int, int, Page, error) {
	if limit == 0 {
		limit = DefaultPageLimit
	}
	if limit < 1 || limit > MaxPageLimit {
		return 0, 0, Page{}, appError(CodeInvalidArgument, fmt.Sprintf("limit must be between 1 and %d", MaxPageLimit), false, nil, nil)
	}
	start := 0
	if cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return 0, 0, Page{}, invalidCursor()
		}
		var token cursorToken
		decodeErr := json.Unmarshal(data, &token)
		canonical, marshalErr := json.Marshal(token)
		if decodeErr != nil || marshalErr != nil || !bytes.Equal(data, canonical) || len(token.Checksum) != sha256.Size*2 ||
			subtle.ConstantTimeCompare([]byte(token.Checksum), []byte(cursorChecksum(token))) != 1 || token.Version != 1 || token.Operation != operation || token.Key != key {
			return 0, 0, Page{}, invalidCursor()
		}
		if token.Snapshot != s.snapshot {
			return 0, 0, Page{}, appError(CodeStaleSnapshot, "the cursor belongs to a different snapshot", true, map[string]any{"expected": token.Snapshot, "actual": s.snapshot}, nil)
		}
		if token.Offset < 0 || token.Offset > total {
			return 0, 0, Page{}, invalidCursor()
		}
		start = token.Offset
	}
	end := start + limit
	if end > total {
		end = total
	}
	page := Page{Total: total, Returned: end - start, HasMore: end < total}
	if page.HasMore {
		token := cursorToken{Version: 1, Operation: operation, Key: key, Snapshot: s.snapshot, Offset: end}
		token.Checksum = cursorChecksum(token)
		data, _ := json.Marshal(token)
		next := base64.RawURLEncoding.EncodeToString(data)
		page.NextCursor = &next
	}
	return start, end, page, nil
}

func cursorChecksum(token cursorToken) string {
	token.Checksum = ""
	data, _ := json.Marshal(token)
	digest := sha256.Sum256(append([]byte("change-saga-living-cursor-v1\x00"), data...))
	return hex.EncodeToString(digest[:])
}

func invalidCursor() error {
	return appError(CodeInvalidArgument, "cursor does not apply to this query", false, nil, nil)
}
func normalizedQueryKey(filters Filters) string {
	data, _ := json.Marshal(filters)
	return string(data)
}

func snapshotTree(root string) (string, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("change-saga-livingapp-v1\x00"))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(rel)))
		_, _ = hash.Write([]byte{0})
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in saga")
		}
		if entry.IsDir() {
			_, _ = hash.Write([]byte("dir\x00"))
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file in saga")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = hash.Write([]byte("file\x00"))
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func appError(code ErrorCode, message string, retryable bool, details map[string]any, cause error) error {
	return &Error{Code: code, Message: message, Retryable: retryable, Details: details, cause: cause}
}

func sanitizePlanIssues(issues []workplan.Issue) []map[string]string {
	result := make([]map[string]string, 0, len(issues))
	for _, issue := range issues {
		result = append(result, map[string]string{"severity": issue.Severity, "path": filepath.ToSlash(issue.Path), "message": issue.Message})
	}
	return result
}
func sanitizeSagaIssues(issues []saga.Issue) []map[string]string {
	result := make([]map[string]string, 0, len(issues))
	for _, issue := range issues {
		result = append(result, map[string]string{"severity": issue.Severity, "path": filepath.ToSlash(issue.Path), "message": issue.Message})
	}
	return result
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func sliceLength(rows any) int {
	switch v := rows.(type) {
	case []Requirement:
		return len(v)
	case []HistoryEvent:
		return len(v)
	case []Citation:
		return len(v)
	case []Relation:
		return len(v)
	case []Wave:
		return len(v)
	case []WorkItem:
		return len(v)
	case []WorkEvent:
		return len(v)
	case []Conflict:
		return len(v)
	case []Traceability:
		return len(v)
	}
	return 0
}
func sliceRange(rows any, start, end int) any {
	switch v := rows.(type) {
	case []Requirement:
		return v[start:end]
	case []HistoryEvent:
		return v[start:end]
	case []Citation:
		return v[start:end]
	case []Relation:
		return v[start:end]
	case []Wave:
		return v[start:end]
	case []WorkItem:
		return v[start:end]
	case []WorkEvent:
		return v[start:end]
	case []Conflict:
		return v[start:end]
	case []Traceability:
		return v[start:end]
	}
	return nil
}
