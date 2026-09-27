package realdebrid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media-engine/internal/domain"
)

func TestResolve(t *testing.T) {
	hash := strings.Repeat("a", 40)
	selected := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		switch req.URL.Path {
		case "/torrents/addMagnet":
			_ = req.ParseForm()
			if !strings.Contains(req.Form.Get("magnet"), hash) {
				t.Fatalf("magnet = %q", req.Form.Get("magnet"))
			}
			writeJSON(t, w, map[string]any{"id": "rd-1", "uri": "magnet"})
		case "/torrents/info/rd-1":
			links := []string{}
			selectedFlag := 0
			if selected {
				selectedFlag = 1
				links = []string{"https://host.example/original"}
			}
			writeJSON(t, w, map[string]any{
				"id": "rd-1", "hash": hash, "status": "downloaded",
				"files": []map[string]any{
					{"id": 1, "path": "/readme.txt", "bytes": 10, "selected": 0},
					{"id": 2, "path": "/movie.mkv", "bytes": 900, "selected": selectedFlag},
				},
				"links": links,
			})
		case "/torrents/selectFiles/rd-1":
			_ = req.ParseForm()
			if got := req.Form.Get("files"); got != "2" {
				t.Fatalf("files = %q", got)
			}
			selected = true
			writeJSON(t, w, map[string]any{})
		case "/unrestrict/link":
			writeJSON(t, w, map[string]any{
				"filename": "movie.mkv",
				"filesize": 900,
				"download": "https://cdn.example/movie.mkv",
			})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	resolver, err := NewResolver("rd", "token", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	result, err := resolver.Resolve(context.Background(), domain.Candidate{
		Kind:    domain.CandidateTorrent,
		Torrent: &domain.TorrentInfo{InfoHash: hash},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.HTTP == nil || result.HTTP.URL != "https://cdn.example/movie.mkv" {
		t.Fatalf("HTTP = %#v", result.HTTP)
	}
	if result.Cached == nil || !*result.Cached {
		t.Fatalf("Cached = %#v", result.Cached)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
