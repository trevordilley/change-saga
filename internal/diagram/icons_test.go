package diagram

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

// lucideBundleSHA256 pins the generated bundle: regenerating it from another
// revision, or editing it by hand, must update this digest deliberately.
const lucideBundleSHA256 = "7e1d96dcb071768234245b5683e9a5ec0a7ed86a30d148636c7edb3b6bca6e45"

func TestLucideBundleIsCompleteAndPinned(t *testing.T) {
	data, err := bundled.ReadFile("assets/lucide/icons.json")
	if err != nil {
		t.Fatal(err)
	}
	if digest := sha256.Sum256(data); hex.EncodeToString(digest[:]) != lucideBundleSHA256 {
		t.Fatalf("icons.json digest %x does not match the pinned bundle; regenerate with internal/diagram/lucidebundle", digest)
	}
	names := Icons("")
	if len(names) < 1800 {
		t.Fatalf("expected the full Lucide set, found %d icons", len(names))
	}
	for _, name := range names {
		data, err := Icon(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, children, err := parseMarkup(string(data)); err != nil || len(children) == 0 {
			t.Errorf("%s does not pass the drawing allowlist: %v", name, err)
		}
	}
	if !strings.Contains(IconLicense(), "ISC License") || !strings.Contains(IconLicense(), "Cole Bemis") {
		t.Fatal("the Lucide ISC and Feather MIT notices must ship with the icons")
	}
}

func TestIconSearchMatchesTagsAndAliases(t *testing.T) {
	storage := Icons("storage")
	for _, want := range []string{"lucide:database", "lucide:hard-drive"} {
		if !slices.Contains(storage, want) {
			t.Errorf("searching storage misses %s: %v", want, storage)
		}
	}
	if !slices.Contains(Icons("alert-triangle"), "lucide:triangle-alert") {
		t.Error("a deprecated alias should find the icon's current name")
	}
	if IconExists("database") || IconExists("lucide:alert-triangle") {
		t.Error("icons are named only by their current lucide: name")
	}
	if _, err := Icon("database"); err == nil {
		t.Error("an unprefixed name must not resolve")
	}
}
