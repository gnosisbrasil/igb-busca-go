package service

import (
	"context"

	"igb-busca-go/model"
	"igb-busca-go/repository"
)

// AnalyticsService mirrors SearchWordAnalyticsService.cs.
type AnalyticsService struct {
	store repository.AnalyticsStore
}

// NewAnalyticsService creates an AnalyticsService.
func NewAnalyticsService(store repository.AnalyticsStore) *AnalyticsService {
	return &AnalyticsService{store: store}
}

// GetAll mirrors GetBooksAsync (the oddly-named analytics listing).
func (s *AnalyticsService) GetAll(ctx context.Context) ([]model.SearchWordAnalytics, error) {
	return s.store.GetSearchWordAnalytics(ctx)
}
