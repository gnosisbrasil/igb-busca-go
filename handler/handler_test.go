package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"igb-busca-go/middleware"
	"igb-busca-go/model"
	"igb-busca-go/service"
)

const testSecret = "test-secret"

// ---------- stubs ----------

type stubUsers struct {
	users []model.User
	byID  map[int64]model.User
	err   error
}

func (s stubUsers) GetUsers(context.Context) ([]model.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.users, nil
}

func (s stubUsers) GetUserByID(_ context.Context, id int64) (model.User, error) {
	if s.err != nil {
		return model.User{}, s.err
	}
	u, ok := s.byID[id]
	if !ok {
		return model.User{}, nil
	}
	return u, nil
}

type stubBooks struct {
	books  []model.Book
	byName map[string]model.Book
	err    error
	got    *[][]string
}

func (s stubBooks) GetBooks(context.Context) ([]model.Book, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.books, nil
}

func (s stubBooks) GetBookByName(_ context.Context, name string) (model.Book, error) {
	if s.err != nil {
		return model.Book{}, s.err
	}
	return s.byName[name], nil
}

func (s stubBooks) GetBookByPerfil(_ context.Context, perfis []string) ([]model.Book, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.got != nil {
		*s.got = append(*s.got, perfis)
	}
	return s.books, nil
}

func (s stubBooks) InsertBook(_ context.Context, name, perfil, driveID string) error {
	if s.err != nil {
		return s.err
	}
	if s.byName != nil {
		maxID := int64(0)
		for _, b := range s.byName {
			if b.ID > maxID {
				maxID = b.ID
			}
		}
		s.byName[name] = model.Book{ID: maxID + 1, Name: &name, Perfil: &perfil, DriveID: &driveID}
	}
	return nil
}

type stubPages struct {
	byText   []model.BookPage
	byNumber *model.BookPage
	err      error
	gotText  *[]string
	gotIDs   *[][]int64
	gotLimit *[]int
}

func (s stubPages) InsertPage(context.Context, int64, int64, string) error { return s.err }

func (s stubPages) GetPageByText(_ context.Context, text string, ids []int64, limit int) ([]model.BookPage, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.gotText != nil {
		*s.gotText = append(*s.gotText, text)
	}
	if s.gotIDs != nil {
		*s.gotIDs = append(*s.gotIDs, ids)
	}
	if s.gotLimit != nil {
		*s.gotLimit = append(*s.gotLimit, limit)
	}
	return s.byText, nil
}

func (s stubPages) GetPageByNumber(context.Context, int64, int64) (*model.BookPage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byNumber, nil
}

type stubAnalytics struct {
	list     []model.SearchWordAnalytics
	err      error
	inserted *[][2]string
}

func (s stubAnalytics) GetSearchWordAnalytics(context.Context) ([]model.SearchWordAnalytics, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

func (s stubAnalytics) InsertSearchWordAnalytics(_ context.Context, perfil, word string) error {
	if s.inserted != nil {
		*s.inserted = append(*s.inserted, [2]string{perfil, word})
	}
	return s.err
}

// ---------- helpers ----------

func strp(s string) *string { return &s }

func testUsers() stubUsers {
	return stubUsers{
		users: []model.User{
			{ID: 1, Name: strp("ana"), Perfil: strp("publico"), Senha: strp("pw1")},
			{ID: 2, Name: strp("bob"), Perfil: strp("sacerdotal"), Senha: strp("pw2")},
		},
		byID: map[int64]model.User{
			1: {ID: 1, Name: strp("ana"), Perfil: strp("publico"), Senha: strp("pw1")},
			2: {ID: 2, Name: strp("bob"), Perfil: strp("sacerdotal"), Senha: strp("pw2")},
		},
	}
}

// withWorkdir runs the test in a temp dir, optionally containing a parseable
// (offline) Credencial.json so ReadFileService construction succeeds.
func withWorkdir(t *testing.T, withCreds bool) {
	t.Helper()
	t.Chdir(t.TempDir())
	if !withCreds {
		return
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cred, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "test@test.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
	})
	if err := os.WriteFile("Credencial.json", cred, 0600); err != nil {
		t.Fatal(err)
	}
}

