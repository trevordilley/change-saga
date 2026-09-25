package server

import (
	"context"
	"errors"
	"html/template"
	"slices"
	"sync"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/semanticgraph"
)

// sagaFilesCache keeps what documentation pages read from the Saga's own
// files while none of them has changed: the narrative, the requirements, the
// test cases, and the technical inventory. Every documentation page reads all
// of them, and decoding them from a large Saga costs several times the stat
// walk that proves none of them changed.
type sagaFilesCache struct {
	mutex   sync.Mutex
	current *sagaFiles
	builds  int
}

// sagaFiles is one fingerprint of the Saga's files and what has been read
// from them. Each part is read the first time a request asks for it. The
// fingerprint is taken before anything is read, so an edit made while a part
// is read changes the next request's fingerprint and is never served from
// here. Callers only read what it holds.
type sagaFiles struct {
	root        string
	fingerprint string

	narrativeOnce sync.Once
	narrativeDoc  *saga.Saga

	recordsOnce sync.Once
	recordsID   string
	recordsDoc  requirements.Document
	recordsErr  error

	testsOnce sync.Once
	testsDoc  quality.Document
	testsErr  error

	projectedOnce sync.Once
	projectedID   string
	projectedDoc  requirements.Document
	projectedErr  error

	// mutation is the Saga's target directories, which every fragment file
	// a page embeds is looked up in.
	mutationOnce       sync.Once
	mutationIndex      saga.MutationIndex
	mutationValidation saga.Validation
	mutationErr        error

	inventoryOnce sync.Once
	inventoryID   string
	inventoryDoc  requirements.Inventory
	inventoryErr  error

	// slidesView is every embedded deck's slides as every page shows them,
	// story links decorated, and slidesHTML the deck viewer rendered from
	// it. Both are built from the narrative and records above, so they hold
	// for as long as those do. Pages only read them.
	slidesOnce sync.Once
	slidesView *sectionView
	slidesHTML template.HTML
	slidesErr  error
}

// sagaFiles is the current Saga's files. When they cannot be fingerprinted
// nothing is kept, and each part is read afresh.
func (a *app) sagaFiles(ctx context.Context) *sagaFiles {
	fingerprint, err := a.sagaState(ctx, false).documentationKey()
	if err != nil {
		return &sagaFiles{root: a.root}
	}
	a.files.mutex.Lock()
	defer a.files.mutex.Unlock()
	if a.files.current == nil || a.files.current.fingerprint != fingerprint {
		a.files.current = &sagaFiles{root: a.root, fingerprint: fingerprint}
		a.files.builds++
	}
	return a.files.current
}

// narrative is the Saga with its complete narrative, or nil when it does
// not load valid.
func (files *sagaFiles) narrative() *saga.Saga {
	files.narrativeOnce.Do(func() {
		document, validation, err := saga.LoadNarrative(files.root)
		if err == nil && validation.Valid {
			files.narrativeDoc = document
		}
	})
	return files.narrativeDoc
}

// records is the Saga's requirements. The relations are the caller's own
// copy, since pages project complete-slide links into them.
func (files *sagaFiles) records(sagaID string) (requirements.Document, error) {
	files.recordsOnce.Do(func() {
		files.recordsID = sagaID
		files.recordsDoc, files.recordsErr = requirements.Load(files.root, sagaID)
	})
	if files.recordsID != sagaID {
		return requirements.Load(files.root, sagaID)
	}
	document := files.recordsDoc
	document.Relations = slices.Clone(document.Relations)
	return document, files.recordsErr
}

// projectedRecords is the records with the complete slides' criterion links
// projected into them from this fingerprint's narrative, as every page reads
// them. The relations are the caller's own copy.
func (files *sagaFiles) projectedRecords(sagaID string) (requirements.Document, error) {
	files.projectedOnce.Do(func() {
		files.projectedID = sagaID
		files.projectedDoc, files.projectedErr = files.records(sagaID)
		if files.projectedErr != nil {
			return
		}
		document := files.narrative()
		if document == nil {
			files.projectedErr = errors.New("the narrative could not be loaded")
			return
		}
		files.projectedErr = semanticgraph.ProjectSlideCriterionLinks(document, &files.projectedDoc)
	})
	if files.projectedID != sagaID {
		return files.project(sagaID)
	}
	document := files.projectedDoc
	document.Relations = slices.Clone(document.Relations)
	return document, files.projectedErr
}

// project projects the links afresh, for a caller the kept projection does
// not serve.
func (files *sagaFiles) project(sagaID string) (requirements.Document, error) {
	document, err := files.records(sagaID)
	if err != nil {
		return document, err
	}
	narrative := files.narrative()
	if narrative == nil {
		return document, errors.New("the narrative could not be loaded")
	}
	return document, semanticgraph.ProjectSlideCriterionLinks(narrative, &document)
}

// mutation is the Saga's mutation index. Callers only read it.
func (files *sagaFiles) mutation() (saga.MutationIndex, saga.Validation, error) {
	files.mutationOnce.Do(func() {
		files.mutationIndex, files.mutationValidation, files.mutationErr = saga.LoadMutationIndex(files.root)
	})
	return files.mutationIndex, files.mutationValidation, files.mutationErr
}

func (files *sagaFiles) tests() (quality.Document, error) {
	files.testsOnce.Do(func() {
		files.testsDoc, files.testsErr = quality.Load(files.root)
	})
	return files.testsDoc, files.testsErr
}

func (files *sagaFiles) inventory(sagaID string) (requirements.Inventory, error) {
	files.inventoryOnce.Do(func() {
		files.inventoryID = sagaID
		files.inventoryDoc, files.inventoryErr = requirements.LoadInventory(files.root, sagaID)
	})
	if files.inventoryID != sagaID {
		return requirements.LoadInventory(files.root, sagaID)
	}
	return files.inventoryDoc, files.inventoryErr
}

// slides is the embedded decks' view and its rendering, built once from this
// fingerprint's narrative and records.
func (files *sagaFiles) slides(build func() (*sectionView, template.HTML, error)) (*sectionView, template.HTML, error) {
	files.slidesOnce.Do(func() {
		files.slidesView, files.slidesHTML, files.slidesErr = build()
	})
	return files.slidesView, files.slidesHTML, files.slidesErr
}
