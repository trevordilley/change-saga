package gitdiff

import (
	"reflect"
	"testing"
)

func TestParseTreeChanges(t *testing.T) {
	patch := []byte(`diff --git a/moved.go b/renamed.go
similarity index 90%
rename from moved.go
rename to renamed.go
index 1111111..2222222 100644
--- a/moved.go
+++ b/renamed.go
@@ -3,0 +4,2 @@ func A() {}
+one
+two
@@ -10 +12 @@ func B() {}
-old
+new
diff --git a/gone.go b/gone.go
deleted file mode 100644
index 3333333..0000000
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-a
-b
diff --git a/new.go b/new.go
new file mode 100644
index 0000000..4444444
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+x
diff --git a/logo.png b/logo.png
index 5555555..6666666 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/run.sh b/run.sh
old mode 100644
new mode 100755
`)
	changes, err := ParseTreeChanges(patch)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileChange{
		{OldPath: "moved.go", NewPath: "renamed.go", Hunks: []Hunk{{OldStart: 3, OldCount: 0, NewStart: 4, NewCount: 2}, {OldStart: 10, OldCount: 1, NewStart: 12, NewCount: 1}}},
		{OldPath: "gone.go", Deleted: true, Hunks: []Hunk{{OldStart: 1, OldCount: 2, NewStart: 0, NewCount: 0}}},
		{NewPath: "new.go", Added: true, Hunks: []Hunk{{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: 1}}},
		{OldPath: "logo.png", NewPath: "logo.png", Binary: true},
		{OldPath: "run.sh", NewPath: "run.sh", ModeChange: true},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("got %#v\nwant %#v", changes, want)
	}
}
