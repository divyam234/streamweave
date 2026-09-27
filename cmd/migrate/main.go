package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"streamweave/internal/db"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/app/db/migrations"
	}
	schema, err := db.Schema(os.Getenv("DATABASE_SCHEMA"))
	if err != nil {
		log.Fatal(err)
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatal(err)
	}
	rendered := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(migrationsDir + "/" + entry.Name())
		if err != nil {
			log.Fatal(err)
		}
		data = []byte(strings.ReplaceAll(string(data), db.Marker+"public", db.Quote(schema)))
		rendered[entry.Name()] = &fstest.MapFile{Data: data}
	}
	var migrations fs.FS = rendered
	goose.SetBaseFS(migrations)
	goose.SetTableName(db.Quote(schema) + ".migrations")

	connection, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer connection.Close()

	if err := connection.Ping(); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if _, err := connection.ExecContext(context.Background(), "CREATE SCHEMA IF NOT EXISTS "+db.Quote(schema)); err != nil {
		log.Fatal(err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("configure migrations: %v", err)
	}
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "up" && command != "status" {
		log.Fatal("supported commands: up, status")
	}
	if err := goose.RunContext(context.Background(), command, connection, "."); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	version, err := goose.GetDBVersion(connection)
	if err != nil {
		log.Fatalf("read migration version: %v", err)
	}
	fmt.Printf("database migrated to version %d\n", version)
}
