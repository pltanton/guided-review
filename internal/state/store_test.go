package state_test

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/pltanton/guided-review/internal/state"
)

func TestConcurrentUpdatesKeepEveryWrite(t *testing.T) {
	s := state.Store{Dir: t.TempDir()}
	if err := s.Save(&state.Review{ID: "mr-1"}); err != nil {
		t.Fatal(err)
	}
	const writers, each = 6, 15
	done := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		defer close(readErr)
		for {
			select {
			case <-done:
				return
			default:
			}
			if _, err := s.Load("mr-1"); err != nil {
				readErr <- err
				return
			}
		}
	}()
	errs := make(chan error, writers*each)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				errs <- s.Update("mr-1", func(r *state.Review) error {
					r.Comments = append(r.Comments, state.Comment{ID: w*each + i})
					return nil
				})
			}
		})
	}
	wg.Wait()
	close(done)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := <-readErr; err != nil {
		t.Fatalf("a reader saw a partial state: %v", err)
	}
	r, err := s.Load("mr-1")
	if err != nil || len(r.Comments) != writers*each {
		t.Fatalf("got %d comments, want %d (%v)", len(r.Comments), writers*each, err)
	}
	entries, _ := os.ReadDir(s.ReviewDir("mr-1"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestUpdate(t *testing.T) {
	s := state.Store{Dir: t.TempDir()}
	err := s.Update("nope", func(*state.Review) error { return nil })
	if !errors.Is(err, state.ErrNoReview) {
		t.Fatalf("Update on a missing review: %v", err)
	}
	if err := s.Save(&state.Review{ID: "mr-1"}); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	err = s.Update("mr-1", func(r *state.Review) error {
		r.Summary = "changed"
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Update must return the apply error: %v", err)
	}
	if r, _ := s.Load("mr-1"); r.Summary != "" {
		t.Fatal("a failed apply must not be saved")
	}
}
