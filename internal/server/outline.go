package server

import (
	"context"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/saga"
)

// outlineCache is intentionally independent from snapshotCache. Its input set
// excludes coverage and source-comparison data, so adding a mapping or changing
// a source commit cannot force the root request onto the expensive path.
type outlineCache struct {
	mutex       sync.Mutex
	fingerprint string
	document    *saga.Saga
	builds      int
}

func (a *app) outlineDocument(ctx context.Context) *saga.Saga {
	// The key is taken before the Saga is read, so an edit made while it is
	// read changes the next request's key and is never served from here.
	fingerprint, err := a.sagaState(ctx, false).outlineKey()
	if err != nil {
		return nil
	}
	a.outline.mutex.Lock()
	defer a.outline.mutex.Unlock()
	if a.outline.document != nil && fingerprint == a.outline.fingerprint {
		return a.outline.document
	}
	document, validation, err := saga.LoadOutline(a.root)
	if err != nil || !validation.Valid {
		return nil
	}
	a.outline.builds++
	a.outline.fingerprint, a.outline.document = fingerprint, document
	return document
}

// The outline's files are what sagaFingerprints commits to as the outline.
// In particular, ___code directories and fragment bodies are skipped as
// directories/files rather than merely omitted from the digest after a full
// walk. A root request therefore does not scale with per-line evidence or code
// attachment size even during freshness checks.
func skipOutlineDirectory(rel, base string) bool {
	switch base {
	// Reviews are read fresh by the review pages; the shell never shows them.
	case saga.CodeDirName, saga.ReviewsDir, "___claims", "___verifications", "___landmarks":
		return true
	}
	// Fragment packages may contain arbitrarily large asset trees. The outline
	// reads only fragment.json from them.
	parts := strings.Split(rel, "/")
	for index := 0; index < len(parts)-1; index++ {
		if strings.HasSuffix(parts[index], ".fragment") && !strings.HasPrefix(base, "___") {
			return true
		}
	}
	return false
}

func outlineFile(rel, base string) bool {
	switch base {
	case "saga.json", "chapter.json", "section.json", "fragment.json":
		return true
	}
	if strings.HasPrefix(rel, saga.EmbeddedSlidesDir+"/") && strings.HasSuffix(base, ".json") {
		return strings.HasPrefix(base, "10-d-") || strings.HasPrefix(base, "20-s-") || strings.HasPrefix(base, "30-i-")
	}
	return strings.Contains(rel, "/events/")
}