type testApp struct {
	server *httptest.Server
	tokens *service.TokenService
}

func newTestApp(users stubUsers, books stubBooks, pages stubPages, analytics stubAnalytics) *testApp {
	tokens := service.NewTokenService(testSecret)
	authHandler := NewAuthHandler(users, tokens)
	bookHandler := NewBookHandler(books, pages, analytics)
	analyticsHandler := NewAnalyticsHandler(service.NewAnalyticsService(analytics))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/book", bookHandler.PostBooks)
	mux.HandleFunc("POST /api/book/upload", bookHandler.Upload)
	mux.HandleFunc("GET /api/book/all", bookHandler.GetBooks)
	mux.HandleFunc("GET /api/book/search-word", bookHandler.GetPageByText)
	mux.HandleFunc("GET /api/book/{bookId}/page/{pageNumber}", bookHandler.GetPageByNumber)
	mux.HandleFunc("GET /api/search-word-analytics/all", analyticsHandler.GetAll)

	app := middleware.CORS(middleware.JWT(users, tokens)(mux))
	return &testApp{server: httptest.NewServer(app), tokens: tokens}
}

func (a *testApp) token(t *testing.T, userID string) string {
	t.Helper()
	token, err := a.tokens.GenerateToken(userID)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func do(t *testing.T, method, url, body, token string) (int, string) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(data))
}

// ---------- auth ----------

func TestLoginSuccess(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	code, body := do(t, "POST", app.server.URL+"/api/auth/login", `{"name":"ana","senha":"pw1"}`, "")
	if code != 200 {
		t.Fatalf("code = %d, body = %s", code, body)
	}
	var resp model.LoginResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Token == "" {
		t.Fatalf("bad login body: %s", body)
	}
	claims, err := app.tokens.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("token does not validate: %v", err)
	}
	if claims["id"] != "1" {
		t.Fatalf("id claim = %v", claims["id"])
	}
}

func TestLoginFailures(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	if code, _ := do(t, "POST", app.server.URL+"/api/auth/login", `{"name":"ana","senha":"nope"}`, ""); code != 401 {
		t.Fatalf("wrong password code = %d, want 401", code)
	}
	if code, _ := do(t, "POST", app.server.URL+"/api/auth/login", `{"name":"ghost","senha":"x"}`, ""); code != 401 {
		t.Fatalf("unknown user code = %d, want 401", code)
	}
	if code, _ := do(t, "POST", app.server.URL+"/api/auth/login", ``, ""); code != 400 {
		t.Fatalf("empty body code = %d, want 400", code)
	}
	if code, _ := do(t, "POST", app.server.URL+"/api/auth/login", `not-json`, ""); code != 400 {
		t.Fatalf("invalid json code = %d, want 400", code)
	}
}

func TestLoginCaseInsensitiveKeys(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	if code, _ := do(t, "POST", app.server.URL+"/api/auth/login", `{"Name":"bob","Senha":"pw2"}`, ""); code != 200 {
		t.Fatalf("capitalized keys code = %d, want 200", code)
	}
}

// ---------- book guards ----------

