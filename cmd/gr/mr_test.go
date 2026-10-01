package main

import (
	"slices"
	"testing"

	"github.com/pltanton/guided-review/internal/diff"
)

func TestPRHunks(t *testing.T) {
	files, err := diff.Parse(`diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -5,2 +4,0 @@
-x
-y
@@ -20 +18 @@
-a
+b
@@ -26 +24,2 @@
-c
+d
+e
@@ -1,0 +40 @@
+z
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []lineSpan{{2, 7}, {15, 28}, {37, 43}}
	if got := prHunks(files[0]); !slices.Equal(got, want) {
		t.Fatalf("prHunks = %v, want %v", got, want)
	}
	for line, in := range map[int]bool{1: false, 2: true, 7: true, 8: false, 15: true, 28: true} {
		if _, ok := prHunk(files, "f.go", line); ok != in {
			t.Errorf("line %d in the PR diff = %v, want %v", line, ok, in)
		}
	}
	if _, ok := prHunk(files, "other.go", 2); ok {
		t.Error("a file outside the diff has no hunks")
	}
}
