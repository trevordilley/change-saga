package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnsureDirWithinRejectsReservedSymlinkBeforeSideEffect(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "test.saga")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "___review")); err != nil {
		t.Fatal(err)
	}

	_, err := EnsureDirWithin(root, filepath.Join(root, "___review", "threads", "new"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("EnsureDirWithin error = %v, want symlink rejection", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected symlink created %d outside entries, want zero", len(entries))
	}
}

func TestWriteFileIsAtomicAndCleansTemporaryFileOnFault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	faultHook = func(step, _ string) error {
		if step == "before-commit" {
			return errors.New("injected commit failure")
		}
		return nil
	}
	t.Cleanup(func() { faultHook = nil })

	if err := WriteFile(path, []byte("new\n"), 0o644, false); err == nil {
		t.Fatal("WriteFile succeeded despite injected failure")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("destination changed on failed write: %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "record.json" {
		t.Fatalf("temporary state remained after failure: %v", entries)
	}
}

func TestWriteFileExclusiveDoesNotReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("second\n"), 0o644, true); err == nil {
		t.Fatal("exclusive write replaced an existing record")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first\n" {
		t.Fatalf("existing record changed: data=%q err=%v", data, err)
	}
}

func TestCommitDirPublishesCompleteEntityOrNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	parent := filepath.Join(root, "___review", "threads")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(parent, "one.thread")
	faultHook = func(step, _ string) error {
		if step == "before-stage-commit" {
			return errors.New("injected stage failure")
		}
		return nil
	}
	if err := CommitDir(root, final, func(stage string) error {
		return WriteFile(filepath.Join(stage, "thread.json"), []byte("{}\n"), 0o644, true)
	}); err == nil {
		t.Fatal("CommitDir succeeded despite injected failure")
	}
	faultHook = nil
	t.Cleanup(func() { faultHook = nil })
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial final entity exists: %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging state remained: entries=%v err=%v", entries, err)
	}

	if err := CommitDir(root, final, func(stage string) error {
		return WriteFile(filepath.Join(stage, "thread.json"), []byte("{}\n"), 0o644, true)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(final, "thread.json")); err != nil {
		t.Fatalf("complete entity was not published: %v", err)
	}
}

func TestSagaLockSerializesConcurrentWritersAndTimesOut(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithSagaLock(root, time.Second, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	start := time.Now()
	err := WithSagaLock(root, 40*time.Millisecond, func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("contending lock error = %v, want timeout", err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("lock timeout returned before its bound")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	var active, maximum int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WithSagaLock(root, time.Second, func() error {
				mu.Lock()
				active++
				if active > maximum {
					maximum = active
				}
				mu.Unlock()
				time.Sleep(time.Millisecond)
				mu.Lock()
				active--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("WithSagaLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if maximum != 1 {
		t.Fatalf("maximum concurrent writers = %d, want 1", maximum)
	}
}

func TestWriteBatchRollsBackPublishedFilesOnFailure(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.json")
	created := filepath.Join(dir, "created.json")
	if err := os.WriteFile(existing, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	faultHook = func(point, path string) error {
		if point == "before-batch-commit" && path == existing {
			return errors.New("injected batch failure")
		}
		return nil
	}
	t.Cleanup(func() { faultHook = nil })
	err := WriteBatch([]WriteBatchEntry{
		{Path: created, Data: []byte("new\n"), Exclusive: true},
		{Path: existing, Data: []byte("changed\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "injected batch failure") {
		t.Fatalf("batch error = %v", err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("created file survived rollback: %v", err)
	}
	if got, err := os.ReadFile(existing); err != nil || string(got) != "old\n" {
		t.Fatalf("existing file after rollback = %q, %v", got, err)
	}
}

func TestWriteBatchReportsRollbackFailureAndRetainsRecoveryContent(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.json")
	blocked := filepath.Join(dir, "blocked.json")
	if err := os.WriteFile(existing, []byte("old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	faultHook = func(point, path string) error {
		switch {
		case point == "before-batch-commit" && path == blocked:
			return errors.New("injected publish failure")
		case point == "before-batch-rollback" && path == existing:
			return errors.New("injected restore failure")
		default:
			return nil
		}
	}
	t.Cleanup(func() { faultHook = nil })

	err := WriteBatch([]WriteBatchEntry{
		{Path: existing, Data: []byte("new\n")},
		{Path: blocked, Data: []byte("created\n"), Exclusive: true},
	})
	if err == nil || !strings.Contains(err.Error(), "injected publish failure") || !strings.Contains(err.Error(), "injected restore failure") {
		t.Fatalf("joined batch error = %v", err)
	}
	marker := "recovery content retained at "
	position := strings.Index(err.Error(), marker)
	if position < 0 {
		t.Fatalf("batch error omits recovery path: %v", err)
	}
	recovery := strings.TrimSpace(err.Error()[position+len(marker):])
	if got, readErr := os.ReadFile(recovery); readErr != nil || string(got) != "old\n" {
		t.Fatalf("recovery content = %q, %v; error was %v", got, readErr, err)
	}
	if got, readErr := os.ReadFile(existing); readErr != nil || string(got) != "new\n" {
		t.Fatalf("failed restoration should leave published value visible: %q, %v", got, readErr)
	}
}

func TestWriteBatchCleansPreparedTempAfterRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	faultHook = func(point, target string) error {
		if point != "before-batch-commit" || target != path {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Mkdir(path, 0o755)
	}
	t.Cleanup(func() { faultHook = nil })

	if err := WriteBatch([]WriteBatchEntry{{Path: path, Data: []byte("new\n")}}); err == nil {
		t.Fatal("WriteBatch succeeded despite rename destination becoming a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".change-saga-batch-") || strings.HasPrefix(entry.Name(), ".change-saga-recovery-") {
			t.Fatalf("temporary state remained after rename failure: %s", entry.Name())
		}
	}
}