func TestBookAllGuards(t *testing.T) {
	withWorkdir(t, true)
	books := stubBooks{books: []model.Book{{ID: 1, Name: strp("b.pdf"), Perfil: strp("publico"), DriveID: strp("")}}}
	app := newTestApp(testUsers(), books, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/book/all", "", app.token(t, "2"))
	if code != 200 {
		t.Fatalf("sacerdotal code = %d", code)
	}
	var got []model.Book
	if err := json.Unmarshal([]byte(body), &got); err != nil || len(got) != 1 || *got[0].Name != "b.pdf" {
		t.Fatalf("bad books body: %s", body)
	}
	if !strings.Contains(body, `"driveId"`) {
		t.Fatalf("missing camelCase driveId: %s", body)
	}

	if code, _ := do(t, "GET", app.server.URL+"/api/book/all", "", app.token(t, "1")); code != 400 {
		t.Fatalf("publico code = %d, want 400", code)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/book/all", "", ""); code != 400 {
		t.Fatalf("anonymous code = %d, want 400", code)
	}
}

func TestBookRoutesWithoutCreds(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	if code, _ := do(t, "GET", app.server.URL+"/api/book/all", "", app.token(t, "2")); code != 500 {
		t.Fatalf("code = %d, want 500 without Credencial.json", code)
	}
}

// ---------- search ----------

func TestSearchWord(t *testing.T) {
	withWorkdir(t, true)
	var gotPerfis [][]string
	var gotText []string
	books := stubBooks{
		books: []model.Book{{ID: 7, Name: strp("x.pdf"), Perfil: strp("publico"), DriveID: strp("")}},
		got:   &gotPerfis,
	}
	pages := stubPages{
		byText: []model.BookPage{
			{BookID: 7, BookName: strp("x.pdf"), PageNumber: 3, PageText: strp("text"), DriveID: strp("drv1")},
			{BookID: 7, BookName: strp("x.pdf"), PageNumber: 4, PageText: strp("text"), DriveID: strp("")},
		},
		gotText: &gotText,
	}
	var inserted [][2]string
	app := newTestApp(testUsers(), books, pages, stubAnalytics{inserted: &inserted})
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/book/search-word?pageText=alma+livre&limit=10", "", "")
	if code != 200 {
		t.Fatalf("code = %d, body = %s", code, body)
	}
	// "+" in query decodes to space in both stacks; the service maps "+"->"%"
	// on the already-decoded value.
	if len(gotText) != 1 || gotText[0] != "alma livre" {
		t.Fatalf("pageText passed = %v", gotText)
	}
	if len(gotPerfis) != 1 || len(gotPerfis[0]) != 1 || gotPerfis[0][0] != "publico" {
		t.Fatalf("perfis = %v", gotPerfis)
	}
	if len(inserted) != 1 || inserted[0] != [2]string{"publico", "alma livre"} {
		t.Fatalf("analytics inserted = %v", inserted)
	}
	if !strings.Contains(body, "https://drive.google.com/file/d/drv1/view") {
		t.Fatalf("drive url not mapped: %s", body)
	}
	if !strings.Contains(body, "https://gnosisbrasil.com/livrosgnosticos") {
		t.Fatalf("fallback url missing: %s", body)
	}
}

func TestSearchWordValidation(t *testing.T) {
	withWorkdir(t, true)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	if code, _ := do(t, "GET", app.server.URL+"/api/book/search-word?pageText=x", "", ""); code != 400 {
		t.Fatalf("missing limit code = %d, want 400", code)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/book/search-word?pageText=x&limit=abc", "", ""); code != 400 {
		t.Fatalf("invalid limit code = %d, want 400", code)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/book/search-word?limit=5", "", ""); code != 500 {
		t.Fatalf("missing pageText code = %d, want 500", code)
	}
}

func TestSearchWordMissingPageTextInsertsAnalytics(t *testing.T) {
	withWorkdir(t, true)
	var inserted [][2]string
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{inserted: &inserted})
	defer app.server.Close()

	code, _ := do(t, "GET", app.server.URL+"/api/book/search-word?limit=5", "", app.token(t, "1"))
	if code != 500 {
		t.Fatalf("code = %d, want 500", code)
	}
	if len(inserted) != 1 || inserted[0] != [2]string{"publico", ""} {
		t.Fatalf("analytics inserted = %v", inserted)
	}
}

// ---------- page by number ----------

