package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"igb-busca-go/repository"
)

type fakeReprocessStore struct {
	rows    []repository.PageTextRow
	updated map[int64]string
	count   int64
	err     error
}

func (f *fakeReprocessStore) CountPages(context.Context) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.count > 0 {
		return f.count, nil
	}
	return int64(len(f.rows)), nil
}

func (f *fakeReprocessStore) ListPageTexts(_ context.Context, limit, offset int64) ([]repository.PageTextRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	if offset >= int64(len(f.rows)) {
		return nil, nil
	}
	end := offset + limit
	if end > int64(len(f.rows)) {
		end = int64(len(f.rows))
	}
	return f.rows[offset:end], nil
}

func (f *fakeReprocessStore) UpdatePageText(_ context.Context, id int64, text string) error {
	if f.err != nil {
		return f.err
	}
	if f.updated == nil {
		f.updated = map[int64]string{}
	}
	f.updated[id] = text
	return nil
}

func TestReprocessDryRun(t *testing.T) {
	store := &fakeReprocessStore{rows: []repository.PageTextRow{
		{ID: 1, BookID: 7, BookName: "b.pdf", PageNumber: 1, Text: "alma livre"},
		{ID: 2, BookID: 7, BookName: "b.pdf", PageNumber: 2, Text: "precipitan-\ndose"},
	}}
	var progress []int64
	res, err := ReprocessPageTexts(context.Background(), store, true, func(done, _ int64) {
		progress = append(progress, done)
	})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if res.Total != 2 || res.Changed != 1 {
		t.Fatalf("result = %+v, want total 2 changed 1", res)
	}
	if len(res.Samples) != 1 || res.Samples[0].After != "precipitandose" {
		t.Fatalf("samples = %+v", res.Samples)
	}
	if len(store.updated) != 0 {
		t.Fatalf("dry run wrote %d rows", len(store.updated))
	}
	if len(progress) == 0 || progress[len(progress)-1] != 2 {
		t.Fatalf("progress = %v", progress)
	}
}

func TestReprocessWritesOnlyChanged(t *testing.T) {
	store := &fakeReprocessStore{rows: []repository.PageTextRow{
		{ID: 1, Text: "alma livre"},
		{ID: 2, Text: "a  \n\n\nb"},
	}}
	res, err := ReprocessPageTexts(context.Background(), store, false, nil)
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if res.Changed != 1 {
		t.Fatalf("changed = %d, want 1", res.Changed)
	}
	if len(store.updated) != 1 || store.updated[2] != "a\n\nb" {
		t.Fatalf("updated = %+v", store.updated)
	}
}

func TestReprocessStoreError(t *testing.T) {
	store := &fakeReprocessStore{err: errors.New("db down")}
	if _, err := ReprocessPageTexts(context.Background(), store, true, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestJobManagerLifecycle(t *testing.T) {
	mgr := NewJobManager()
	job, err := mgr.Start(JobKindReprocess, 10, func(ctx context.Context, set func(done, changed int64)) (any, error) {
		set(4, 1)
		return map[string]int{"changed": 1}, nil
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := mgr.Start(JobKindSync, -1, nil); !errors.Is(err, ErrJobRunning) {
		t.Fatalf("second start err = %v, want ErrJobRunning", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, ok := mgr.Get(job.ID)
		if !ok {
			t.Fatal("job lost")
		}
		if got.Status == JobDone {
			if got.Done != 4 || got.Changed != 1 {
				t.Fatalf("job = %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck at %s", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := mgr.Get("nope"); ok {
		t.Fatal("unknown job found")
	}
	if list := mgr.List(); len(list) != 1 || list[0].ID != job.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestJobManagerError(t *testing.T) {
	mgr := NewJobManager()
	job, err := mgr.Start(JobKindSync, -1, func(context.Context, func(done, changed int64)) (any, error) {
		return nil, errors.New("boom")
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := mgr.Get(job.ID)
		if got.Status == JobError {
			if got.Error != "boom" {
				t.Fatalf("error = %q", got.Error)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck at %s", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
