package changeview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordRevisionIgnoresCheckoutLineEndings(t *testing.T) {
	root := t.TempDir()
	file := "record.json"
	path := filepath.Join(root, file)
	if err := os.WriteFile(path, []byte("{\n  \"title\": \"Checkout\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := &inventoryBuilder{root: root}
	lf := builder.digest([]string{file})

	if err := os.WriteFile(path, []byte("{\r\n  \"title\": \"Checkout\"\r\n}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if crlf := builder.digest([]string{file}); crlf != lf {
		t.Fatalf("checkout line endings changed record revision: LF %s, CRLF %s", lf, crlf)
	}

	if err := os.WriteFile(path, []byte("{\r\n  \"title\": \"Pay\"\r\n}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed := builder.digest([]string{file}); changed == lf {
		t.Fatal("an authored content change kept the same record revision")
	}
}
