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

	provider, err := New("test", server.URL, server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{Media: domain.MediaRef{Type: "movie", ID: "tt1234567"}})
	if err != nil || len(results) != 1 {
		t.Fatalf("Search returned %d results: %v", len(results), err)
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
