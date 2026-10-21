package view

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

func finishModel(t *testing.T, mr bool) (*model, *[][]string) {
	t.Helper()
	m, _ := newTestModel(t)
	if mr {
		m.review.MR = &state.MR{Host: "gitlab.com", Project: "g/p", IID: 7}
	}
	m.review.Publish = &state.PublishPlan{Verdict: "changes", Decisions: "kept the cache"}
	for i := range m.review.Steps {
		m.review.Steps[i].Status = state.StatusDone
	}
	var ran [][]string
	m.runGr = func(args ...string) (string, error) {
		ran = append(ran, args)
		if args[0] == "prepare" && args[2] != "approve" && slices.Contains(args, "--approve") {
			return "--approve only goes with --verdict approve", errors.New("exit status 1")
		}
		return "--- #4 a.go:3\n**nit** rename\n", nil
	}
	m.Update(key("P"))
	return m, &ran
}

func lastPrepare(ran [][]string) []string {
	for i := len(ran) - 1; i >= 0; i-- {
		if ran[i][0] == "prepare" {
			return ran[i]
		}
	}
	return nil
}

func TestPreviewVerdictAndApprove(t *testing.T) {
	m, ran := finishModel(t, true)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "v verdict · a approve") {
		t.Fatalf("the preview header names the keys:\n%s", v)
	}
	m.Update(key("v"))
	want := []string{"prepare", "--verdict", "blocked", "--decisions", "kept the cache"}
	if got := lastPrepare(*ran); !slices.Equal(got, want) || m.review.Publish.Verdict != "blocked" {
		t.Fatalf("v moves changes → blocked through gr prepare: %q", got)
	}
	m.Update(key("a"))
	want = []string{"prepare", "--verdict", "approve", "--decisions", "kept the cache", "--approve"}
	if got := lastPrepare(*ran); !slices.Equal(got, want) || !m.review.Publish.Approve || m.err != nil {
		t.Fatalf("a on another verdict switches to approve and approves: %q %v", got, m.err)
	}
	m.Update(key("v"))
	want = []string{"prepare", "--verdict", "changes", "--decisions", "kept the cache"}
	if got := lastPrepare(*ran); !slices.Equal(got, want) || m.review.Publish.Approve {
		t.Fatalf("leaving the approve verdict turns approve off: %q", got)
	}
	if m.preview == "" || m.visual {
		t.Fatal("v stays in the preview, it does not select lines")
	}
}

func TestPreviewPartialAndOpenThreads(t *testing.T) {
	m, ran := finishModel(t, true)
	m.review.Steps[1].Status = state.StatusPending
	m.Update(key("v"))
	if got := lastPrepare(*ran); !slices.Contains(got, "--partial") {
		t.Fatalf("a review with steps left stays partial when the verdict changes: %q", got)
	}
	m.review.MR.Me = "me"
	m.review.Discussions = []state.Discussion{{ID: "d1", Author: "me", Resolvable: true}}
	n := len(*ran)
	m.Update(key("a"))
	if len(*ran) != n || !strings.Contains(m.status, "approve waits for your open threads: d1") {
		t.Fatalf("a with open threads explains and runs nothing: %q", m.status)
	}
	m.Update(key("v"))
	if got := lastPrepare(*ran); got[2] != "changes" {
		t.Fatalf("v skips approve while threads are open: %q", got)
	}
}

func TestFinishWithNothingNew(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Round = 2
	m.review.Publish = &state.PublishPlan{Verdict: "changes"}
	m.runGr = func(args ...string) (string, error) { return "", nil }
	m.Update(key("P"))
	if m.preview != "" || !strings.Contains(m.status, "round 2 is published already") {
		t.Fatalf("P with nothing new says so instead of doing nothing: %q", m.status)
	}
}

func TestPreviewApproveNeedsMR(t *testing.T) {
	m, ran := finishModel(t, false)
	if v := ansi.Strip(m.View()); strings.Contains(v, "a approve") {
		t.Fatalf("no approve hint without a merge request:\n%s", v)
	}
	m.Update(key("a"))
	if lastPrepare(*ran) != nil || !strings.Contains(m.status, "merge request") {
		t.Fatalf("a without an MR explains and runs nothing: %q", m.status)
	}
}

func TestPreviewKeysFollowTheKeymap(t *testing.T) {
	m, ran := finishModel(t, true)
	var err error
	m.km, err = newKeymap(map[string][]string{
		"verdict": {"V"}, "delete-comment": {"X"}, "finish": {"F"},
	})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(key("v"))
	if lastPrepare(*ran) != nil {
		t.Fatal("a remapped verdict no longer listens to v")
	}
	m.Update(key("V"))
	if lastPrepare(*ran) == nil {
		t.Fatal("V cycles the remapped verdict")
	}
	m.Update(key("X"))
	m.Update(key("X"))
	if !slices.ContainsFunc(*ran, func(a []string) bool {
		return slices.Equal(a, []string{"comment", "delete", "4"})
	}) {
		t.Fatalf("the remapped delete key deletes in the preview: %q", *ran)
	}
	m.Update(key("F"))
	if !slices.ContainsFunc(*ran, func(a []string) bool { return slices.Equal(a, []string{"export"}) }) {
		t.Fatalf("the remapped finish key hands the review over: %q", *ran)
	}
	if _, err := newKeymap(map[string][]string{"approve": {"E"}}); err == nil {
		t.Fatal("a preview key taken twice is a conflict")
	}
}

func TestPreviewHidesCommentMarkers(t *testing.T) {
	m, _ := finishModel(t, true)
	m.preview = "--- #4 a.go:3\n**nit** rename\n\n" + state.CommentMarker(4) + "\n"
	lines, _ := m.finishBody(80)
	if text := ansi.Strip(strings.Join(lines, "\n")); strings.Contains(text, "gr:comment") ||
		!strings.Contains(text, "rename") {
		t.Fatalf("preview shows the marker or lost the body:\n%s", text)
	}
}
