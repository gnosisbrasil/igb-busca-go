package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"igb-busca-go/model"
)

type stubBookStore struct {
	byName      map[string]model.Book
	inserted    [][3]string
	nextID      int64
	getErr      error
	insertErr   error
	byPerfil    []model.Book
	byPerfilErr error
}

func (s *stubBookStore) GetBooks(context.Context) ([]model.Book, error) { return nil, s.getErr }
func (s *stubBookStore) GetBookByName(_ context.Context, name string) (model.Book, error) {
	if s.getErr != nil {
		return model.Book{}, s.getErr
	}
	return s.byName[name], nil
}
func (s *stubBookStore) GetBookByPerfil(context.Context, []string) ([]model.Book, error) {
	return s.byPerfil, s.byPerfilErr
}
func (s *stubBookStore) InsertBook(_ context.Context, name, perfil, driveID string) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.inserted = append(s.inserted, [3]string{name, perfil, driveID})
	s.nextID++
	s.byName[name] = model.Book{ID: s.nextID, Name: &name, Perfil: &perfil, DriveID: &driveID}
	return nil
}

type stubPageStore struct {
	inserted  [][3]any
	insertErr error
}

func (s *stubPageStore) InsertPage(_ context.Context, bookID, pageNumber int64, text string) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.inserted = append(s.inserted, [3]any{bookID, pageNumber, text})
	return nil
}
func (s *stubPageStore) GetPageByText(context.Context, string, []int64, int) ([]model.BookPage, error) {
	return nil, nil
}
func (s *stubPageStore) GetPageByNumber(context.Context, int64, int64) (*model.BookPage, error) {
	return nil, nil
}

func TestImportUploadSuccess(t *testing.T) {
	books := &stubBookStore{byName: map[string]model.Book{}}
	pages := &stubPageStore{}
	svc := NewUploadService(books, pages)

	f, err := os.Open(buildTestPDF(t, []string{"Pagina um", "Pagina dois"}))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	res, err := svc.ImportUpload(context.Background(), "Livro Teste.pdf", "sacerdotal", f)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Name != "Livro Teste.pdf" || res.Perfil != "sacerdotal" || res.Pages != 2 {
		t.Fatalf("result = %+v", res)
	}
	if len(books.inserted) != 1 || books.inserted[0] != [3]string{"Livro Teste.pdf", "sacerdotal", ""} {
		t.Fatalf("books inserted = %v", books.inserted)
	}
	if len(pages.inserted) != 2 {
		t.Fatalf("pages inserted = %d, want 2", len(pages.inserted))
	}
	if pages.inserted[0][1] != int64(1) || pages.inserted[1][1] != int64(2) {
		t.Fatalf("page numbers = %v", pages.inserted)
	}
	if !strings.Contains(pages.inserted[0][2].(string), "Pagina um") {
		t.Fatalf("page 1 text = %q", pages.inserted[0][2])
	}
}

func TestImportUploadDuplicate(t *testing.T) {
	name := "Existe.pdf"
	books := &stubBookStore{byName: map[string]model.Book{name: {ID: 5}}}
	svc := NewUploadService(books, &stubPageStore{})

	f, err := os.Open(buildTestPDF(t, []string{"x"}))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := svc.ImportUpload(context.Background(), name, "publico", f); !errors.Is(err, ErrBookExists) {
		t.Fatalf("err = %v, want ErrBookExists", err)
	}
	if len(books.inserted) != 0 {
		t.Fatalf("should not insert duplicates")
	}
}

func TestImportUploadStripsQuotes(t *testing.T) {
	books := &stubBookStore{byName: map[string]model.Book{}}
	svc := NewUploadService(books, &stubPageStore{})

	f, err := os.Open(buildTestPDF(t, []string{"x"}))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	res, err := svc.ImportUpload(context.Background(), "Li'vro.pdf", "publico", f)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Name != "Livro.pdf" {
		t.Fatalf("name = %q", res.Name)
	}
}

func TestImportUploadInvalidPDF(t *testing.T) {
	books := &stubBookStore{byName: map[string]model.Book{}}
	svc := NewUploadService(books, &stubPageStore{})

	if _, err := svc.ImportUpload(context.Background(), "x.pdf", "publico", strings.NewReader("not a pdf")); err == nil {
		t.Fatal("expected error for invalid pdf")
	}
}

func TestValidPerfil(t *testing.T) {
	for _, p := range []string{"publico", "segundacamara", "sacerdotal"} {
		if !ValidPerfil(p) {
			t.Fatalf("%q should be valid", p)
		}
	}
	for _, p := range []string{"", "admin", "Publico", " sacerdotal"} {
		if ValidPerfil(p) {
			t.Fatalf("%q should be invalid", p)
		}
	}
}
