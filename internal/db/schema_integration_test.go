package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run after migrating the alpha and beta schemas in a disposable PostgreSQL database.
func TestSchemaIsolation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL schema integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO alpha.providers (name, kind) VALUES ('schema-test', 'remote-addon') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM alpha.providers WHERE id=$1`, id) })
	for _, test := range []struct {
		schema string
		want   int
	}{{"alpha", 1}, {"beta", 0}} {
		queries, err := NewQueries(pool, test.schema)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := queries.ListProviders(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != test.want {
			t.Fatalf("%s: got %d rows, want %d", test.schema, len(rows), test.want)
		}
	}
}
