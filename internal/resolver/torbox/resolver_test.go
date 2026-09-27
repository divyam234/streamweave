package torbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"media-engine/internal/domain"
)

func TestResolveUsenet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/api/usenet/createusenetdownload":
			_ = req.ParseForm()
			if req.Form.Get("link") != "https://indexer.example/file.nzb" {
				t.Fatalf("link = %q", req.Form.Get("link"))
			}
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"usenetdownload_id": 7, "hash": "abc"}})
		case "/v1/api/usenet/mylist":
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "download_present": true, "download_finished": true, "download_state": "completed",
				"files": []map[string]any{{"id": 3, "name": "/Movie/movie.mkv", "short_name": "movie.mkv", "size": 999}},
			}})
		case "/v1/api/usenet/requestdl":
			if req.URL.Query().Get("usenet_id") != "7" || req.URL.Query().Get("file_id") != "3" {
				t.Fatalf("query = %s", req.URL.RawQuery)
			}
			writeJSON(t, w, map[string]any{"success": true, "data": "https://cdn.example/movie.mkv"})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	resolver, err := NewResolver("tb", "token", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	result, err := resolver.Resolve(context.Background(), domain.Candidate{
		Kind:   domain.CandidateUsenet,
		Usenet: &domain.UsenetInfo{NZBURL: "https://indexer.example/file.nzb", Hash: "abc"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.HTTP == nil || result.HTTP.URL != "https://cdn.example/movie.mkv" {
		t.Fatalf("HTTP = %#v", result.HTTP)
	}
	if result.Kind != domain.CandidateDirect {
		t.Fatalf("kind = %q", result.Kind)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
