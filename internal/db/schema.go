package db

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	gen "streamweave/internal/db/gen"
)

const DefaultSchema = "streamweave"
const Marker = "/* TEMPLATE: schema */"

var schemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Schema(raw string) (string, error) {
	if raw == "" {
		raw = DefaultSchema
	}
	if !schemaPattern.MatchString(raw) {
		return "", errors.New("invalid database schema")
	}
	return raw, nil
}

func Quote(schema string) string { return pgx.Identifier{schema}.Sanitize() }

type qualified struct {
	gen.DBTX
	prefix string
}

func NewQueries(connection gen.DBTX, schema string) (*gen.Queries, error) {
	name, err := Schema(schema)
	if err != nil {
		return nil, err
	}
	return gen.New(qualified{DBTX: connection, prefix: Quote(name) + "."}), nil
}

func (q qualified) render(query string) string { return strings.ReplaceAll(query, Marker, q.prefix) }
func (q qualified) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return q.DBTX.Exec(ctx, q.render(query), args...)
}
func (q qualified) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return q.DBTX.Query(ctx, q.render(query), args...)
}
func (q qualified) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return q.DBTX.QueryRow(ctx, q.render(query), args...)
}
