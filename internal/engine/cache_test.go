package engine

import (
	"context"
	"sync"
	"testing"

	"streamweave/internal/domain"
)

type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) ID() string { return "counting" }

func (p *countingProvider) Search(context.Context, SearchRequest) ([]domain.Candidate, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return []domain.Candidate{{ID: "one", SourceID: p.ID(), Kind: domain.CandidateTorrent, Torrent: &domain.TorrentInfo{InfoHash: "abc"}}}, nil
}

func TestSearchCachesDiscovery(t *testing.T) {
	provider := &countingProvider{}
	e := New(provider)
	req := SearchRequest{InstallationID: "install", Media: domain.MediaRef{Type: "movie", ID: "tt1"}}
	for range 2 {
		if _, err := e.Search(context.Background(), req); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestSearchCacheKeyIncludesInstallation(t *testing.T) {
	provider := &countingProvider{}
	e := New(provider)
	for _, installation := range []string{"one", "two"} {
		if _, err := e.Search(context.Background(), SearchRequest{InstallationID: installation, Media: domain.MediaRef{Type: "movie", ID: "tt1"}}); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}

type flakyProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *flakyProvider) ID() string { return "flaky" }

func (p *flakyProvider) Search(context.Context, SearchRequest) ([]domain.Candidate, error) {
	p.mu.Lock()
	p.calls++
	calls := p.calls
	p.mu.Unlock()
	if calls == 1 {
		return []domain.Candidate{}, nil
	}
	return []domain.Candidate{{ID: "one", SourceID: p.ID(), Kind: domain.CandidateTorrent, Torrent: &domain.TorrentInfo{InfoHash: "abc"}}}, nil
}

func TestSearchDoesNotCacheEmptyDiscovery(t *testing.T) {
	provider := &flakyProvider{}
	e := New(provider)
	req := SearchRequest{InstallationID: "install", Media: domain.MediaRef{Type: "movie", ID: "tt1"}}
	first, err := e.SearchUnresolved(context.Background(), req)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("first len = %d, want 0", len(first))
	}
	second, err := e.SearchUnresolved(context.Background(), req)
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second len = %d, want 1 (empty result must not be cached)", len(second))
	}
}
