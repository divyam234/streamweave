package resolver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"streamweave/internal/db"
	"streamweave/internal/secretbox"
)

// Requires a disposable PostgreSQL database with the alpha schema migrated.
func TestProxyStreamRangeAndRevocation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for stream integration test")
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "video.example" || r.Header.Get("Range") != "bytes=2-" {
			t.Errorf("unexpected proxy request: %s %s", r.URL, r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 2-3/4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("cd"))
	}))
	defer proxy.Close()
	box, err := secretbox.NewFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := box.Seal([]byte(proxy.URL))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var id string
	if err := pool.QueryRow(t.Context(), `INSERT INTO alpha.resolver_accounts (name,kind,secret_ciphertext,secret_nonce,proxy_ciphertext,proxy_nonce) VALUES ('test','alldebrid',$1,$2,$3,$4) RETURNING id::text`, []byte("secret"), nonce, ciphertext, nonce).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM alpha.resolver_accounts WHERE id=$1`, id) })
	queries, err := db.NewQueries(pool, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	source := NewSource(queries, box, nil, BaseURLs{}, nil)
	data, _ := json.Marshal(streamToken{Account: id, URL: "http://video.example/file", Expires: time.Now().Add(time.Hour).Unix()})
	token, err := box.SealURL(data)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/stream/"+token, nil)
	request.Header.Set("Range", "bytes=2-")
	response := httptest.NewRecorder()
	source.ServeStream(response, request, token)
	if response.Code != http.StatusPartialContent || response.Body.String() != "cd" || response.Header().Get("Content-Range") != "bytes 2-3/4" {
		t.Fatalf("unexpected stream: %d %s", response.Code, response.Body.String())
	}
	if _, err := pool.Exec(t.Context(), `UPDATE alpha.resolver_accounts SET enabled=false WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	source.ServeStream(response, request, token)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled account returned %d", response.Code)
	}
}
