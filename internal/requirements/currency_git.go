package requirements

import (
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/currentrecord"
)

// This cache belongs to one read snapshot. Earlier content is retrieved only
// from Git by its exact digest; no content-version ledger is stored in Saga.
// Missing Git content leaves the evidence stale rather than inferring equality.
type storyCurrencyHistory struct {
	source      *currentrecord.GitSource
	legacy      map[string][]Revision
	pins        *currentrecord.Pins
	pinsChecked bool
	root        string
	paths       map[string]string
	mu          sync.Mutex
	checked     map[string]bool
	values      map[string]*Revision
}

func newStoryCurrencyHistory(document Document) *storyCurrencyHistory {
	history := &storyCurrencyHistory{root: document.Root, paths: map[string]string{}, legacy: map[string][]Revision{}, checked: map[string]bool{}, values: map[string]*Revision{}}
	history.source = document.gitSource
	for _, story := range document.Stories {
		urn, _ := storyURN(document.SagaID, story.Identity.ID)
		history.paths[urn] = storyPackagePath(story.Feature, story.Identity.ID)
		if !story.current {
			history.legacy[urn] = story.Revisions
		}
	}
	return history
}

func (history *storyCurrencyHistory) pinned(endpoint, pin string) *Revision {
	if history == nil || !currentrecord.ValidDigest(pin) {
		return nil
	}
	story := endpoint
	if at := strings.Index(story, ":criterion:"); at >= 0 {
		story = story[:at]
	}
	path := history.paths[story]
	if path == "" {
		return nil
	}
	key := story + "\x00" + pin
	history.mu.Lock()
	defer history.mu.Unlock()
	if history.checked[key] {
		return history.values[key]
	}
	history.checked[key] = true
	if len(history.legacy[story]) > 0 {
		if !history.pinsChecked {
			history.pinsChecked = true
			history.pins, _ = currentrecord.AuditPins(history.root)
		}
		if history.pins != nil {
			for _, value := range history.legacy[story] {
				digest, err := history.pins.Resolve(ContentPin(story, value.ID))
				if err == nil && digest == pin {
					copy := value
					history.values[key] = &copy
					return &copy
				}
			}
		}
	}
	var saved currentrecord.GitContent
	var err error
	if history.source != nil {
		saved, err = currentrecord.FindGitContentAt(*history.source, path, pin)
	} else {
		saved, err = currentrecord.FindGitContent(history.root, path, pin)
	}
	if err != nil {
		return nil
	}
	value := Revision{Schema: RevisionSchemaURL, Version: Version, ID: "sha256-" + strings.TrimPrefix(pin, "sha256:"), Parents: []string{}, CreatedAt: time.Unix(1, 0).UTC()}
	if err := currentrecord.Project(saved.Content, &value); err != nil || value.Story != story {
		return nil
	}
	parts := strings.Split(story, ":")
	if len(parts) != 5 || validateRevision(value, parts[2], parts[4]) != nil {
		return nil
	}
	history.values[key] = &value
	return &value
}

// A stable criterion absent from an existing current story is removed even
// when the old definition is now available only through Git.
func (heads currencyHeads) criterionMissing(endpoint string) bool {
	at := strings.Index(endpoint, ":criterion:")
	if at < 0 {
		return false
	}
	_, storyCurrent := heads.revisions[endpoint[:at]]
	_, criterionCurrent := heads.revisions[endpoint]
	return storyCurrent && !criterionCurrent
}
