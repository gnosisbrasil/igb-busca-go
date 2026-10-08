package service

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"igb-busca-go/model"
	"igb-busca-go/repository"
)

// ValidPerfis for book uploads (same vocabulary as the Drive import).
var ValidPerfis = []string{"publico", "segundacamara", "sacerdotal"}

// ErrBookExists is returned when the uploaded file name is already indexed.
var ErrBookExists = errors.New("book already exists")

// UploadService imports a single PDF sent by the client (new endpoint,
// no .NET counterpart). Unlike the Drive import it needs no Credencial.json.
type UploadService struct {
	books repository.BookStore
	pages repository.PageStore
}

// NewUploadService creates an UploadService.
func NewUploadService(books repository.BookStore, pages repository.PageStore) *UploadService {
	return &UploadService{books: books, pages: pages}
}

// ValidPerfil reports whether perfil is an accepted book perfil.
func ValidPerfil(perfil string) bool {
	for _, p := range ValidPerfis {
		if p == perfil {
			return true
		}
	}
	return false
}

// ImportUpload stores the PDF as a new book and indexes its pages.
// The file name receives the same sanitization as the Drive import
// (single quotes stripped); uploads have no Drive file, so driveId is "".
func (s *UploadService) ImportUpload(ctx context.Context, fileName, perfil string, src io.Reader) (model.UploadResult, error) {
	name := strings.ReplaceAll(fileName, "'", "")

	existing, err := s.books.GetBookByName(ctx, name)
	if err != nil {
		return model.UploadResult{}, err
	}
	if existing.ID > 0 {
		return model.UploadResult{}, ErrBookExists
	}

	tmp, err := os.CreateTemp("", "igb-upload-*.pdf")
	if err != nil {
		return model.UploadResult{}, err
	}
	tmpPath := tmp.Name()
	_, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if copyErr != nil {
			return model.UploadResult{}, copyErr
		}
		return model.UploadResult{}, closeErr
	}
	defer os.Remove(tmpPath)

	if err := s.books.InsertBook(ctx, name, perfil, ""); err != nil {
		return model.UploadResult{}, err
	}

	book, err := s.books.GetBookByName(ctx, name)
	if err != nil {
		return model.UploadResult{}, err
	}
	if book.ID <= 0 {
		return model.UploadResult{}, errors.New("book insert did not persist")
	}

	texts, err := extractPDFPages(tmpPath)
	if err != nil {
		return model.UploadResult{}, err
	}
	for i, text := range texts {
		if err := s.pages.InsertPage(ctx, book.ID, int64(i+1), strings.ReplaceAll(text, "'", "")); err != nil {
			return model.UploadResult{}, err
		}
	}

	return model.UploadResult{Name: name, Perfil: perfil, Pages: len(texts)}, nil
}
