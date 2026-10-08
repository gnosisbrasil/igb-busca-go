package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"igb-busca-go/middleware"
	"igb-busca-go/repository"
	"igb-busca-go/service"
)

// adminPageStore is the page access used by admin jobs: the ported
// PageStore plus the reprocess scan (no .NET counterpart).
type adminPageStore interface {
	repository.PageStore
	CountPages(ctx context.Context) (int64, error)
	ListPageTexts(ctx context.Context, limit, offset int64) ([]repository.PageTextRow, error)
	UpdatePageText(ctx context.Context, id int64, text string) error
}

// writeAccepted mirrors writeJSON with a 202 status.
func writeAccepted(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(v)
}

// AdminHandler serves the manual admin jobs (new endpoints, no .NET
// counterpart): Drive sync, text reprocess and job status. All routes
// require the sacerdotal perfil, guarded like the ported admin routes
// (400 with an empty body).
type AdminHandler struct {
	books     repository.BookStore
	pages     adminPageStore
	analytics repository.AnalyticsStore
	jobs      *service.JobManager
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(books repository.BookStore, pages adminPageStore, analytics repository.AnalyticsStore, jobs *service.JobManager) *AdminHandler {
	return &AdminHandler{books: books, pages: pages, analytics: analytics, jobs: jobs}
}

func (h *AdminHandler) readFileService() (*service.ReadFileService, error) {
	return service.NewReadFileService(h.books, h.pages, h.analytics)
}

// Sync starts a background Drive import: POST /api/book/sync -> 202
// {jobId}. Only one job runs at a time (409 while busy).
func (h *AdminHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	svc, err := h.readFileService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	job, err := h.jobs.Start(service.JobKindSync, -1, func(ctx context.Context, set func(done, changed int64)) (any, error) {
		names, err := svc.PostBooks(ctx)
		if err != nil {
			return nil, err
		}
		set(int64(len(names)), int64(len(names)))
		return map[string]any{"imported": names}, nil
	})
	if err != nil {
		if errors.Is(err, service.ErrJobRunning) {
			writeError(w, http.StatusConflict, "another job is already running")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeAccepted(w, map[string]string{"jobId": job.ID})
}

// Reprocess starts a background text cleanup of every stored page:
// POST /api/book/reprocess -> 202 {jobId}.
func (h *AdminHandler) Reprocess(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	total, err := h.pages.CountPages(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	job, err := h.jobs.Start(service.JobKindReprocess, total, func(ctx context.Context, set func(done, changed int64)) (any, error) {
		return service.ReprocessPageTexts(ctx, h.pages, false, set)
	})
	if err != nil {
		if errors.Is(err, service.ErrJobRunning) {
			writeError(w, http.StatusConflict, "another job is already running")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeAccepted(w, map[string]string{"jobId": job.ID})
}

// JobStatus reports one job: GET /api/book/jobs/{id}.
func (h *AdminHandler) JobStatus(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	job, ok := h.jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, job)
}

// Jobs lists kept jobs, newest first: GET /api/book/jobs.
func (h *AdminHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	writeJSON(w, h.jobs.List())
}

// DryRun scans stored pages without writing:
// GET /api/book/reprocess/dry-run -> {total, changed, samples}.
func (h *AdminHandler) DryRun(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	result, err := service.ReprocessPageTexts(r.Context(), h.pages, true, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
