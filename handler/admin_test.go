package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"igb-busca-go/middleware"
	"igb-busca-go/repository"
	"igb-busca-go/service"
)

// ---------- admin test doubles ----------

type stubAdminPages struct {
	stubPages
	rows    []repository.PageTextRow
	updated map[int64]string
}

func (s *stubAdminPages) CountPages(context.Context) (int64, error) {
	return int64(len(s.rows)), nil
}

func (s *stubAdminPages) ListPageTexts(_ context.Context, limit, offset int64) ([]repository.PageTextRow, error) {
	if offset >= int64(len(s.rows)) {
		return nil, nil
	}
	end := offset + limit
	if end > int64(len(s.rows)) {
		end = int64(len(s.rows))
	}
	return s.rows[offset:end], nil
}

func (s *stubAdminPages) UpdatePageText(_ context.Context, id int64, text string) error {
	if s.updated == nil {
		s.updated = map[int64]string{}
	}
	s.updated[id] = text
	return nil
}

func newAdminTestApp(users stubUsers, books stubBooks, pages *stubAdminPages, jobs *service.JobManager) *testApp {
	tokens := service.NewTokenService(testSecret)
	authHandler := NewAuthHandler(users, tokens)
	adminHandler := NewAdminHandler(books, pages, stubAnalytics{}, jobs)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/me", authHandler.Me)
	mux.HandleFunc("POST /api/book/sync", adminHandler.Sync)
	mux.HandleFunc("POST /api/book/reprocess", adminHandler.Reprocess)
	mux.HandleFunc("GET /api/book/jobs", adminHandler.Jobs)
	mux.HandleFunc("GET /api/book/jobs/{id}", adminHandler.JobStatus)
	mux.HandleFunc("GET /api/book/reprocess/dry-run", adminHandler.DryRun)

	app := middleware.CORS(middleware.JWT(users, tokens)(mux))
	return &testApp{server: httptest.NewServer(app), tokens: tokens}
}

func adminRows() []repository.PageTextRow {
	return []repository.PageTextRow{
		{ID: 1, BookID: 7, BookName: "b.pdf", PageNumber: 1, Text: "alma livre"},
		{ID: 2, BookID: 7, BookName: "b.pdf", PageNumber: 2, Text: "precipitan-\ndose"},
	}
}

