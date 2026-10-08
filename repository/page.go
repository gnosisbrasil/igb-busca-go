package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"igb-busca-go/model"
)

// PageStore mirrors IPageRepository.
type PageStore interface {
	InsertPage(ctx context.Context, bookID, pageNumber int64, pageText string) error
	GetPageByText(ctx context.Context, pageText string, bookIDs []int64, limit int) ([]model.BookPage, error)
	GetPageByNumber(ctx context.Context, bookID, pageNumber int64) (*model.BookPage, error)
}

// PageRepository mirrors PageRepository.cs.
type PageRepository struct {
	pool *pgxpool.Pool
}

// NewPageRepository creates a PageRepository.
func NewPageRepository(pool *pgxpool.Pool) *PageRepository {
	return &PageRepository{pool: pool}
}

// InsertPage mirrors InsertPageAsync.
func (r *PageRepository) InsertPage(ctx context.Context, bookID, pageNumber int64, pageText string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO gnosis.pages (book_id, page_number, page_text) VALUES ($1, $2, $3)`,
		bookID, pageNumber, pageText)
	return err
}

// errEmptyBookIDs replicates the original behavior when no books match the
// perfiles: MySQL receives `IN()` which is a syntax error, surfacing as
// HTTP 500 from the controller.
var errEmptyBookIDs = errors.New("empty book id list")

// GetPageByText mirrors GetPageByTextAsync.
//
// Original MySQL semantics: LIKE '%text%' (case/accent-insensitive via
// utf8mb4_0900_ai_ci), or REGEXP '(^|[^a-zA-Z0-9])text([^a-zA-Z0-9]|$)'
// when the text is wrapped in double quotes. PostgreSQL port uses ILIKE
// (case-insensitive) and ~* (case-insensitive regex); accent-insensitivity
// is the one documented collation difference (see README).
//
// Improvement over both: the column is stripped of hyphen-like characters
// (hyphen-minus, soft hyphen U+00AD, U+2010, U+2011) so "contraindo-os"
// matches every stored spelling; the query side is stripped in the
// service layer (StripHyphens), keeping % wildcards intact.
func (r *PageRepository) GetPageByText(ctx context.Context, pageText string, bookIDs []int64, limit int) ([]model.BookPage, error) {
	if len(bookIDs) == 0 {
		return nil, errEmptyBookIDs
	}

	operator := "ILIKE"
	begin := "%"
	end := "%"
	if strings.HasPrefix(pageText, `"`) && strings.HasSuffix(pageText, `"`) {
		operator = "~*"
		begin = `(^|[^a-zA-Z0-9])`
		end = `([^a-zA-Z0-9]|$)`
		pageText = strings.ReplaceAll(pageText, `"`, "")
	}

	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", limit)
	}

	// chr(173/8208/8209) = soft hyphen, U+2010, U+2011: written as
	// chr() calls so the source holds no invisible characters.
	query := fmt.Sprintf(`SELECT b.id as bookid,
		b.name as bookname,
		b.driveid as driveid,
		p.page_number as pagenumber,
		p.page_text as pagetext
	FROM gnosis.books b
	INNER JOIN gnosis.pages p on b.id = p.book_id
	WHERE p.book_id = ANY($1)
		AND regexp_replace(p.page_text, '[-' || chr(173) || chr(8208) || chr(8209) || ']', '', 'g') %s $2
	%s`, operator, limitClause)

	rows, err := r.pool.Query(ctx, query, bookIDs, begin+pageText+end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pages := []model.BookPage{}
	for rows.Next() {
		var p model.BookPage
		if err := rows.Scan(&p.BookID, &p.BookName, &p.DriveID, &p.PageNumber, &p.PageText); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// PageTextRow is one stored page for the reprocess scan.
type PageTextRow struct {
	ID         int64
	BookID     int64
	BookName   string
	PageNumber int64
	Text       string
}

// CountPages returns the total page rows (new endpoint support,
// no .NET counterpart).
func (r *PageRepository) CountPages(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM gnosis.pages`).Scan(&n)
	return n, err
}

// ListPageTexts returns page rows ordered by id (new endpoint support,
// no .NET counterpart).
func (r *PageRepository) ListPageTexts(ctx context.Context, limit, offset int64) ([]PageTextRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.id, p.book_id, b.name, p.page_number, p.page_text
		FROM gnosis.pages p
		JOIN gnosis.books b ON b.id = p.book_id
		ORDER BY p.id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PageTextRow{}
	for rows.Next() {
		var row PageTextRow
		if err := rows.Scan(&row.ID, &row.BookID, &row.BookName, &row.PageNumber, &row.Text); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// UpdatePageText rewrites one stored page text (new endpoint support,
// no .NET counterpart).
func (r *PageRepository) UpdatePageText(ctx context.Context, id int64, text string) error {
	_, err := r.pool.Exec(ctx, `UPDATE gnosis.pages SET page_text = $1 WHERE id = $2`, text, id)
	return err
}

// GetPageByNumber mirrors GetPageByNumberAsync (nil when not found,
// error on real DB failures just like QueryFirstOrDefaultAsync throws).
func (r *PageRepository) GetPageByNumber(ctx context.Context, bookID, pageNumber int64) (*model.BookPage, error) {
	var p model.BookPage
	err := r.pool.QueryRow(ctx,
		`SELECT b.id as bookid,
			b.name as bookname,
			b.driveid as driveid,
			p.page_number as pagenumber,
			p.page_text as pagetext
		FROM gnosis.books b
		INNER JOIN gnosis.pages p on b.id = p.book_id
		WHERE p.book_id = $1
			AND p.page_number = $2`,
		bookID, pageNumber,
	).Scan(&p.BookID, &p.BookName, &p.DriveID, &p.PageNumber, &p.PageText)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}
