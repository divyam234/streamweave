package prowlarr

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

func TestSearchSplitsTorrentAndUsenetIndexers(t *testing.T) {
	hash := strings.Repeat("b", 40)
	searchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Api-Key") != "secret" {
			t.Fatalf("api key header missing")
		}
		switch req.URL.Path {
		case "/api/v1/indexer":
			writeJSON(t, w, []map[string]any{
				{"id": 1, "name": "Torrent", "enable": true, "protocol": "torrent"},
				{"id": 2, "name": "Usenet", "enable": true, "protocol": "usenet"},
			})
		case "/api/v1/search":
			searchCalls++
			ids := req.URL.Query()["indexerIds"]
			if len(ids) != 1 {
				t.Fatalf("indexerIds = %#v", ids)
			}
			if ids[0] == "1" {
				writeJSON(t, w, []map[string]any{{
					"title": "Movie.1080p", "ageHours": 1, "size": 1000,
					"indexer": "Torrent", "infoHash": hash, "seeders": 20,
				}})
			} else {
				writeJSON(t, w, []map[string]any{{
					"title": "Movie.2160p", "ageHours": 1, "size": 2000,
					"indexer": "Usenet", "downloadUrl": "https://nzb.example/file.nzb",
				}})
			}
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	provider, err := New("p1", server.URL, "secret", server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "movie", ID: "tt1234567"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if searchCalls != 2 {
		t.Fatalf("search calls = %d, want 2", searchCalls)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Torrent == nil || results[1].Usenet == nil {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
