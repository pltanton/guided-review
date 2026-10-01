package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/state"
)

func TestMain(m *testing.M) {
	switch os.Getenv("GR_TEST_AS") {
	case "gr":
		main()
		os.Exit(0)
	case "glab", "gh":
		os.Exit(fakeForge(os.Args[1:]))
	}
	os.Exit(m.Run())
}

type forge struct {
	Drafts      []string `json:"drafts"`
	Notes       []string `json:"notes"`
	Reviews     int      `json:"reviews"`
	Replies     int      `json:"replies"`
	Resolves    int      `json:"resolves"`
	Approved    bool     `json:"approved"`
	DraftPosts  int      `json:"draft_posts"`
	FailDraft   int      `json:"fail_draft,omitempty"`
	FailApprove bool     `json:"fail_approve,omitempty"`
	FailReply   bool     `json:"fail_reply,omitempty"`
}

func forgePath() string { return filepath.Join(os.Getenv("GR_TEST_FORGE"), "forge.json") }

func loadForge() (forge, error) {
	var f forge
	data, err := os.ReadFile(forgePath())
	if os.IsNotExist(err) {
		return f, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &f)
	}
	return f, err
}

func (f forge) save() error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(forgePath(), data, 0o600)
}

func fakeForge(args []string) int {
	method, path, input := "GET", "", ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-X":
			method, i = args[i+1], i+1
		case "--input":
			input, i = args[i+1], i+1
		case "--hostname", "-H", "-f", "-F":
			i++
		case "--paginate":
		default:
			path = args[i]
		}
	}
	f, err := loadForge()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fail := func(what string) int {
		_ = f.save()
		fmt.Fprintln(os.Stderr, "fake forge refuses "+what)
		return 1
	}
	switch {
	case method == "GET" && strings.Contains(path, "/draft_notes"):
		var out []map[string]string
		for _, n := range f.Drafts {
			out = append(out, map[string]string{"note": n})
		}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))
		return 0
	case strings.HasSuffix(path, "/draft_notes"):
		f.DraftPosts++
		if f.DraftPosts == f.FailDraft {
			f.FailDraft = 0
			return fail("the draft")
		}
		var d struct{ Note string }
		data, err := os.ReadFile(input)
		if err == nil {
			err = json.Unmarshal(data, &d)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		f.Drafts = append(f.Drafts, d.Note)
	case strings.HasSuffix(path, "/bulk_publish"):
		f.Notes, f.Drafts = append(f.Notes, f.Drafts...), nil
	case strings.HasSuffix(path, "/approve"):
		if f.FailApprove {
			f.FailApprove = false
			return fail("the approve")
		}
		f.Approved = true
	case strings.HasSuffix(path, "/reviews"):
		f.Reviews++
	case strings.HasSuffix(path, "/notes") || strings.HasSuffix(path, "/replies"):
		if f.FailReply {
			f.FailReply = false
			return fail("the reply")
		}
		f.Replies++
	case method == "PUT" || path == "graphql":
		f.Resolves++
	default:
		return fail(method + " " + path)
	}
	if err := f.save(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func (h *harness) publishScript(provider string, set func(*forge)) (forge, string, error) {
	h.t.Helper()
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			h.t.Skipf("the publish scripts need %s", tool)
		}
	}
	self, err := os.Executable()
	if err != nil {
		h.t.Fatal(err)
	}
	dir := filepath.Join(h.cache, "forge")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	for _, tool := range []string{"gr", "glab", "gh"} {
		script := fmt.Sprintf("#!/bin/sh\nGR_TEST_AS=%s exec %q \"$@\"\n", tool, self)
		if err := os.WriteFile(filepath.Join(dir, "bin", tool), []byte(script), 0o755); err != nil {
			h.t.Fatal(err)
		}
	}
	t := h.t
	t.Setenv("GR_TEST_FORGE", dir)
	f, err := loadForge()
	if err != nil {
		t.Fatal(err)
	}
	set(&f)
	if err := f.save(); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "skills", "guided-review", "scripts",
		"publish-"+provider+".sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = h.repo.Dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"))
	out, runErr := cmd.CombinedOutput()
	f, err = loadForge()
	if err != nil {
		t.Fatal(err)
	}
	return f, string(out), runErr
}

