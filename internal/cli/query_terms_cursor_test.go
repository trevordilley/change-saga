package cli

import "testing"

func TestTermsCursorChecksSnapshotBeforeCurrentCollectionSize(t *testing.T) {
	t.Parallel()
	key := `{"story":"checkout"}`
	cursor := encodeTermsCursor(key, "old-snapshot", 3)
	if _, err := decodeTermsCursor(cursor, key, "new-snapshot", 1); err == nil || err.Code != "stale_snapshot" || !err.Retryable {
		t.Fatalf("valid cursor after collection shrink = %#v", err)
	}
	if _, err := decodeTermsCursor(cursor, key, "old-snapshot", 1); err == nil || err.Code != "invalid_argument" || err.Retryable {
		t.Fatalf("out-of-range cursor at the same snapshot = %#v", err)
	}
	if _, err := decodeTermsCursor(cursor+"A", key, "new-snapshot", 1); err == nil || err.Code != "invalid_argument" {
		t.Fatalf("tampered cursor = %#v", err)
	}
}
