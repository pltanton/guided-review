package diff_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/testrepo"
)

func numbered(prefix string, n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s %d\n", prefix, i)
	}
	return b.String()
}

type change struct {
	status  diff.Status
	oldPath string
	binary  bool
}

func oddRepo(t *testing.T) (*testrepo.Repo, string, string, map[string]change) {
	tr := testrepo.New(t)
	tr.Write("sp ace.txt", "one\n")
	tr.Write("x b/y.txt", "one\n")
	tr.Write(`q"uote.txt`, "one\n")
	tr.Write("кириллица.txt", "one\n")
	tr.Write("mode.sh", "echo\n")
	tr.Write("nonl.txt", "a\nb\n")
	tr.Write("old/name.go", numbered("line", 20))
	tr.Write("logo.png", "\x89PNG\x00\x01\x02")
	tr.Write("gone.txt", "bye\n")
	base := tr.Commit("base")

	tr.Write("sp ace.txt", "one\ntwo\n")
	tr.Write("x b/y.txt", "uno\n")
	tr.Write(`q"uote.txt`, "one\ntwo\n")
	tr.Write("кириллица.txt", "один\n")
	if err := os.Chmod(filepath.Join(tr.Dir, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	tr.Write("nonl.txt", "a\nb")
	tr.Git("rm", "-q", "old/name.go")
	tr.Write("new/name.go", strings.Replace(numbered("line", 20), "line 7\n", "line seven\n", 1))
	tr.Write("logo.png", "\x89PNG\x00\x03\x04")
	tr.Git("rm", "-q", "gone.txt")
	tr.Write("empty.txt", "")
	head := tr.Commit("head")

	return tr, base, head, map[string]change{
		"sp ace.txt":    {status: diff.Modified},
		"x b/y.txt":     {status: diff.Modified},
		`q"uote.txt`:    {status: diff.Modified},
		"кириллица.txt": {status: diff.Modified},
		"mode.sh":       {status: diff.Modified},
		"nonl.txt":      {status: diff.Modified},
		"new/name.go":   {status: diff.Renamed, oldPath: "old/name.go"},
		"logo.png":      {status: diff.Modified, binary: true},
		"gone.txt":      {status: diff.Deleted},
		"empty.txt":     {status: diff.Added},
	}
}

type numstat struct {
	added, deleted int
	binary         bool
}

func gitNumstat(t *testing.T, tr *testrepo.Repo, algo, base, head string) map[string]numstat {
	out := tr.Git("diff", "--numstat", "-z", "-M", "--diff-algorithm="+algo, base, head)
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	stats := map[string]numstat{}
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		path := parts[2]
		if path == "" {
			path = fields[i+2]
			i += 2
		}
		if parts[0] == "-" {
			stats[path] = numstat{binary: true}
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		stats[path] = numstat{added: a, deleted: d}
	}
	return stats
}

func TestFilesFromGit(t *testing.T) {
	configs := map[string]string{
		"clean": "",
		"noprefix": "[diff]\n\tnoprefix = true\n\tmnemonicPrefix = true\n" +
			"[core]\n\tquotePath = true\n",
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			gitconfig := filepath.Join(t.TempDir(), "gitconfig")
			if err := os.WriteFile(gitconfig, []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
			tr, base, head, want := oddRepo(t)
			repo := gitx.Repo{Dir: tr.Dir}
			for _, algo := range gitx.DiffAlgorithms {
				files, err := repo.Files(context.Background(), algo, base, head)
				if err != nil {
					t.Fatalf("%s: %v", algo, err)
				}
				stats := gitNumstat(t, tr, algo, base, head)
				raw, err := repo.DiffWith(context.Background(), algo, base, head)
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := diff.Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				for i, f := range parsed {
					if len(f.Hunks) > 0 && f.Path != files[i].Path {
						t.Errorf("%s: Parse path %q, name-status %q", algo, f.Path, files[i].Path)
					}
				}
				if len(files) != len(want) {
					t.Fatalf("%s: %d files, want %d: %+v", algo, len(files), len(want), files)
				}
				for _, f := range files {
					w, ok := want[f.Path]
					if !ok {
						t.Fatalf("%s: unexpected path %q", algo, f.Path)
					}
					if f.Status != w.status || f.OldPath != w.oldPath || f.Binary != w.binary {
						t.Errorf("%s %q: status %s from %q binary %v, want %s from %q binary %v",
							algo, f.Path, f.Status, f.OldPath, f.Binary, w.status, w.oldPath, w.binary)
					}
					added, deleted := f.Stat()
					if got := (numstat{added, deleted, f.Binary}); got != stats[f.Path] {
						t.Errorf("%s %q: stat %+v, git numstat %+v", algo, f.Path, got, stats[f.Path])
					}
				}
			}
		})
	}
}

func TestNoNewlineAtEnd(t *testing.T) {
	tr, base, head, _ := oddRepo(t)
	files, err := gitx.Repo{Dir: tr.Dir}.Files(context.Background(), "histogram", base, head)
	if err != nil {
		t.Fatal(err)
	}
	files = slices.DeleteFunc(files, func(f diff.File) bool { return f.Path != "nonl.txt" })
	want := []diff.Line{{Kind: '-', Text: "b"}, {Kind: '+', Text: "b"}}
	if len(files) != 1 || len(files[0].Hunks) != 1 ||
		fmt.Sprint(files[0].Hunks[0].Lines) != fmt.Sprint(want) {
		t.Fatalf("nonl.txt: %+v", files)
	}
}

func TestSymlinkTypeChange(t *testing.T) {
	tr := testrepo.New(t)
	tr.Write("target", "x\n")
	tr.Write("link", "y\n")
	base := tr.Commit("base")
	if err := os.Remove(filepath.Join(tr.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(tr.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	tr.Write("after", "z\n")
	head := tr.Commit("head")
	files, err := gitx.Repo{Dir: tr.Dir}.Files(context.Background(), "histogram", base, head)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.Path + " " + string(f.Status)
	}
	if want := "after added,link deleted,link added"; strings.Join(got, ",") != want {
		t.Fatalf("files = %v, want %s", got, want)
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

func TestOldLineFor(t *testing.T) {
	tr := testrepo.New(t)
	old := strings.Split(strings.TrimSuffix(numbered("line", 12), "\n"), "\n")
	tr.Write("a.txt", strings.Join(old, "\n")+"\n")
	base := tr.Commit("base")
	cur := append([]string{old[0], "x", "y"}, old[2:9]...)
	cur = append(cur, old[11])
	tr.Write("a.txt", strings.Join(cur, "\n")+"\n")
	head := tr.Commit("head")

	for _, algo := range gitx.DiffAlgorithms {
		files, err := gitx.Repo{Dir: tr.Dir}.Files(context.Background(), algo, base, head)
		if err != nil || len(files) != 1 {
			t.Fatalf("%s: %+v, %v", algo, files, err)
		}
		for n := 1; n <= len(cur); n++ {
			o, added := files[0].OldLineFor(n)
			switch {
			case added != (cur[n-1] == "x" || cur[n-1] == "y"):
				t.Errorf("%s: OldLineFor(%d) added = %v", algo, n, added)
			case !added && (o < 1 || o > len(old) || old[o-1] != cur[n-1]):
				t.Errorf("%s: OldLineFor(%d) = %d, %q is not %q", algo, n, o, cur[n-1], old[max(o-1, 0)])
			}
		}
	}
}
