package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/pltanton/guided-review/internal/inbox"
)

func TestAppendReadWait(t *testing.T) {
	dir := t.TempDir()
	if evs, err := inbox.All(dir); err != nil || len(evs) != 0 {
		t.Fatalf("empty inbox: %v, %v", evs, err)
	}
	ev := inbox.Event{Kind: inbox.KindMessage, Text: "why?", File: "a.go", Lines: "3-4", Step: "s1"}
	if err := inbox.Append(dir, ev); err != nil {
		t.Fatal(err)
	}
	if err := inbox.Append(dir, inbox.Event{Kind: inbox.KindNext, Step: "s1"}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	evs, err := inbox.Wait(ctx, dir, time.Second, 10*time.Millisecond)
	if err != nil || len(evs) != 2 || evs[0].Text != "why?" || evs[1].Kind != inbox.KindNext ||
		evs[0].Time.IsZero() {
		t.Fatalf("Wait = %+v, %v", evs, err)
	}
	evs, err = inbox.Wait(ctx, dir, 50*time.Millisecond, 10*time.Millisecond)
	if err != nil || len(evs) != 0 {
		t.Fatalf("second Wait must time out empty: %+v, %v", evs, err)
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = inbox.Append(dir, inbox.Event{Kind: inbox.KindExplain, File: "a.go", Lines: "9"})
	}()
	evs, err = inbox.Wait(ctx, dir, time.Second, 10*time.Millisecond)
	if err != nil || len(evs) != 1 || evs[0].Kind != inbox.KindExplain {
		t.Fatalf("Wait for late event = %+v, %v", evs, err)
	}

	all, err := inbox.All(dir)
	if err != nil || len(all) != 3 {
		t.Fatalf("All = %d events, %v", len(all), err)
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		e    inbox.Event
		want string
	}{
		{
			inbox.Event{
				Kind:  inbox.KindMessage,
				Step:  "s3",
				File:  "a.go",
				Lines: "57-58",
				Text:  "what if it fails?",
			},
			"[message] s3 a.go:57-58: what if it fails?",
		},
		{
			inbox.Event{Kind: inbox.KindMessage, Step: "s3", Text: "looks fine"},
			"[message] s3: looks fine",
		},
		{
			inbox.Event{Kind: inbox.KindExplain, Step: "s3", File: "a.go", Lines: "40-52"},
			"[explain] s3 a.go:40-52",
		},
		{inbox.Event{Kind: inbox.KindNext, Step: "s3"}, "[next] s3"},
		{inbox.Event{Kind: inbox.KindSkip, Step: "s3", Text: "trivial"}, "[skip] s3: trivial"},
		{inbox.Event{Kind: inbox.KindGoto, Step: "s5"}, "[goto] s5"},
		{
			inbox.Event{
				Kind:    inbox.KindMessage,
				Step:    "s3",
				File:    "a.go",
				Lines:   "57",
				Comment: 2,
				Text:    "not a nit",
			},
			"[message] s3 a.go:57 re #2: not a nit",
		},
		{
			inbox.Event{Kind: inbox.KindEdit, Step: "s3", Comment: 2, Text: "rename to total"},
			"[edit] s3 #2: rename to total",
		},
		{
			inbox.Event{Kind: inbox.KindMessage, Step: "s3", Text: "two things:\n- a\n[x] b"},
			"[message] s3: two things:\n    - a\n    [x] b",
		},
	}
	for _, tt := range tests {
		if got := tt.e.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestWaitingMarker(t *testing.T) {
	dir := t.TempDir()
	if _, ok := inbox.WaitingSince(dir); ok {
		t.Fatal("no marker yet")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = inbox.Wait(context.Background(), dir, time.Second, 10*time.Millisecond)
	}()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if _, ok := inbox.WaitingSince(dir); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("marker not set while waiting")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = inbox.Append(dir, inbox.Event{Kind: inbox.KindNext})
	<-done
	if _, ok := inbox.WaitingSince(dir); ok {
		t.Fatal("marker must be removed after Wait returns")
	}
}

func TestLastWait(t *testing.T) {
	dir := t.TempDir()
	if _, ok := inbox.LastWait(dir); ok {
		t.Fatal("no last wait yet")
	}
	before := time.Now()
	_, _ = inbox.Wait(context.Background(), dir, 10*time.Millisecond, 5*time.Millisecond)
	at, ok := inbox.LastWait(dir)
	if !ok || at.Before(before) {
		t.Fatalf("LastWait = %v, %v", at, ok)
	}
}

func TestIdle(t *testing.T) {
	dir := t.TempDir()
	if _, ok := inbox.IdleSince(dir); ok {
		t.Fatal("no idle marker yet")
	}
	if err := inbox.MarkIdle(dir); err != nil {
		t.Fatal(err)
	}
	if at, ok := inbox.IdleSince(dir); !ok || time.Since(at) > time.Minute {
		t.Fatalf("IdleSince = %v, %v", at, ok)
	}
}
