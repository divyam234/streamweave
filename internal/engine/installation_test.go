package engine

import (
	"context"
	"errors"
	"testing"

	"media-engine/internal/domain"
)

type rejectingResolverSource struct{}

func (rejectingResolverSource) ValidateInstallation(context.Context, string) error {
	return ErrInstallationUnavailable
}

func (rejectingResolverSource) ResolutionFor(context.Context, string) (Resolution, error) {
	return Resolution{Mode: ResolutionClient}, nil
}

type countingSource struct {
	calls int
}

func (s *countingSource) Providers(context.Context) ([]Provider, error) {
	s.calls++
	return nil, nil
}

func TestSearchRejectsInstallationBeforeDiscovery(t *testing.T) {
	source := &countingSource{}
	engine := NewWithSources(source, rejectingResolverSource{}, 8)

	_, err := engine.Search(context.Background(), SearchRequest{
		InstallationID: "invalid",
		Media:          domain.MediaRef{Type: "movie", ID: "tt1234567"},
	})
	if !errors.Is(err, ErrInstallationUnavailable) {
		t.Fatalf("Search error = %v", err)
	}
	if source.calls != 0 {
		t.Fatalf("provider source called %d times, want 0", source.calls)
	}
}
