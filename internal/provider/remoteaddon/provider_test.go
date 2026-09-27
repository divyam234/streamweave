package remoteaddon

import (
	"testing"

	"streamweave/internal/domain"
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
