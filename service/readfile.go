package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"igb-busca-go/model"
	"igb-busca-go/repository"
)

// Drive link templates from ReadFileService.
const (
	driveFileURL = "https://drive.google.com/file/d/%s/view"
	fallbackURL  = "https://gnosisbrasil.com/livrosgnosticos"
)

// credentialFile mirrors @"./Credencial.json" (same spelling).
const credentialFile = "./Credencial.json"

// importTempFile mirrors the "file.pdf" temp download of the original.
const importTempFile = "file.pdf"

// ReadFileService mirrors ReadFileService.cs.
type ReadFileService struct {
	books     repository.BookStore
	pages     repository.PageStore
	analytics repository.AnalyticsStore
}

// NewReadFileService creates a ReadFileService. Like the .NET Transient
// service, the Drive client is built per service instance; a missing or
// invalid Credencial.json fails book routes with HTTP 500, as in the
// original (constructor throw -> unhandled exception).
func NewReadFileService(books repository.BookStore, pages repository.PageStore, analytics repository.AnalyticsStore) (*ReadFileService, error) {
	if _, err := newDriveService(context.Background()); err != nil {
		return nil, err
	}
	return &ReadFileService{books: books, pages: pages, analytics: analytics}, nil
}

func newDriveService(ctx context.Context) (*drive.Service, error) {
	cred, err := os.ReadFile(credentialFile)
	if err != nil {
		return nil, err
	}
	conf, err := google.JWTConfigFromJSON(cred, drive.DriveScope)
	if err != nil {
		return nil, err
	}
	return drive.NewService(ctx, option.WithHTTPClient(conf.Client(ctx)))
}

// PostBooks mirrors PostBooksAsync: imports new Drive PDFs into books+pages,
// returning the names of the imported files.
func (s *ReadFileService) PostBooks(ctx context.Context) ([]string, error) {
	resultBooks := []string{}

	stored, err := s.books.GetBooks(ctx)
	if err != nil {
		return nil, err
	}
	books := make([]string, 0, len(stored))
	for _, b := range stored {
		if b.Name != nil {
			books = append(books, *b.Name)
		}
	}
	sort.Strings(books)

	driveService, err := newDriveService(ctx)
	if err != nil {
		return nil, err
	}

	var driveFiles []*drive.File
	var pageToken string
	for {
		// Server-side filter: the import only ever uses PDFs, so non-PDF
		// and trashed files are not even listed (the original listed the
		// whole Drive and skipped them client-side).
		req := driveService.Files.List().
			Q("mimeType='application/pdf' and trashed=false").
			Fields("nextPageToken, files(id, name, parents, mimeType)")
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}
		result, err := req.Do()
		if err != nil {
			return nil, err
		}
		if result.Files == nil {
			break
		}
		driveFiles = append(driveFiles, result.Files...)
		log.Printf("Total de registros PDF: %d", len(result.Files))
		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	if len(driveFiles) == 0 {
		return resultBooks, nil
	}

	sort.Slice(driveFiles, func(i, j int) bool { return driveFiles[i].Name < driveFiles[j].Name })

	cache := map[string]*drive.File{}
	for _, file := range driveFiles {
		fileName := strings.ReplaceAll(file.Name, "'", "")
		log.Printf("fileName: %s", fileName)
		if !contains(books, fileName) && file.MimeType == "application/pdf" {
			name, err := s.importFile(ctx, driveService, cache, file, fileName)
			if err != nil {
				log.Printf("Erro ao inserir registros do livro %s: %s", fileName, err.Error())
				return nil, err
			}
			books = append(books, fileName)
			resultBooks = append(resultBooks, name)
		}
	}

	return resultBooks, nil
}

