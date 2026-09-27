package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"streamweave/internal/domain"
)

type limitProvider struct {
	candidates []domain.Candidate
}

func (p limitProvider) ID() string { return "limit-provider" }

func (p limitProvider) Search(context.Context, SearchRequest) ([]domain.Candidate, error) {
	return cloneCandidates(p.candidates), nil
}

type limitResolver struct {
	calls atomic.Int32
}

func (r *limitResolver) ID() string { return "limit-resolver" }

func (r *limitResolver) Resolve(_ context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	r.calls.Add(1)
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: "https://cdn.example/" + candidate.ID}
	return candidate, nil
}

type limitResolverSource struct {
	mode     ResolutionMode
	resolver Resolver
}

func (s limitResolverSource) ResolutionFor(context.Context, string) (Resolution, error) {
	return Resolution{Mode: s.mode, Resolver: s.resolver}, nil
}

func TestSearchCapsReturnedCandidates(t *testing.T) {
	candidates := make([]domain.Candidate, 150)
	for i := range candidates {
		candidates[i] = domain.Candidate{
			ID:       fmt.Sprintf("direct-%03d", i),
			SourceID: "provider",
			Kind:     domain.CandidateDirect,
			HTTP:     &domain.HTTPStream{URL: fmt.Sprintf("https://cdn.example/%03d", i)},
		}
	}

	engine := NewWithSources(StaticSource{limitProvider{candidates: candidates}}, NoResolvers{}, 8)
	got, err := engine.Search(context.Background(), SearchRequest{Media: domain.MediaRef{Type: "movie", ID: "tt1"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != defaultMaxCandidates {
		t.Fatalf("len = %d, want %d", len(got), defaultMaxCandidates)
	}
}

func TestServerResolutionCapsResolverAmplification(t *testing.T) {
	candidates := make([]domain.Candidate, 30)
	for i := range candidates {
		candidates[i] = domain.Candidate{
			ID:       fmt.Sprintf("torrent-%03d", i),
			SourceID: "provider",
			Kind:     domain.CandidateTorrent,
			Torrent:  &domain.TorrentInfo{InfoHash: fmt.Sprintf("%040x", i+1)},
		}
	}
	resolver := &limitResolver{}
	engine := NewWithSources(
		StaticSource{limitProvider{candidates: candidates}},
		limitResolverSource{mode: ResolutionServer, resolver: resolver},
		8,
	)
	got, err := engine.Search(context.Background(), SearchRequest{
		InstallationID: "test",
		Media:          domain.MediaRef{Type: "movie", ID: "tt1"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotCalls := int(resolver.calls.Load()); gotCalls != defaultMaxResolveTargets {
		t.Fatalf("resolver calls = %d, want %d", gotCalls, defaultMaxResolveTargets)
	}
	if len(got) != defaultMaxResolveTargets {
		t.Fatalf("len = %d, want %d", len(got), defaultMaxResolveTargets)
	}
}

func TestHybridKeepsCandidatesBeyondResolutionCap(t *testing.T) {
	candidates := make([]domain.Candidate, 30)
	for i := range candidates {
		candidates[i] = domain.Candidate{
			ID:       fmt.Sprintf("torrent-%03d", i),
			SourceID: "provider",
			Kind:     domain.CandidateTorrent,
			Torrent:  &domain.TorrentInfo{InfoHash: fmt.Sprintf("%040x", i+1)},
		}
	}
	resolver := &limitResolver{}
	engine := NewWithSources(
		StaticSource{limitProvider{candidates: candidates}},
		limitResolverSource{mode: ResolutionHybrid, resolver: resolver},
		8,
	)
	got, err := engine.Search(context.Background(), SearchRequest{
		InstallationID: "test",
		Media:          domain.MediaRef{Type: "movie", ID: "tt1"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotCalls := int(resolver.calls.Load()); gotCalls != defaultMaxResolveTargets {
		t.Fatalf("resolver calls = %d, want %d", gotCalls, defaultMaxResolveTargets)
	}
	if len(got) != 30 {
		t.Fatalf("len = %d, want 30", len(got))
	}
}
