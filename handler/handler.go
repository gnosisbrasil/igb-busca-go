// Package handler mirrors SuperGnosis.Api Controllers.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"igb-busca-go/middleware"
	"igb-busca-go/model"
	"igb-busca-go/repository"
	"igb-busca-go/service"
)

// defaultMaxUploadBytes caps PDF uploads (100 MB).
const defaultMaxUploadBytes = 100 << 20

// writeJSON mirrors Ok(object): 200 + application/json; charset=utf-8.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// writeError is the JSON error envelope of the new endpoints.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: msg})
}

// AuthHandler mirrors AuthController (api/auth).
type AuthHandler struct {
	users  repository.UserStore
	tokens *service.TokenService
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(users repository.UserStore, tokens *service.TokenService) *AuthHandler {
	return &AuthHandler{users: users, tokens: tokens}
}

// loginBody mirrors the [FromBody] User binding (case-insensitive keys,
// null-safe comparison like C# string ==).
type loginBody struct {
	Name  *string `json:"name"`
	Senha *string `json:"senha"`
}

func strEquals(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Login mirrors Login: POST api/auth/login -> {token} or 401.
// Unparseable/missing body -> 400 like [ApiController] binding.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	users, err := h.users.GetUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, u := range users {
		if strEquals(u.Name, body.Name) && strEquals(u.Senha, body.Senha) {
			token, err := h.tokens.GenerateToken(strconv.FormatInt(u.ID, 10))
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeJSON(w, model.LoginResponse{Token: token})
			return
		}
	}

	w.WriteHeader(http.StatusUnauthorized)
}

// Me reports the caller identity: GET api/auth/me -> {name, perfil}
// (new endpoint, no .NET counterpart). Anonymous callers get 401.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	name, perfil := "", ""
	if user.Name != nil {
		name = *user.Name
	}
	if user.Perfil != nil {
		perfil = *user.Perfil
	}
	writeJSON(w, map[string]string{"name": name, "perfil": perfil})
}

// BookHandler mirrors BookController (api/book), plus the new Upload.
type BookHandler struct {
	books          repository.BookStore
	pages          repository.PageStore
	analytics      repository.AnalyticsStore
	maxUploadBytes int64
}

// NewBookHandler creates a BookHandler. The ReadFileService is built per
// request like the .NET Transient service (missing Credencial.json -> 500
// on every ported book route, mirroring the constructor throw).
func NewBookHandler(books repository.BookStore, pages repository.PageStore, analytics repository.AnalyticsStore) *BookHandler {
	return &BookHandler{books: books, pages: pages, analytics: analytics, maxUploadBytes: defaultMaxUploadBytes}
}

func (h *BookHandler) readFileService() (*service.ReadFileService, error) {
	return service.NewReadFileService(h.books, h.pages, h.analytics)
}

// PostBooks mirrors PostBooksAsync: POST api/book (sacerdotal only).
func (h *BookHandler) PostBooks(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	svc, err := h.readFileService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result, err := svc.PostBooks(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// GetBooks mirrors GetBooksAsync: GET api/book/all (sacerdotal only).
func (h *BookHandler) GetBooks(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	svc, err := h.readFileService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result, err := svc.GetBooks(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// GetPageByText mirrors GetPageByTextAsync: GET api/book/search-word.
// limit is a required 32-bit int (missing/invalid -> 400); a missing
// pageText inserts an analytics row and then fails with 500, exactly like
// the NullReferenceException in the original.
func (h *BookHandler) GetPageByText(w http.ResponseWriter, r *http.Request) {
	perfil := "publico"
	if p := middleware.PerfilOf(middleware.UserFromContext(r.Context())); p != "" {
		perfil = p
	}

	limitRaw := r.URL.Query().Get("limit")
	limit64, err := strconv.ParseInt(limitRaw, 10, 32)
	if limitRaw == "" || err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pageTexts, present := r.URL.Query()["pageText"]
	if !present {
		if _, svcErr := h.readFileService(); svcErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = h.analytics.InsertSearchWordAnalytics(r.Context(), perfil, "")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	svc, err := h.readFileService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result, err := svc.GetPageByText(r.Context(), perfil, pageTexts[0], int(limit64))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// GetPageByNumber mirrors GetPageByNumberAsync:
// GET api/book/{bookId}/page/{pageNumber} (64-bit ids, invalid -> 400).
func (h *BookHandler) GetPageByNumber(w http.ResponseWriter, r *http.Request) {
	perfil := "publico"
	if p := middleware.PerfilOf(middleware.UserFromContext(r.Context())); p != "" {
		perfil = p
	}

	bookID, err := strconv.ParseInt(r.PathValue("bookId"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	pageNumber, err := strconv.ParseInt(r.PathValue("pageNumber"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	svc, err := h.readFileService()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result, err := svc.GetPageByNumber(r.Context(), perfil, bookID, pageNumber)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// Upload imports one PDF sent as multipart form (new endpoint, no .NET
// counterpart): POST api/book/upload with fields file (.pdf) and perfil.
// Sacerdotal only (400 empty otherwise, like every admin guard); needs no
// Credencial.json. Validation failures use JSON errors for the future
// frontend: 400/409/413 {"error":"..."}.
func (h *BookHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the 100 MB limit")
		} else {
			writeError(w, http.StatusBadRequest, "multipart form with file is required")
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		writeError(w, http.StatusBadRequest, "only .pdf files are accepted")
		return
	}

	perfil := r.FormValue("perfil")
	if !service.ValidPerfil(perfil) {
		writeError(w, http.StatusBadRequest, "perfil must be one of: publico, segundacamara, sacerdotal")
		return
	}

	svc := service.NewUploadService(h.books, h.pages)
	result, err := svc.ImportUpload(r.Context(), header.Filename, perfil, file)
	if err != nil {
		if errors.Is(err, service.ErrBookExists) {
			writeError(w, http.StatusConflict, "book already exists")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// AnalyticsHandler mirrors SearchWordAnalyticsController.
type AnalyticsHandler struct {
	svc *service.AnalyticsService
}

// NewAnalyticsHandler creates an AnalyticsHandler.
func NewAnalyticsHandler(svc *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// GetAll mirrors GetBooksAsync: GET api/search-word-analytics/all
// (sacerdotal only).
func (h *AnalyticsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if middleware.PerfilOf(middleware.UserFromContext(r.Context())) != "sacerdotal" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	result, err := h.svc.GetAll(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
