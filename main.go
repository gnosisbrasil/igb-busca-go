// igb-busca-go is a Go + PostgreSQL port of the SuperGnosis.Api backend
// (Gnosis/igb-supergnosis), keeping the same routes, logic and JSON shapes.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"igb-busca-go/handler"
	"igb-busca-go/middleware"
	"igb-busca-go/repository"
	"igb-busca-go/service"
)

func main() {
	connStr := os.Getenv("CONNECTION_STRING")
	if connStr == "" {
		log.Fatal("CONNECTION_STRING env is required (PostgreSQL DSN)")
	}

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	books := repository.NewBookRepository(pool)
	pages := repository.NewPageRepository(pool)
	users := repository.NewUserRepository(pool)
	analyticsStore := repository.NewSearchWordAnalyticsRepository(pool)

	tokens := service.NewTokenService(os.Getenv("SECRET_JWT"))
	authHandler := handler.NewAuthHandler(users, tokens)
	bookHandler := handler.NewBookHandler(books, pages, analyticsStore)
	analyticsHandler := handler.NewAnalyticsHandler(service.NewAnalyticsService(analyticsStore))
	adminHandler := handler.NewAdminHandler(books, pages, analyticsStore, service.NewJobManager())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/auth/me", authHandler.Me)
	mux.HandleFunc("POST /api/book", bookHandler.PostBooks)
	mux.HandleFunc("POST /api/book/upload", bookHandler.Upload)
	mux.HandleFunc("GET /api/book/all", bookHandler.GetBooks)
	mux.HandleFunc("GET /api/book/search-word", bookHandler.GetPageByText)
	mux.HandleFunc("GET /api/book/{bookId}/page/{pageNumber}", bookHandler.GetPageByNumber)
	mux.HandleFunc("GET /api/search-word-analytics/all", analyticsHandler.GetAll)
	mux.HandleFunc("POST /api/book/sync", adminHandler.Sync)
	mux.HandleFunc("POST /api/book/reprocess", adminHandler.Reprocess)
	mux.HandleFunc("GET /api/book/jobs", adminHandler.Jobs)
	mux.HandleFunc("GET /api/book/jobs/{id}", adminHandler.JobStatus)
	mux.HandleFunc("GET /api/book/reprocess/dry-run", adminHandler.DryRun)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Pipeline mirrors Program.cs: CORS -> JWT middleware -> controllers.
	app := middleware.CORS(middleware.JWT(users, tokens)(mux))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, app))
}