func (s *ReadFileService) importFile(ctx context.Context, driveService *drive.Service, cache map[string]*drive.File, file *drive.File, fileName string) (string, error) {
	absPath, err := absPath(driveService, cache, file)
	if err != nil {
		return "", err
	}

	perfil := "sacerdotal"
	switch {
	case strings.Contains(absPath, "publico"):
		perfil = "publico"
	case strings.Contains(absPath, "segundacamara"):
		perfil = "segundacamara"
	}

	driveID := ""
	if strings.Contains(absPath, "downloadlink") {
		driveID = file.Id
	}

	if err := downloadFile(driveService, file.Id); err != nil {
		return "", err
	}

	if err := s.books.InsertBook(ctx, fileName, perfil, driveID); err != nil {
		return "", err
	}

	book, err := s.books.GetBookByName(ctx, fileName)
	if err != nil {
		return "", err
	}

	if book.ID > 0 {
		texts, err := extractPDFPages(importTempFile)
		if err != nil {
			return "", err
		}
		for i, text := range texts {
			if err := s.pages.InsertPage(ctx, book.ID, int64(i+1), strings.ReplaceAll(text, "'", "")); err != nil {
				return "", err
			}
		}
	}

	return fileName, nil
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func downloadFile(driveService *drive.Service, fileID string) error {
	resp, err := driveService.Files.Get(fileID).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(importTempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// absPath mirrors AbsPath: Drive folder path of the file, root excluded.
func absPath(driveService *drive.Service, cache map[string]*drive.File, file *drive.File) (string, error) {
	if len(file.Parents) == 0 {
		return file.Name, nil
	}

	path := []string{}
	for {
		parent, err := getParent(driveService, cache, file.Parents[0])
		if err != nil {
			return "", err
		}
		if len(parent.Parents) == 0 {
			break
		}
		path = append([]string{parent.Name}, path...)
		file = parent
	}
	path = append(path, file.Name)
	return strings.Join(path, string(os.PathSeparator)), nil
}

// getParent mirrors GetParent with the same in-memory cache.
func getParent(driveService *drive.Service, cache map[string]*drive.File, id string) (*drive.File, error) {
	if parent, ok := cache[id]; ok {
		return parent, nil
	}
	parent, err := driveService.Files.Get(id).Fields("name,parents").Do()
	if err != nil {
		return nil, err
	}
	cache[id] = parent
	return parent, nil
}

// extractPDFPages mirrors the PdfPig ContentOrderTextExtractor loop,
// returning one text per page (1-based order). The Go PDF library differs
// in layout heuristics; see README. Each page is normalized with
// CleanPageText (hyphenation, ligatures, unicode) before returning.
func extractPDFPages(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	reader, err := pdf.NewReader(f, info.Size())
	if err != nil {
		return nil, err
	}

	texts := make([]string, 0, reader.NumPage())
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			texts = append(texts, "")
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i, err)
		}
		var sb strings.Builder
		for _, row := range rows {
			fragments := make([]string, 0, len(row.Content))
			for _, cell := range row.Content {
				fragments = append(fragments, cell.S)
			}
			sb.WriteString(strings.Join(fragments, " "))
			sb.WriteString("\n")
		}
		texts = append(texts, CleanPageText(sb.String()))
	}
	return texts, nil
}

// GetBooks mirrors GetBooksAsync.
func (s *ReadFileService) GetBooks(ctx context.Context) ([]model.Book, error) {
	return s.books.GetBooks(ctx)
}

// visiblePerfis mirrors the perfil expansion in both search methods:
// publico always, plus segundacamara for segundacamara/sacerdotal,
// plus sacerdotal for sacerdotal.
func visiblePerfis(perfil string) []string {
	perfis := []string{"publico"}
	if perfil == "segundacamara" {
		perfis = append(perfis, perfil)
	} else if perfil == "sacerdotal" {
		perfis = append(perfis, "segundacamara", perfil)
	}
	return perfis
}

// GetPageByText mirrors GetPageByTextAsync.
func (s *ReadFileService) GetPageByText(ctx context.Context, perfil, pageText string, limit int) ([]model.BookPage, error) {
	perfis := visiblePerfis(perfil)

	if err := s.analytics.InsertSearchWordAnalytics(ctx, perfil, pageText); err != nil {
		return nil, err
	}

	books, err := s.books.GetBookByPerfil(ctx, perfis)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(books))
	for _, b := range books {
		ids = append(ids, b.ID)
	}

	// "+" is the wildcard (historical); hyphens are stripped on both
	// sides so "contraindo-os" also matches soft-hyphen spellings.
	bookPages, err := s.pages.GetPageByText(ctx, StripHyphens(strings.ReplaceAll(pageText, "+", "%")), ids, limit)
	if err != nil {
		return nil, err
	}

	for i := range bookPages {
		bookPages[i].DriveID = strPtr(mapDriveURL(bookPages[i].DriveID))
	}

	return bookPages, nil
}

// errPageNotFound replicates the NullReferenceException of the original
// when the page row is missing (book allowed, page absent -> HTTP 500).
var errPageNotFound = errors.New("page not found")

// GetPageByNumber mirrors GetPageByNumberAsync (nil, nil when the book is
// outside the caller's perfil -> HTTP 200 null).
func (s *ReadFileService) GetPageByNumber(ctx context.Context, perfil string, bookID, pageNumber int64) (*model.BookPage, error) {
	perfis := visiblePerfis(perfil)

	books, err := s.books.GetBookByPerfil(ctx, perfis)
	if err != nil {
		return nil, err
	}

	allowed := false
	for _, b := range books {
		if b.ID == bookID {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, nil
	}

	bookPage, err := s.pages.GetPageByNumber(ctx, bookID, pageNumber)
	if err != nil {
		return nil, err
	}
	if bookPage == nil {
		return nil, errPageNotFound
	}

	bookPage.DriveID = strPtr(mapDriveURL(bookPage.DriveID))

	return bookPage, nil
}

// mapDriveURL mirrors the DriveId link mapping in both search methods.
func mapDriveURL(driveID *string) string {
	if driveID != nil && *driveID != "" {
		return fmt.Sprintf(driveFileURL, *driveID)
	}
	return fallbackURL
}

func strPtr(s string) *string { return &s }
