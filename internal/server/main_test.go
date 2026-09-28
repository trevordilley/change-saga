package server

import (
	"os"
	"testing"

	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

// TestMain keeps the caches a served Saga writes, the snapshot cache and the
// review ledger, out of the user's real cache directory.
func TestMain(m *testing.M) {
	cache, err := os.MkdirTemp("", "change-saga-server-cache-")
	if err != nil {
		panic(err)
	}
	os.Setenv(snapshotcache.DirEnv, cache)
	code := m.Run()
	_ = os.RemoveAll(cache)
	os.Exit(code)
}
