package diff_test

import (
	"reflect"
	"testing"

	"github.com/pltanton/guided-review/internal/diff"
)

const sample = `diff --git a/api/transfer.go b/api/transfer.go
index 111..222 100644
--- a/api/transfer.go
+++ b/api/transfer.go
@@ -10,0 +11,2 @@ func A() {
+	x := 1
+	y := 2
@@ -20,2 +21,0 @@ func B() {
-	old1
-	--- old2
diff --git a/new.go b/new.go
new file mode 100644
index 000..333
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+package new
diff --git a/gone.go b/gone.go
deleted file mode 100644
index 444..000
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-package gone
diff --git a/old/name.go b/new/name.go
similarity index 90%
rename from old/name.go
rename to new/name.go
diff --git a/logo.png b/logo.png
index 555..666 100644
Binary files a/logo.png and b/logo.png differ
`

func TestParse(t *testing.T) {
	got, err := diff.Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	want := []diff.File{
		{Path: "api/transfer.go", Status: diff.Modified, Hunks: []diff.Hunk{
			{
				OldStart: 10,
				OldLines: 0,
				NewStart: 11,
				NewLines: 2,
				Lines: []diff.Line{
					{Kind: '+', Text: "\tx := 1"},
					{Kind: '+', Text: "\ty := 2"},
				},
			},
			{
				OldStart: 20,
				OldLines: 2,
				NewStart: 21,
				NewLines: 0,
				Lines: []diff.Line{
					{Kind: '-', Text: "\told1"},
					{Kind: '-', Text: "\t--- old2"},
				},
			},
		}},
		{Path: "new.go", Status: diff.Added, Hunks: []diff.Hunk{
			{
				OldStart: 0,
				OldLines: 0,
				NewStart: 1,
				NewLines: 1,
				Lines:    []diff.Line{{Kind: '+', Text: "package new"}},
			},
		}},
		{Path: "gone.go", Status: diff.Deleted, Hunks: []diff.Hunk{
			{
				OldStart: 1,
				OldLines: 1,
				NewStart: 0,
				NewLines: 0,
				Lines:    []diff.Line{{Kind: '-', Text: "package gone"}},
			},
		}},
		{Path: "new/name.go", OldPath: "old/name.go", Status: diff.Renamed},
		{Path: "logo.png", Status: diff.Modified, Binary: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse:\n got %+v\nwant %+v", got, want)
	}
}

func TestHunkRange(t *testing.T) {
	tests := []struct {
		h    diff.Hunk
		end  int
		text string
	}{
		{diff.Hunk{NewStart: 11, NewLines: 2}, 12, "11-12"},
		{diff.Hunk{NewStart: 5, NewLines: 1}, 5, "5"},
		{diff.Hunk{NewStart: 21, NewLines: 0}, 21, "21(del)"},
	}
	for _, tt := range tests {
		if got := tt.h.NewEnd(); got != tt.end {
			t.Errorf("%+v NewEnd = %d, want %d", tt.h, got, tt.end)
		}
		if got := tt.h.Range(); got != tt.text {
			t.Errorf("%+v Range = %q, want %q", tt.h, got, tt.text)
		}
	}
}

func TestStat(t *testing.T) {
	files, err := diff.Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if a, d := files[0].Stat(); a != 2 || d != 2 {
		t.Fatalf("Stat = +%d -%d, want +2 -2", a, d)
	}
}

func TestOldLineFor(t *testing.T) {
	f := diff.File{Hunks: []diff.Hunk{
		{OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 2},
		{OldStart: 10, OldLines: 2, NewStart: 11, NewLines: 0},
	}}
	tests := []struct {
		line, old int
		added     bool
	}{
		{1, 1, false},
		{2, 0, true},
		{3, 0, true},
		{5, 4, false},
		{11, 10, false},
		{12, 13, false},
	}
	for _, tt := range tests {
		old, added := f.OldLineFor(tt.line)
		if old != tt.old || added != tt.added {
			t.Errorf("OldLineFor(%d) = %d, %v; want %d, %v", tt.line, old, added, tt.old, tt.added)
		}
	}
}
