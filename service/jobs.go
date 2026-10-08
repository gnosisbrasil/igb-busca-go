package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Job kinds run by the admin sync page.
const (
	JobKindSync       = "sync"
	JobKindReprocess  = "reprocess"
	maxKeptJobs       = 20
	defaultJobTimeout = 30 * time.Minute
)

// Job statuses, from queued to a terminal state.
const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobDone    = "done"
	JobError   = "error"
)

// Job is one background admin task (Drive sync or text reprocess).
// Total is -1 while unknown (the Drive listing has no upfront count).
type Job struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Total      int64     `json:"total"`
	Done       int64     `json:"done"`
	Changed    int64     `json:"changed"`
	Result     any       `json:"result,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// ErrJobRunning is returned when a job starts while another is active:
// sync and reprocess are both heavy, so they never overlap.
var ErrJobRunning = errors.New("another job is already running")

// JobManager tracks background admin jobs in memory (the API runs as a
// single instance). History keeps the newest maxKeptJobs entries.
type JobManager struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	order   []string
	running bool
	timeout time.Duration
}

// NewJobManager creates a JobManager.
func NewJobManager() *JobManager {
	return &JobManager{jobs: map[string]*Job{}, timeout: defaultJobTimeout}
}

// Start queues fn as a background job of the given kind and returns it
// immediately (status queued/running). Only one job runs at a time.
// fn receives a progress setter and returns the JSON-marshalable result.
func (m *JobManager) Start(kind string, total int64, fn func(ctx context.Context, set func(done, changed int64)) (any, error)) (*Job, error) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, ErrJobRunning
	}
	m.running = true
	job := &Job{ID: newJobID(), Kind: kind, Status: JobQueued, Total: total, StartedAt: time.Now().UTC()}
	m.jobs[job.ID] = job
	m.order = append(m.order, job.ID)
	if len(m.order) > maxKeptJobs {
		delete(m.jobs, m.order[0])
		m.order = m.order[1:]
	}
	m.mu.Unlock()

	go func() {
		m.mu.Lock()
		job.Status = JobRunning
		m.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		set := func(done, changed int64) {
			m.mu.Lock()
			job.Done = done
			job.Changed = changed
			m.mu.Unlock()
		}
		result, err := fn(ctx, set)

		m.mu.Lock()
		defer m.mu.Unlock()
		m.running = false
		job.FinishedAt = time.Now().UTC()
		if err != nil {
			job.Status = JobError
			job.Error = err.Error()
			return
		}
		job.Status = JobDone
		job.Result = result
	}()

	return job, nil
}

// Get returns a snapshot copy of the job, or false when unknown.
func (m *JobManager) Get(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *job, true
}

// List returns snapshot copies of kept jobs, newest first.
func (m *JobManager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		out = append(out, *m.jobs[m.order[i]])
	}
	return out
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return time.Now().UTC().Format("20060102150405") + hex.EncodeToString(b[:])
}