func TestPageByNumber(t *testing.T) {
	withWorkdir(t, true)
	books := stubBooks{books: []model.Book{{ID: 7}}}
	pages := stubPages{byNumber: &model.BookPage{BookID: 7, BookName: strp("x.pdf"), PageNumber: 2, PageText: strp("t"), DriveID: strp("")}}
	app := newTestApp(testUsers(), books, pages, stubAnalytics{})
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/book/7/page/2", "", "")
	if code != 200 || !strings.Contains(body, `"pageNumber":2`) {
		t.Fatalf("code = %d, body = %s", code, body)
	}

	// book outside perfil -> 200 null
	booksOut := stubBooks{books: []model.Book{{ID: 9}}}
	app2 := newTestApp(testUsers(), booksOut, pages, stubAnalytics{})
	defer app2.server.Close()
	code, body = do(t, "GET", app2.server.URL+"/api/book/7/page/2", "", "")
	if code != 200 || body != "null" {
		t.Fatalf("outside perfil: code = %d, body = %s", code, body)
	}

	// page row missing -> 500 (NullReference parity)
	app3 := newTestApp(testUsers(), books, stubPages{byNumber: nil}, stubAnalytics{})
	defer app3.server.Close()
	if code, _ := do(t, "GET", app3.server.URL+"/api/book/7/page/99", "", ""); code != 500 {
		t.Fatalf("missing page code = %d, want 500", code)
	}

	// invalid ids -> 400
	if code, _ := do(t, "GET", app.server.URL+"/api/book/abc/page/2", "", ""); code != 400 {
		t.Fatalf("bad bookId code = %d, want 400", code)
	}
}

// ---------- analytics ----------

func TestAnalyticsGuards(t *testing.T) {
	withWorkdir(t, false)
	analytics := stubAnalytics{list: []model.SearchWordAnalytics{{ID: 1, Perfil: strp("publico"), SearchWord: strp("alma")}}}
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, analytics)
	defer app.server.Close()

	code, body := do(t, "GET", app.server.URL+"/api/search-word-analytics/all", "", app.token(t, "2"))
	if code != 200 || !strings.Contains(body, `"searchWord":"alma"`) || !strings.Contains(body, `"amount":0`) {
		t.Fatalf("sacerdotal: code = %d, body = %s", code, body)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/search-word-analytics/all", "", app.token(t, "1")); code != 400 {
		t.Fatalf("publico code = %d, want 400", code)
	}
	if code, _ := do(t, "GET", app.server.URL+"/api/search-word-analytics/all", "", ""); code != 400 {
		t.Fatalf("anonymous code = %d, want 400", code)
	}
}

// ---------- middleware ----------

func TestMiddlewareInvalidToken(t *testing.T) {
	withWorkdir(t, true)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	if code, _ := do(t, "GET", app.server.URL+"/api/book/all", "", "bogus.token.here"); code != 500 {
		t.Fatalf("invalid token code = %d, want 500", code)
	}
}

func TestCORS(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	req, _ := http.NewRequest("OPTIONS", app.server.URL+"/api/book/all", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("OPTIONS code = %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS headers")
	}
}

// ---------- upload ----------

// testPDFBytes builds a minimal valid one-page PDF containing line.
func testPDFBytes(line string) []byte {
	var body strings.Builder
	offsets := []int{}
	addObj := func(n int, content string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", n, content)
	}
	body.WriteString("%PDF-1.4\n")
	stream := fmt.Sprintf("BT /F1 24 Tf 100 700 Td (%s) Tj ET", line)
	addObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	addObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	addObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	addObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	addObj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	xrefPos := body.Len()
	fmt.Fprintf(&body, "xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&body, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos)
	return []byte(body.String())
}

func uploadRequest(t *testing.T, url, token, filename string, content []byte, perfil string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if filename != "" {
		fw, err := w.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(content)
	}
	if perfil != "" {
		_ = w.WriteField("perfil", perfil)
	}
	_ = w.Close()
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(data))
}

