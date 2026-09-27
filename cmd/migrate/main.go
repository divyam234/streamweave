package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
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

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("configure migrations: %v", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	version, err := goose.GetDBVersion(db)
	if err != nil {
		log.Fatalf("read migration version: %v", err)
	}
	fmt.Printf("database migrated to version %d\n", version)
}
