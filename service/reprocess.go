package service

import (
	"context"

	"igb-busca-go/repository"
)

// reprocessBatchSize bounds one reprocess scan page.
const reprocessBatchSize = 2000

// maxReprocessSamples caps before/after examples in a result.
const maxReprocessSamples = 5

// sampleRunes truncates sample texts so dry-run payloads stay small.
const sampleRunes = 300

// pageReprocessStore is the subset of PageRepository used by the reprocess
// scan (kept narrow so tests use a small fake).
type pageReprocessStore interface {
	CountPages(ctx context.Context) (int64, error)
	ListPageTexts(ctx context.Context, limit, offset int64) ([]repository.PageTextRow, error)
	UpdatePageText(ctx context.Context, id int64, text string) error
}

// ReprocessSample shows one changed page before/after.
type ReprocessSample struct {
	BookID     int64  `json:"bookId"`
	BookName   string `json:"bookName"`
	PageNumber int64  `json:"pageNumber"`
	Before     string `json:"before"`
	After      string `json:"after"`
}

// ReprocessResult summarizes a reprocess scan.
type ReprocessResult struct {
	Total   int64             `json:"total"`
	Changed int64             `json:"changed"`
	Samples []ReprocessSample `json:"samples"`
}

// ReprocessPageTexts cleans every stored page text with CleanPageText,
// rewriting rows that change (dryRun only counts and samples).
// progress reports (done, changed) per batch; it may be nil.
func ReprocessPageTexts(ctx context.Context, store pageReprocessStore, dryRun bool, progress func(done, changed int64)) (ReprocessResult, error) {
	result := ReprocessResult{Samples: []ReprocessSample{}}
	total, err := store.CountPages(ctx)
	if err != nil {
		return result, err
	}
	result.Total = total

	var done, changed int64
	report := func() {
		if progress != nil {
			progress(done, changed)
		}
	}
	for offset := int64(0); offset < total; {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		rows, err := store.ListPageTexts(ctx, reprocessBatchSize, offset)
		if err != nil {
			return result, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			cleaned := CleanPageText(row.Text)
			done++
			if cleaned == row.Text {
				continue
			}
			changed++
			if !dryRun {
				if err := store.UpdatePageText(ctx, row.ID, cleaned); err != nil {
					return result, err
				}
			}
			if len(result.Samples) < maxReprocessSamples {
				result.Samples = append(result.Samples, ReprocessSample{
					BookID:     row.BookID,
					BookName:   row.BookName,
					PageNumber: row.PageNumber,
					Before:     truncateRunes(row.Text, sampleRunes),
					After:      truncateRunes(cleaned, sampleRunes),
				})
			}
		}
		offset += int64(len(rows))
		report()
	}
	result.Changed = changed
	report()
	return result, nil
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
