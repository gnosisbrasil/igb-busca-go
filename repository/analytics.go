package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"igb-busca-go/model"
)

// AnalyticsStore mirrors ISearchWordAnalyticsRepository.
type AnalyticsStore interface {
	GetSearchWordAnalytics(ctx context.Context) ([]model.SearchWordAnalytics, error)
	InsertSearchWordAnalytics(ctx context.Context, perfil, searchWord string) error
}

// SearchWordAnalyticsRepository mirrors SearchWordAnalyticsRepository.cs.
type SearchWordAnalyticsRepository struct {
	pool *pgxpool.Pool
}

// NewSearchWordAnalyticsRepository creates a SearchWordAnalyticsRepository.
func NewSearchWordAnalyticsRepository(pool *pgxpool.Pool) *SearchWordAnalyticsRepository {
	return &SearchWordAnalyticsRepository{pool: pool}
}

// GetSearchWordAnalytics mirrors GetSearchWordAnalyticsAsync.
// Amount has no column and stays 0, as in the original.
func (r *SearchWordAnalyticsRepository) GetSearchWordAnalytics(ctx context.Context) ([]model.SearchWordAnalytics, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id as id,
			perfil as perfil,
			search_word as searchword
		FROM gnosis.search_word_analytics`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []model.SearchWordAnalytics{}
	for rows.Next() {
		var a model.SearchWordAnalytics
		if err := rows.Scan(&a.ID, &a.Perfil, &a.SearchWord); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// InsertSearchWordAnalytics mirrors InsertSearchWordAnalyticsAsync.
func (r *SearchWordAnalyticsRepository) InsertSearchWordAnalytics(ctx context.Context, perfil, searchWord string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO gnosis.search_word_analytics (perfil, search_word) VALUES ($1, $2)`,
		perfil, searchWord)
	return err
}
