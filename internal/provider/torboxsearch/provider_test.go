package torboxsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media-engine/internal/domain"
	"media-engine/internal/engine"
)

func TestSearchReturnsTorrentAndUsenet(t *testing.T) {
	hash := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization missing")
		}
		switch {
		case strings.HasPrefix(req.URL.Path, "/torrents/imdb_id:"):
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"torrents": []map[string]any{{
				"hash": hash, "raw_title": "Movie.1080p", "size": 1234, "tracker": "Test", "type": "torrent", "last_known_seeders": 12, "cached": true,
			}}}})
		case strings.HasPrefix(req.URL.Path, "/usenet/imdb_id:"):
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"nzbs": []map[string]any{{
				"hash": "x", "raw_title": "Movie.2160p", "size": 2345, "tracker": "TestNZB", "type": "usenet", "last_known_seeders": -1, "nzb": "https://indexer.example/a.nzb", "cached": true,
			}}}})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	p, err := New("tb-search", server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := p.Search(context.Background(), engine.SearchRequest{Media: domain.MediaRef{Type: "movie", ID: "tt1234567"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Torrent == nil || got[1].Usenet == nil {
		t.Fatalf("unexpected candidates: %#v", got)
	}
	if got[0].Cached == nil || !*got[0].Cached {
		t.Fatal("torrent cached flag missing")
	}
	if got[1].Usenet.NZBURL != "https://indexer.example/a.nzb" {
		t.Fatalf("nzb=%q", got[1].Usenet.NZBURL)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
