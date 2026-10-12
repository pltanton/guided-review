package view

import (
	"fmt"
	"testing"
	"time"

	"github.com/pltanton/guided-review/internal/state"
)

func bigStepModel(b *testing.B, rows, width, height int) *model {
	b.Helper()
	r := &state.Review{ID: "mr-1", Current: "s1", Steps: []state.Step{
		{ID: "s1", Title: "first", Kind: "logic", Status: state.StatusPending},
	}}
	m := &model{review: r, step: &r.Steps[0], width: width, height: height, context: 3}
	for i := range 30 {
		m.review.Messages = append(m.review.Messages, state.Message{
			Time: time.Now(), Step: "s1", Text: fmt.Sprintf("message %d with some words in it", i),
		})
	}
	file := ""
	for i := range rows {
		if i%300 == 0 {
			file = fmt.Sprintf("pkg/file%d.go", i/300)
			m.rows = append(m.rows, Row{Kind: RowFile, File: file, Text: file})
		}
		text := fmt.Sprintf("\tresult, err := compute(value%d, options.With(%d), \"literal text\")", i, i)
		kind := RowCode
		if i%3 == 0 {
			kind = RowAdded
		}
		m.rows = append(m.rows, Row{
			Kind: kind, File: file, Line: i + 1, OldLine: i + 1, Text: text, Plain: text,
			HunkStart: i%3 == 0,
		})
	}
	m.relist()
	return m
}

func BenchmarkKeyAndView(b *testing.B) {
	m := bigStepModel(b, 3000, 220, 70)
	b.ResetTimer()
	for i := range b.N {
		k := "j"
		if i%40 >= 20 {
			k = "k"
		}
		m.Update(key(k))
		_ = m.View()
	}
}
