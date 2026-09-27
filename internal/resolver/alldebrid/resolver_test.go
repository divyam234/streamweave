package alldebrid

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"streamweave/internal/domain"
)

func TestResolveReadyMagnet(t *testing.T) {
	hash := strings.Repeat("a", 40)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}

		switch req.URL.Path {
		case "/v4/magnet/upload":
			if got := req.Form.Get("magnets[]"); got != hash {
				t.Fatalf("magnet hash = %q", got)
			}
			writeTestJSON(t, w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"magnets": []map[string]any{{
						"id": 42, "hash": hash, "name": "release",
						"size": 1000, "ready": true,
					}},
				},
			})
		case "/v4/magnet/files":
			if got := req.Form.Get("id[]"); got != "42" {
				t.Fatalf("magnet id = %q", got)
			}
			writeTestJSON(t, w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"magnets": []map[string]any{{
						"id": "42",
						"files": []map[string]any{
							{"n": "readme.txt", "s": 10, "l": "https://alldebrid.com/f/readme"},
							{"n": "movie.mkv", "s": 900, "l": "https://alldebrid.com/f/movie"},
						},
					}},
				},
			})
		case "/v4/link/unlock":
			if got := req.Form.Get("link"); got != "https://alldebrid.com/f/movie" {
				t.Fatalf("unlock link = %q", got)
			}
			writeTestJSON(t, w, map[string]any{
				"status": "success",
				"data": map[string]any{
					"link":     "https://cdn.example/movie.mkv",
					"filename": "movie.mkv",
					"filesize": 900,
				},
			})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("token", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClientWithBaseURL: %v", err)
	}
	resolver := NewResolver("resolver-1", client)

	result, err := resolver.Resolve(context.Background(), domain.Candidate{
		ID:   "candidate-1",
		Kind: domain.CandidateTorrent,
		Torrent: &domain.TorrentInfo{
			InfoHash: hash,
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.HTTP == nil || result.HTTP.URL != "https://cdn.example/movie.mkv" {
		t.Fatalf("unexpected HTTP result: %#v", result.HTTP)
	}
	if result.Cached == nil || !*result.Cached {
		t.Fatal("expected cached result")
	}
}

func TestResolveUncachedMagnetKeepsTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"status": "success",
			"data": map[string]any{
				"magnets": []map[string]any{{
					"id": 42, "hash": "abc", "ready": false,
				}},
			},
		})
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("token", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClientWithBaseURL: %v", err)
	}
	resolver := NewResolver("resolver-1", client)

	result, err := resolver.Resolve(context.Background(), domain.Candidate{
		Kind:    domain.CandidateTorrent,
		Torrent: &domain.TorrentInfo{InfoHash: "abc"},
	})
	if err == nil {
		t.Fatal("expected uncached error")
	}
	if result.HTTP != nil {
		t.Fatalf("unexpected HTTP result: %#v", result.HTTP)
	}
	if result.Cached == nil || *result.Cached {
		t.Fatal("expected uncached result")
	}
}

func TestSelectFileUsesTorrentIndex(t *testing.T) {
	index := 1
	file, ok := selectFile([]File{
		{Name: "first.txt", Link: "one"},
		{Name: "second.mkv", Link: "two"},
	}, &index)
	if !ok || file.Link != "two" {
		t.Fatalf("selected %#v, ok=%v", file, ok)
	}
}

func TestResolveRejectsDelayedLinkWithoutPolling(t *testing.T) {
	hash := strings.Repeat("b", 40)
	delayedCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch req.URL.Path {
		case "/v4/magnet/upload":
			writeTestJSON(t, w, map[string]any{"status": "success", "data": map[string]any{"magnets": []map[string]any{{"id": 7, "hash": hash, "ready": true}}}})
		case "/v4/magnet/files":
			writeTestJSON(t, w, map[string]any{"status": "success", "data": map[string]any{"magnets": []map[string]any{{"files": []map[string]any{{"n": "movie.mkv", "s": 100, "l": "https://alldebrid.com/f/movie"}}}}}})
		case "/v4/link/unlock":
			writeTestJSON(t, w, map[string]any{"status": "success", "data": map[string]any{"filename": "movie.mkv", "filesize": 100, "delayed": 99}})
		case "/v4/link/delayed":
			delayedCalls++
			if got := req.Form.Get("id"); got != "99" {
				t.Fatalf("delayed id = %q", got)
			}
			status := 1
			link := ""
			if delayedCalls == 2 {
				status = 2
				link = "https://cdn.example/movie.mkv"
			}
			writeTestJSON(t, w, map[string]any{"status": "success", "data": map[string]any{"status": status, "time_left": 0, "link": link}})
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("token", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClientWithBaseURL: %v", err)
	}
	resolver := NewResolver("resolver-1", client)

	_, err = resolver.Resolve(context.Background(), domain.Candidate{Kind: domain.CandidateTorrent, Torrent: &domain.TorrentInfo{InfoHash: hash}})
	if !errors.Is(err, ErrDelayed) || delayedCalls != 0 {
		t.Fatalf("error=%v, delayed calls=%d", err, delayedCalls)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
