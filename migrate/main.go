// One-shot MySQL -> Postgres migration for the gnosis database.
// Usage: MYSQL_DSN='u:p@tcp(127.0.0.1:13306)/gnosis?charset=utf8mb4' PG_DSN='postgres://...' go run .
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	ctx := context.Background()
	my, err := sql.Open("mysql", os.Getenv("MYSQL_DSN"))
	must(err)
	defer my.Close()
	pg, err := pgx.Connect(ctx, os.Getenv("PG_DSN"))
	must(err)
	defer pg.Close(ctx)

	_, err = pg.Exec(ctx, "TRUNCATE gnosis.pages, gnosis.search_word_analytics, gnosis.books, gnosis.users RESTART IDENTITY")
	must(err)

	copyTable := func(name, cols string) int64 {
		colList := strings.Split(cols, ",")
		rows, err := my.Query(fmt.Sprintf("SELECT %s FROM gnosis.%s ORDER BY id", cols, name))
		must(err)
		defer rows.Close()
		tx, err := pg.Begin(ctx)
		must(err)
		defer tx.Rollback(ctx)
		var total int64
		batch := [][]any{}
		flush := func() {
			if len(batch) == 0 {
				return
			}
			_, err := tx.CopyFrom(ctx, pgx.Identifier{"gnosis", name}, colList, pgx.CopyFromRows(batch))
			must(err)
			total += int64(len(batch))
			batch = batch[:0]
		}
		for rows.Next() {
			vals := make([]any, len(colList))
			ptrs := make([]any, len(vals))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			must(rows.Scan(ptrs...))
			for i, v := range vals {
				if b, ok := v.([]byte); ok {
					// Postgres TEXT rejects NUL bytes that MySQL LONGTEXT accepts.
					vals[i] = strings.ReplaceAll(string(b), "\x00", "")
				}
			}
			batch = append(batch, vals)
			if len(batch) >= 1000 {
				flush()
			}
		}
		must(rows.Err())
		flush()
		must(tx.Commit(ctx))
		return total
	}

	fmt.Printf("users: %d\n", copyTable("users", "id,name,perfil,senha"))
	fmt.Printf("books: %d\n", copyTable("books", "id,name,perfil,driveid"))
	fmt.Printf("pages: %d\n", copyTable("pages", "id,book_id,page_number,page_text"))
	fmt.Printf("search_word_analytics: %d\n", copyTable("search_word_analytics", "id,perfil,search_word"))

	for _, t := range []string{"users", "books", "pages", "search_word_analytics"} {
		var max sql.NullInt64
		must(pg.QueryRow(ctx, fmt.Sprintf("SELECT MAX(id) FROM gnosis.%s", t)).Scan(&max))
		if max.Valid {
			_, err := pg.Exec(ctx, fmt.Sprintf("SELECT setval(pg_get_serial_sequence('gnosis.%s','id'), %d)", t, max.Int64))
			must(err)
		}
		fmt.Printf("seq %s -> %d\n", t, max.Int64)
	}
}
