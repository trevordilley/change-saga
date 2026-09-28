package server

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/theme"
)

// A slide that paints with fixed colours, such as a hand-authored SVG or
// HTML page or a raster image, cannot follow dark mode, and a transparent one
// would put dark ink on the dark page. It sits on a light paper card in dark
// mode instead, its frame kept in the light scheme, so it reads as paper
// rather than glare. A visual that switches on prefers-color-scheme, as every
// generated diagram does, or paints with a contract token, which every served
// visual declares for both schemes, follows the app theme.

// paperEntry caches one file's answer until the file changes.
type paperEntry struct {
	modified time.Time
	size     int64
	paper    bool
}

var paperCache sync.Map // path -> paperEntry

// visualPaper reports whether the visual at entrypoint in directory, of
// mediaType, needs the paper card in dark mode.
func visualPaper(mediaType, directory, entrypoint string) bool {
	if mediaType != "image/svg+xml" && mediaType != "text/html" {
		return strings.HasPrefix(mediaType, "image/")
	}
	path := filepath.Join(directory, filepath.FromSlash(entrypoint))
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	if cached, ok := paperCache.Load(path); ok {
		if entry := cached.(paperEntry); entry.modified.Equal(info.ModTime()) && entry.size == info.Size() {
			return entry.paper
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	paper := !bytes.Contains(data, []byte("prefers-color-scheme")) && !usesToken(data)
	paperCache.Store(path, paperEntry{modified: info.ModTime(), size: info.Size(), paper: paper})
	return paper
}

var tokenUse = regexp.MustCompile(`var\(\s*--([a-z][a-z0-9-]*)`)

// usesToken reports whether data reads any token of the contract.
func usesToken(data []byte) bool {
	for _, match := range tokenUse.FindAllSubmatch(data, -1) {
		if _, ok := theme.Lookup(string(match[1])); ok {
			return true
		}
	}
	return false
}

// fragmentPaper is visualPaper for a fragment's or slide's entrypoint.
func fragmentPaper(fragment *saga.Fragment) bool {
	if fragment == nil {
		return false
	}
	return visualPaper(fragment.MediaType, fragment.Directory, fragment.Entrypoint)
}