func TestUploadSuccess(t *testing.T) {
	withWorkdir(t, false) // no Credencial.json needed for uploads
	books := stubBooks{byName: map[string]model.Book{}}
	app := newTestApp(testUsers(), books, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	code, body := uploadRequest(t, app.server.URL+"/api/book/upload", app.token(t, "2"), "novo.pdf", testPDFBytes("conteudo sagradinho"), "segundacamara")
	if code != 200 {
		t.Fatalf("code = %d, body = %s", code, body)
	}
	var res model.UploadResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("bad body: %s", body)
	}
	if res.Name != "novo.pdf" || res.Perfil != "segundacamara" || res.Pages != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestUploadGuards(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{}, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	pdf := testPDFBytes("x")
	if code, body := uploadRequest(t, app.server.URL+"/api/book/upload", "", "a.pdf", pdf, "publico"); code != 400 || body != "" {
		t.Fatalf("anon: code = %d body = %q, want 400 empty", code, body)
	}
	if code, _ := uploadRequest(t, app.server.URL+"/api/book/upload", app.token(t, "1"), "a.pdf", pdf, "publico"); code != 400 {
		t.Fatalf("publico code = %d, want 400", code)
	}
}

func TestUploadValidation(t *testing.T) {
	withWorkdir(t, false)
	app := newTestApp(testUsers(), stubBooks{byName: map[string]model.Book{}}, stubPages{}, stubAnalytics{})
	defer app.server.Close()
	sac := app.token(t, "2")

	pdf := testPDFBytes("x")
	if code, body := uploadRequest(t, app.server.URL+"/api/book/upload", sac, "", pdf, "publico"); code != 400 || !strings.Contains(body, `"error"`) {
		t.Fatalf("missing file: code = %d body = %s", code, body)
	}
	if code, _ := uploadRequest(t, app.server.URL+"/api/book/upload", sac, "a.txt", []byte("x"), "publico"); code != 400 {
		t.Fatalf("non-pdf code = %d, want 400", code)
	}
	if code, _ := uploadRequest(t, app.server.URL+"/api/book/upload", sac, "a.pdf", pdf, ""); code != 400 {
		t.Fatalf("missing perfil code = %d, want 400", code)
	}
	if code, _ := uploadRequest(t, app.server.URL+"/api/book/upload", sac, "a.pdf", pdf, "admin"); code != 400 {
		t.Fatalf("bad perfil code = %d, want 400", code)
	}
	if code, _ := uploadRequest(t, app.server.URL+"/api/book/upload", sac, "a.pdf", []byte("not a pdf"), "publico"); code != 500 {
		t.Fatalf("corrupt pdf code = %d, want 500", code)
	}
}

func TestUploadDuplicate(t *testing.T) {
	withWorkdir(t, false)
	books := stubBooks{byName: map[string]model.Book{"dup.pdf": {ID: 9}}}
	app := newTestApp(testUsers(), books, stubPages{}, stubAnalytics{})
	defer app.server.Close()

	code, body := uploadRequest(t, app.server.URL+"/api/book/upload", app.token(t, "2"), "dup.pdf", testPDFBytes("x"), "publico")
	if code != 409 || !strings.Contains(body, "already exists") {
		t.Fatalf("code = %d body = %s, want 409", code, body)
	}
}

func TestUploadTooLarge(t *testing.T) {
	withWorkdir(t, false)
	tokens := service.NewTokenService(testSecret)
	users := testUsers()
	bh := NewBookHandler(stubBooks{byName: map[string]model.Book{}}, stubPages{}, stubAnalytics{})
	bh.maxUploadBytes = 1024 // 1 KB for the test
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/book/upload", bh.Upload)
	server := httptest.NewServer(middleware.CORS(middleware.JWT(users, tokens)(mux)))
	defer server.Close()
	token, _ := tokens.GenerateToken("2")

	big := bytes.Repeat([]byte("a"), 4096)
	code, body := uploadRequest(t, server.URL+"/api/book/upload", token, "big.pdf", big, "publico")
	if code != 413 || !strings.Contains(body, "100 MB") {
		t.Fatalf("code = %d body = %s, want 413", code, body)
	}
}