func TestPublishGitLabRetriesOnlyTheRest(t *testing.T) {
	h, _ := mrHarness(t)
	h.mustRun("", "comment", "add", "--file", "api/transfer.go", "--lines", "5",
		"--severity", "nit", "first")
	h.mustRun("", "comment", "add", "--file", "api/transfer.go", "--lines", "7",
		"--severity", "nit", "second")
	h.mustRun("", "step", "next")
	h.mustRun("", "step", "next")
	h.mustRun("", "prepare", "--verdict", "approve", "--approve")
	h.mustRun("", "export")

	f, out, err := h.publishScript("gitlab", func(f *forge) { f.FailDraft = 2 })
	if err == nil || len(f.Drafts) != 1 || len(f.Notes) != 0 {
		t.Fatalf("the second draft fails and nothing is published: %+v\n%s", f, out)
	}
	assertContains(t, out, "not published: #1, #2, summary, approve")
	if _, err := h.run("", "prepare", "--verdict", "changes"); err != nil {
		t.Fatalf("nothing went out, so the plan stays as prepared: %v", err)
	}
	h.mustRun("", "prepare", "--verdict", "approve", "--approve")

	h.mustRun("", "export")
	f, out, err = h.publishScript("gitlab", func(f *forge) { f.FailApprove = true })
	if err == nil || len(f.Notes) != 3 || len(f.Drafts) != 0 || f.Approved {
		t.Fatalf("the retry reuses the waiting draft and stops at approve: %+v\n%s", f, out)
	}
	assertContains(t, out, "marked 2 comments", "not published: approve")
	if _, err := h.run("", "export"); err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(h.mustRun("", "export", "--dir"))
	data, err := os.ReadFile(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	var x export
	if err := json.Unmarshal(data, &x); err != nil || len(x.Drafts) != 0 || !x.Approve {
		t.Fatalf("the next export holds only the approve: %s %v", data, err)
	}

	f, out, err = h.publishScript("gitlab", func(*forge) {})
	if err != nil || len(f.Notes) != 3 || !f.Approved {
		t.Fatalf("the last run approves without new notes: %+v %v\n%s", f, err, out)
	}
	_, r, err := loadReview(t.Context(), h.repo.Dir)
	if err != nil || r.Publish != nil || !r.Comments[0].Published || r.SummaryRound != 1 {
		t.Fatalf("everything is marked: %+v %v", r.Publish, err)
	}
}

func TestPublishGitHubThreadRetry(t *testing.T) {
	h := newHarness(t)
	h.stubGitHub()
	h.mustRun("", "init", "https://github.com/o/r/pull/7")
	h.mustRun(goodPlan, "plan", "set")
	h.mustRun("", "comment", "add", "--file", "api/transfer.go", "--lines", "5",
		"--severity", "nit", "first")
	h.mustRun("", "step", "next")
	h.mustRun("", "step", "next")
	h.edit(func(r *state.Review) {
		r.Discussions = []state.Discussion{{ID: "t1", Author: "me", Body: "**nit** old",
			Resolvable: true, ReplyTo: 9, Notes: []state.Note{
				{Author: "me", Body: "**nit** old"}, {Author: "bob", Body: "done"}}}}
	})
	h.decide("t1", state.VerdictResolve, "thanks")
	h.mustRun("", "prepare", "--verdict", "changes")
	h.mustRun("", "export")

	f, out, err := h.publishScript("github", func(f *forge) { f.FailReply = true })
	if err == nil || f.Reviews != 1 || f.Replies != 0 {
		t.Fatalf("the review goes out, the reply fails: %+v\n%s", f, out)
	}
	assertContains(t, out, "marked 1 comments and 0 threads", "not published: thread t1")
	h.mustRun("", "export")
	f, out, err = h.publishScript("github", func(*forge) {})
	if err != nil || f.Reviews != 1 || f.Replies != 1 || f.Resolves != 1 {
		t.Fatalf("the retry sends only the thread: %+v %v\n%s", f, err, out)
	}
}

func TestMarkPublishedChecksTheExport(t *testing.T) {
	h, _ := mrHarness(t)
	h.mustRun("", "step", "next")
	h.mustRun("", "step", "next")
	h.mustRun("", "prepare", "--verdict", "changes")
	h.mustRun("", "export")
	h.mustRun("", "prepare", "--verdict", "blocked")
	h.sendAll()
	if _, err := h.run("", "mark-published"); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("an export from before the last prepare is refused, got %v", err)
	}
	for _, args := range [][]string{{"export"}, {"prepare", "--verdict", "changes"}} {
		if _, err := h.run("", args...); err == nil ||
			!strings.Contains(err.Error(), "run gr mark-published first") {
			t.Fatalf("gr %v must wait while a sent part is unmarked, got %v", args, err)
		}
	}
	if err := os.Remove(filepath.Join(h.exportDir("mr-7"), publishedLog)); err != nil {
		t.Fatal(err)
	}
	h.mustRun("", "export")
	assertContains(t, h.mustRun("", "mark-published"), "not published: summary")
	h.publishAll()
	if _, err := h.run("", "mark-published"); err == nil ||
		!strings.Contains(err.Error(), "marked already") {
		t.Fatalf("a marked export is not marked twice, got %v", err)
	}
}
