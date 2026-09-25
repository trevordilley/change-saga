package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// reviewCoverageCache keeps each review's coverage by exactly what it reads:
// the two commits of its range, the repository they are read as, and the
// code its deck's Items reference. Commits never change, so an entry is
// right for as long as the review's Items are; a range that moves, or an Item
// that is edited, is a new key.
type reviewCoverageCache struct {
	mutex   sync.Mutex
	entries map[string]*reviewstate.Coverage
	order   []string
	// reads counts coverages computed rather than found.
	reads int
}

// reviewCoverageLimit bounds the entries kept: a few per review is plenty,
// and an old range is only asked for again when its head moves back.
const reviewCoverageLimit = 128

func (a *app) reviewCoverage(ctx context.Context, review *saga.Review, rng reviewstate.Range, repository string, resolver *coderesolve.Resolver) (*reviewstate.Coverage, error) {
	key, err := reviewCoverageKey(review, rng, repository)
	if err != nil {
		return reviewstate.ReadCoverage(ctx, review, rng, a.sourceDir, repository, resolver)
	}
	cache := &a.reviewCoverages
	cache.mutex.Lock()
	covered, ok := cache.entries[key]
	cache.mutex.Unlock()
	if ok {
		return covered, nil
	}
	covered, err = reviewstate.ReadCoverage(ctx, review, rng, a.sourceDir, repository, resolver)
	if err != nil {
		return nil, err
	}
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.reads++
	if cache.entries == nil {
		cache.entries = map[string]*reviewstate.Coverage{}
	}
	if _, present := cache.entries[key]; !present {
		cache.order = append(cache.order, key)
		for len(cache.order) > reviewCoverageLimit {
			delete(cache.entries, cache.order[0])
			cache.order = cache.order[1:]
		}
	}
	cache.entries[key] = covered
	return covered, nil
}

// reviewCoverageKey names what one review's coverage reads.
func reviewCoverageKey(review *saga.Review, rng reviewstate.Range, repository string) (string, error) {
	type item struct {
		Target string          `json:"target"`
		Code   []saga.CodeFile `json:"code"`
	}
	var items []item
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			for _, one := range slide.Items {
				items = append(items, item{Target: one.Target, Code: one.Code})
			}
		}
	}
	encoded, err := json.Marshal(struct {
		Base, Head, Repository string
		Items                  []item
	}{rng.BaseOID, rng.HeadOID, repository, items})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
