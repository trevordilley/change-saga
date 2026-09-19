package coderef

import (
	"strings"
	"testing"
)

func TestLocationRoundTrip(t *testing.T) {
	commit := strings.Repeat("a", 40)
	for _, value := range []string{
		commit + ":internal/cli/cover.go#L12-L20",
		commit + ":internal/cli/cover.go#L7",
		commit + ":docs/a#b.md",
		commit + ":weird:name.go#L3",
	} {
		location, err := ParseLocation(value)
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if location.String() != value {
			t.Fatalf("round trip %s -> %s", value, location)
		}
	}
	for _, value := range []string{
		"HEAD:main.go", commit + ":", commit + ":main.go#L5-L3", commit + ":main.go#L3-L3", commit + ":../x",
	} {
		if _, err := ParseLocation(value); err == nil {
			t.Fatalf("%s parsed", value)
		}
	}
}

func TestDigestRangeAndFind(t *testing.T) {
	content := []byte("a\nb\nc\nb\nc\nd")
	digest, err := DigestRange(content, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := FindDigest(content, 2, digest); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("matches %v", got)
	}
	if _, err := DigestRange(content, 6, 7); err == nil {
		t.Fatal("out of range digest accepted")
	}
	last, _ := DigestRange(content, 6, 6)
	if last != DigestBytes([]byte("d")) {
		t.Fatal("last line without newline digested wrongly")
	}
	if err := Validate(Reference{Commit: strings.Repeat("b", 40), Path: "x.go", Digest: DigestBytes(content)}); err != nil {
		t.Fatal(err)
	}
}
