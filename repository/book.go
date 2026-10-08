// Package repository mirrors SuperGnosis.Api Repository classes,
// ported from MySQL (Dapper/MySqlConnector) to PostgreSQL (pgx).
//
// Queries keep the same shape and semantics; parameters are bound
// ($1, $2, ...) instead of string-interpolated.
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"igb-busca-go/model"
)

// BookStore mirrors IBookRepository.
type BookStore interface {
	GetBooks(ctx context.Context) ([]model.Book, error)
	GetBookByName(ctx context.Context, name string) (model.Book, error)
	GetBookByPerfil(ctx context.Context, perfis []string) ([]model.Book, error)
	InsertBook(ctx context.Context, name, perfil, driveID string) error
}

// BookRepository mirrors BookRepository.cs.
type BookRepository struct {
	pool *pgxpool.Pool
}

// NewBookRepository creates a BookRepository.
func NewBookRepository(pool *pgxpool.Pool) *BookRepository {
	return &BookRepository{pool: pool}
}

// GetBooks mirrors GetBooksAsync: SELECT * FROM gnosis.books.
func (r *BookRepository) GetBooks(ctx context.Context) ([]model.Book, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, perfil, driveid FROM gnosis.books`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := []model.Book{}
	for rows.Next() {
		var b model.Book
		if err := rows.Scan(&b.ID, &b.Name, &b.Perfil, &b.DriveID); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// GetBookByName mirrors GetBookByNameAsync (zero Book when not found,
// error on real DB failures just like QueryFirstOrDefaultAsync throws).
func (r *BookRepository) GetBookByName(ctx context.Context, name string) (model.Book, error) {
	var b model.Book
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, perfil, driveid FROM gnosis.books WHERE name = $1`, name,
	).Scan(&b.ID, &b.Name, &b.Perfil, &b.DriveID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Book{}, nil
		}
		return model.Book{}, err
	}
	return b, nil
}

// GetBookByPerfil mirrors GetBookByPerfilAsync.
func (r *BookRepository) GetBookByPerfil(ctx context.Context, perfis []string) ([]model.Book, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, perfil, driveid FROM gnosis.books WHERE perfil = ANY($1)`, perfis)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := []model.Book{}
	for rows.Next() {
		var b model.Book
		if err := rows.Scan(&b.ID, &b.Name, &b.Perfil, &b.DriveID); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// InsertBook mirrors InsertBookAsync.
func (r *BookRepository) InsertBook(ctx context.Context, name, perfil, driveID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO gnosis.books (name, perfil, driveid) VALUES ($1, $2, $3)`,
		name, perfil, driveID)
	return err
}
