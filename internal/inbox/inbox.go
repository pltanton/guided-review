package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	KindMessage  = "message"
	KindExplain  = "explain"
	KindNext     = "next"
	KindSkip     = "skip"
	KindGoto     = "goto"
	KindEdit     = "edit"
	KindFinished = "finished"
	KindComment  = "comment"
	KindAsk      = "ask"
	KindDetail   = "detail"

	FileName     = "inbox.jsonl"
	offsetFile   = "inbox.offset"
	waitingFile  = "waiting"
	lastWaitFile = "last-wait"
	idleFile     = "idle"
)

type Event struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Step    string    `json:"step,omitempty"`
	File    string    `json:"file,omitempty"`
	Lines   string    `json:"lines,omitempty"`
	Comment int       `json:"comment,omitempty"`
	Text    string    `json:"text,omitempty"`
}

func (e Event) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s", e.Kind, e.Step)
	if e.File != "" {
		fmt.Fprintf(&b, " %s:%s", e.File, e.Lines)
	}
	switch {
	case e.Comment > 0 && e.Kind == KindEdit:
		fmt.Fprintf(&b, " #%d", e.Comment)
	case e.Comment > 0:
		fmt.Fprintf(&b, " re #%d", e.Comment)
	}
	if e.Text != "" {
		fmt.Fprintf(&b, ": %s", strings.ReplaceAll(e.Text, "\n", "\n    "))
	}
	return b.String()
}

func Append(dir string, e Event) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, FileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func All(dir string) ([]Event, error) {
	evs, _, err := readFrom(dir, 0)
	return evs, err
}

func Wait(ctx context.Context, dir string, timeout, poll time.Duration) ([]Event, error) {
	offset, err := readOffset(dir)
	if err != nil {
		return nil, err
	}
	marker := filepath.Join(dir, waitingFile)
	now := time.Now()
	deadline := now.Add(timeout)
	if err := writeStamp(filepath.Join(dir, lastWaitFile), now); err != nil {
		return nil, err
	}
	// The marker holds the deadline: a killed agent cannot remove it, so it expires instead.
	if err := writeStamp(marker, deadline); err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(marker) }()
	for {
		evs, next, err := readFrom(dir, offset)
		if err != nil {
			return nil, err
		}
		if len(evs) > 0 {
			return evs, writeOffset(dir, next)
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func readFrom(dir string, offset int64) ([]Event, int64, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, offset, nil
	}
	if err != nil {
		return nil, offset, err
	}
	if offset > int64(len(data)) {
		offset = 0
	}
	rest := data[offset:]
	end := bytes.LastIndexByte(rest, '\n')
	if end < 0 {
		return nil, offset, nil
	}
	var evs []Event
	for line := range bytes.SplitSeq(rest[:end], []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, offset, fmt.Errorf("%s: %w", FileName, err)
		}
		evs = append(evs, e)
	}
	return evs, offset + int64(end) + 1, nil
}

func readOffset(dir string) (int64, error) {
	data, err := os.ReadFile(filepath.Join(dir, offsetFile))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

func writeOffset(dir string, offset int64) error {
	return os.WriteFile(
		filepath.Join(dir, offsetFile),
		[]byte(strconv.FormatInt(offset, 10)),
		0o600,
	)
}

func Waiting(dir string) bool {
	deadline, ok := readStamp(filepath.Join(dir, waitingFile))
	return ok && time.Now().Before(deadline)
}

func LastWait(dir string) (time.Time, bool) {
	return readStamp(filepath.Join(dir, lastWaitFile))
}

func writeStamp(path string, t time.Time) error {
	return os.WriteFile(path, []byte(t.Format(time.RFC3339Nano)), 0o600)
}

func readStamp(path string) (time.Time, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	return t, err == nil
}

func MarkIdle(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeStamp(filepath.Join(dir, idleFile), time.Now())
}

func IdleSince(dir string) (time.Time, bool) {
	return readStamp(filepath.Join(dir, idleFile))
}
