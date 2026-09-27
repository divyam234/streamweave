package remoteaddon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

func TestValidateEndpointStripsManifestSuffix(t *testing.T) {
	got, err := ValidateEndpoint("https://example.com/config-token/manifest.json", false)
	if err != nil {
		t.Fatalf("ValidateEndpoint: %v", err)
	}
	if got != "https://example.com/config-token" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestSearchIdentifiesClientToAddon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "StreamWeave/0.2.0" {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"streams":[{"infoHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))
	}))
	defer server.Close()

	provider, err := New("test", "torrentio", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{Media: domain.MediaRef{Type: "movie", ID: "tt1234567"}})
	if err != nil || len(results) != 1 {
		t.Fatalf("Search returned %d results: %v", len(results), err)
	}
}

func TestToCandidatePreservesDisplayNames(t *testing.T) {
	provider := &Provider{id: "remote-id", name: "torrentio"}
	media := domain.MediaRef{Type: "movie", ID: "tt1234567"}

	candidate, ok := provider.toCandidate(media, stream{
		Name:     "Torrentio\n4k HDR",
		Title:    "Movie.2026.2160p.WEB-DL.H265.mkv\nDetails",
		InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if !ok {
		t.Fatal("valid torrent was rejected")
	}
	if candidate.SourceName != "torrentio" {
		t.Fatalf("SourceName = %q", candidate.SourceName)
	}
	if candidate.UpstreamName != "Torrentio\n4k HDR" {
		t.Fatalf("UpstreamName = %q", candidate.UpstreamName)
	}
	if candidate.Title != "Movie.2026.2160p.WEB-DL.H265.mkv\nDetails" {
		t.Fatalf("Title = %q", candidate.Title)
	}
}

func TestSearchKeepsSeriesIDColonsUnescaped(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"streams":[]}`))
	}))
	defer server.Close()

	provider, err := New("test", "torrentio", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "series", ID: "tt1234567:2:3"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/stream/series/tt1234567:2:3.json" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestSearchRetriesTransientAddonFailureOnce(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"streams":[{"infoHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))
	}))
	defer server.Close()

	provider, err := New("test", "torrentio", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "movie", ID: "tt1234567"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || calls != 2 {
		t.Fatalf("results = %d, calls = %d", len(results), calls)
	}
}

func TestSearchDoesNotRetryNotFound(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider, err := New("test", "torrentio", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "movie", ID: "tt1234567"},
	}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestSearchRetriesEmptyFirstResponse(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"streams":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"streams":[{"infoHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))
	}))
	defer server.Close()

	provider, err := New("test", "torrentio", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "series", ID: "tt1234567:1:2"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || calls != 2 {
		t.Fatalf("results = %d, calls = %d", len(results), calls)
	}
}

func TestValidateEndpointPreservesConfiguredBase(t *testing.T) {
	got, err := ValidateEndpoint("https://example.com/config-token", false)
	if err != nil {
		t.Fatalf("ValidateEndpoint: %v", err)
	}
	if got != "https://example.com/config-token" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestToCandidateRejectsMalformedTorrent(t *testing.T) {
	provider := &Provider{id: "remote"}
	media := domain.MediaRef{Type: "movie", ID: "tt1234567"}

	if _, ok := provider.toCandidate(media, stream{InfoHash: "not-a-hash"}); ok {
		t.Fatal("malformed info hash was accepted")
	}

	index := -1
	if _, ok := provider.toCandidate(media, stream{
		InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FileIdx:  &index,
	}); ok {
		t.Fatal("negative file index was accepted")
	}
}