func pollJob(t *testing.T, app *testApp, token, id string) service.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		code, body := do(t, "GET", app.server.URL+"/api/book/jobs/"+id, "", token)
		if code != 200 {
			t.Fatalf("job status code = %d (%s)", code, body)
		}
		var job service.Job
		if err := json.Unmarshal([]byte(body), &job); err != nil {
			t.Fatalf("bad job body: %s", body)
		}
		if job.Status == service.JobDone || job.Status == service.JobError {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck at %s", job.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------- me ----------

func TestMe(t *testing.T) {
	app := newAdminTestApp(testUsers(), stubBooks{}, &stubAdminPages{}, service.NewJobManager())
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/auth/me", "", app.token(t, "2"))
	if code != 200 || !strings.Contains(body, `"perfil":"sacerdotal"`) || !strings.Contains(body, `"name":"bob"`) {
		t.Fatalf("me = %d %s", code, body)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/auth/me", "", ""); code != 401 {
		t.Fatalf("anonymous me = %d, want 401", code)
	}
}

// ---------- guards ----------

func TestAdminGuards(t *testing.T) {
	withWorkdir(t, true)
	app := newAdminTestApp(testUsers(), stubBooks{}, &stubAdminPages{}, service.NewJobManager())
	defer app.server.Close()

	publico := app.token(t, "1")
	anon := ""
	cases := []struct{ method, path string }{
		{"POST", "/api/book/sync"},
		{"POST", "/api/book/reprocess"},
		{"GET", "/api/book/jobs"},
		{"GET", "/api/book/jobs/abc"},
		{"GET", "/api/book/reprocess/dry-run"},
	}
	for _, tc := range cases {
		if code, _ := do(t, tc.method, app.server.URL+tc.path, "", publico); code != 400 {
			t.Fatalf("%s %s publico = %d, want 400", tc.method, tc.path, code)
		}
		if code, body := do(t, tc.method, app.server.URL+tc.path, "", anon); code != 400 || body != "" {
			t.Fatalf("%s %s anonymous = %d %q, want empty 400", tc.method, tc.path, code, body)
		}
	}
}

func TestSyncWithoutCreds(t *testing.T) {
	withWorkdir(t, false)
	app := newAdminTestApp(testUsers(), stubBooks{}, &stubAdminPages{}, service.NewJobManager())
	defer app.server.Close()

	if code, _ := do(t, "POST", app.server.URL+"/api/book/sync", "", app.token(t, "2")); code != 500 {
		t.Fatalf("code = %d, want 500 without Credencial.json", code)
	}
}

// ---------- reprocess job ----------

func TestReprocessJob(t *testing.T) {
	pages := &stubAdminPages{rows: adminRows()}
	app := newAdminTestApp(testUsers(), stubBooks{}, pages, service.NewJobManager())
	defer app.server.Close()
	sac := app.token(t, "2")

	code, body := do(t, "POST", app.server.URL+"/api/book/reprocess", "", sac)
	if code != 202 {
		t.Fatalf("start = %d %s, want 202", code, body)
	}
	var started map[string]string
	if err := json.Unmarshal([]byte(body), &started); err != nil || started["jobId"] == "" {
		t.Fatalf("bad start body: %s", body)
	}
	job := pollJob(t, app, sac, started["jobId"])
	if job.Status != service.JobDone {
		t.Fatalf("job = %+v", job)
	}
	if job.Total != 2 || job.Done != 2 || job.Changed != 1 {
		t.Fatalf("job counts = %+v", job)
	}
	if len(pages.updated) != 1 || pages.updated[2] != "precipitandose" {
		t.Fatalf("updated = %+v", pages.updated)
	}

	code, body = do(t, "GET", app.server.URL+"/api/book/jobs", "", sac)
	if code != 200 || !strings.Contains(body, started["jobId"]) {
		t.Fatalf("jobs = %d %s", code, body)
	}
}

func TestReprocessBusy(t *testing.T) {
	jobs := service.NewJobManager()
	release := make(chan struct{})
	_, err := jobs.Start("test", -1, func(ctx context.Context, set func(done, changed int64)) (any, error) {
		<-release
		return nil, nil
	})
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer close(release)

	app := newAdminTestApp(testUsers(), stubBooks{}, &stubAdminPages{rows: adminRows()}, jobs)
	defer app.server.Close()

	if code, body := do(t, "POST", app.server.URL+"/api/book/reprocess", "", app.token(t, "2")); code != 409 || !strings.Contains(body, "already running") {
		t.Fatalf("busy = %d %s, want 409", code, body)
	}
}

func TestDryRun(t *testing.T) {
	pages := &stubAdminPages{rows: adminRows()}
	app := newAdminTestApp(testUsers(), stubBooks{}, pages, service.NewJobManager())
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/book/reprocess/dry-run", "", app.token(t, "2"))
	if code != 200 {
		t.Fatalf("dry-run = %d %s", code, body)
	}
	var res service.ReprocessResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("bad dry-run body: %s", body)
	}
	if res.Total != 2 || res.Changed != 1 || len(res.Samples) != 1 {
		t.Fatalf("dry-run = %+v", res)
	}
	if res.Samples[0].Before != "precipitan-\ndose" || res.Samples[0].After != "precipitandose" {
		t.Fatalf("sample = %+v", res.Samples[0])
	}
	if len(pages.updated) != 0 {
		t.Fatalf("dry-run wrote %d rows", len(pages.updated))
	}
}

func TestJobUnknown(t *testing.T) {
	app := newAdminTestApp(testUsers(), stubBooks{}, &stubAdminPages{}, service.NewJobManager())
	defer app.server.Close()

	if code, _ := do(t, "GET", app.server.URL+"/api/book/jobs/nope", "", app.token(t, "2")); code != 404 {
		t.Fatalf("code = %d, want 404", code)
	}
}
