package stremio

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

type profileResolverSource struct {
	profile engine.InstallationProfile
}

func (s profileResolverSource) ValidateInstallation(context.Context, string) error { return nil }
func (s profileResolverSource) InstallationProfile(context.Context, string) (engine.InstallationProfile, error) {
	return s.profile, nil
}
func (s profileResolverSource) ResolutionFor(context.Context, string) (engine.Resolution, error) {
	return engine.Resolution{Mode: s.profile.ResolutionMode}, nil
}

type resolvingSource struct {
	profile  engine.InstallationProfile
	resolver engine.Resolver
}

func (s resolvingSource) ValidateInstallation(context.Context, string) error { return nil }
func (s resolvingSource) InstallationProfile(context.Context, string) (engine.InstallationProfile, error) {
	return s.profile, nil
}
func (s resolvingSource) ResolutionFor(context.Context, string) (engine.Resolution, error) {
	return engine.Resolution{Mode: s.profile.ResolutionMode, Resolver: s.resolver}, nil
}

type captureProvider struct {
	last engine.SearchRequest
}

func (p *captureProvider) ID() string { return "capture" }
func (p *captureProvider) Search(_ context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	p.last = req
	return torrentProvider{}.Search(context.Background(), req)
}

type directResolver struct{}

func (directResolver) ID() string { return "direct-resolver" }
func (directResolver) Resolve(_ context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	cached := true
	candidate.Kind = domain.CandidateDirect
	candidate.Cached = &cached
	candidate.HTTP = &domain.HTTPStream{URL: "https://cdn.example/video.mkv"}
	return candidate, nil
}

type failingResolver struct{}

func (failingResolver) ID() string { return "failing-resolver" }
func (failingResolver) Resolve(context.Context, domain.Candidate) (domain.Candidate, error) {
	return domain.Candidate{}, errors.New("resolver failed")
}

type torrentProvider struct{}

func (torrentProvider) ID() string { return "torrent" }
func (torrentProvider) Search(context.Context, engine.SearchRequest) ([]domain.Candidate, error) {
	index := 2
	return []domain.Candidate{{
		ID:        "torrent",
		SourceID:  "torrent",
		Kind:      domain.CandidateTorrent,
		Title:     "Movie.1080p",
		Filename:  "Movie.1080p.mkv",
		SizeBytes: 987654321,
		Torrent:   &domain.TorrentInfo{InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", FileIndex: &index},
	}}, nil
}

func TestManifest(t *testing.T) {
	handler := NewHandler(engine.New())
	req := httptest.NewRequest(http.MethodGet, "/default/manifest.json", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var manifest Manifest
	if err := json.NewDecoder(rec.Body).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.ID == "streamweave" || len(manifest.ID) <= len("streamweave.") {
		t.Fatalf("manifest id = %q", manifest.ID)
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0] != "stream" {
		t.Fatalf("unexpected resources: %#v", manifest.Resources)
	}
}

func TestNuvioClientManifestAdvertisesP2P(t *testing.T) {
	e := engine.NewWithSources(engine.StaticSource{}, profileResolverSource{profile: engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionClient,
	}}, 1)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-token/manifest.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var manifest Manifest
	if err := json.NewDecoder(rec.Body).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.IDPrefixes) != 2 || manifest.IDPrefixes[0] != "tt" || manifest.IDPrefixes[1] != "tmdb" {
		t.Fatalf("idPrefixes = %#v", manifest.IDPrefixes)
	}
	if manifest.BehaviorHints == nil || !manifest.BehaviorHints.P2P || manifest.BehaviorHints.P2PNotSupported {
		t.Fatalf("behaviorHints = %#v", manifest.BehaviorHints)
	}
	if manifest.ID == "streamweave" {
		t.Fatalf("manifest ID must be installation-specific")
	}
}

func TestNuvioServerManifestDisablesP2P(t *testing.T) {
	e := engine.NewWithSources(engine.StaticSource{}, profileResolverSource{profile: engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionServer,
	}}, 1)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-server/manifest.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var manifest Manifest
	if err := json.NewDecoder(rec.Body).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.BehaviorHints == nil || manifest.BehaviorHints.P2P || !manifest.BehaviorHints.P2PNotSupported {
		t.Fatalf("behaviorHints = %#v", manifest.BehaviorHints)
	}
}

func TestNuvioClientModeReturnsRawTorrent(t *testing.T) {
	e := engine.NewWithSources(engine.StaticSource{torrentProvider{}}, profileResolverSource{profile: engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionClient,
	}}, 1)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-token/stream/movie/tt0111161.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 1 {
		t.Fatalf("streams = %#v", response.Streams)
	}
	if response.Streams[0].InfoHash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || response.Streams[0].FileIdx == nil || *response.Streams[0].FileIdx != 2 {
		t.Fatalf("stream = %#v", response.Streams[0])
	}
	if response.Streams[0].URL != "" {
		t.Fatalf("unexpected resolved URL %q", response.Streams[0].URL)
	}
	if response.Streams[0].BehaviorHints == nil {
		t.Fatal("missing behavior hints")
	}
	if response.Streams[0].BehaviorHints.Filename != "Movie.1080p.mkv" {
		t.Fatalf("filename hint = %q", response.Streams[0].BehaviorHints.Filename)
	}
	if response.Streams[0].BehaviorHints.VideoSize != 987654321 {
		t.Fatalf("videoSize hint = %d", response.Streams[0].BehaviorHints.VideoSize)
	}
}

func TestNuvioSeriesEpisodeIDFlowsToProviders(t *testing.T) {
	provider := &captureProvider{}
	e := engine.NewWithSources(engine.StaticSource{provider}, profileResolverSource{profile: engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionClient,
	}}, 1)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-token/stream/series/tt1234567:2:3.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if provider.last.Media.Type != "series" || provider.last.Media.ID != "tt1234567:2:3" {
		t.Fatalf("media = %#v", provider.last.Media)
	}
}

func TestNuvioServerModeReturnsResolvedURL(t *testing.T) {
	e := engine.NewWithSources(
		engine.StaticSource{torrentProvider{}},
		resolvingSource{
			profile:  engine.InstallationProfile{ClientMode: "nuvio", ResolutionMode: engine.ResolutionServer},
			resolver: directResolver{},
		},
		1,
	)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-server/stream/movie/tt0111161.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 1 {
		t.Fatalf("streams = %#v", response.Streams)
	}
	if response.Streams[0].URL != "https://cdn.example/video.mkv" || response.Streams[0].InfoHash != "" {
		t.Fatalf("stream = %#v", response.Streams[0])
	}
}

func TestNuvioHybridFallsBackToRawTorrent(t *testing.T) {
	e := engine.NewWithSources(
		engine.StaticSource{torrentProvider{}},
		resolvingSource{
			profile:  engine.InstallationProfile{ClientMode: "nuvio", ResolutionMode: engine.ResolutionHybrid},
			resolver: failingResolver{},
		},
		1,
	)
	handler := NewHandler(e)
	req := httptest.NewRequest(http.MethodGet, "/nuvio-hybrid/stream/movie/tt0111161.json", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 1 {
		t.Fatalf("streams = %#v", response.Streams)
	}
	if response.Streams[0].InfoHash == "" || response.Streams[0].URL != "" {
		t.Fatalf("stream = %#v", response.Streams[0])
	}
}

func TestEmptyStreams(t *testing.T) {
	handler := NewHandler(engine.New())
	req := httptest.NewRequest(http.MethodGet, "/default/stream/movie/tt0111161.json", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 0 {
		t.Fatalf("streams = %d, want 0", len(response.Streams))
	}
}

type directProvider struct {
	url string
}

func (p directProvider) ID() string { return "direct" }

func (p directProvider) Search(context.Context, engine.SearchRequest) ([]domain.Candidate, error) {
	return []domain.Candidate{{
		ID:       "direct",
		SourceID: "direct",
		Kind:     domain.CandidateDirect,
		HTTP:     &domain.HTTPStream{URL: p.url},
	}}, nil
}

func TestRelativeDirectStreamUsesRequestHost(t *testing.T) {
	handler := NewHandlerWithSecureLinks(
		engine.New(directProvider{url: "/api/v1/usenet/stream/token"}),
		true,
	)
	req := httptest.NewRequest(http.MethodGet, "/default/stream/movie/tt0111161.json", nil)
	req.Host = "media.example"
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 1 {
		t.Fatalf("streams = %#v", response.Streams)
	}
	if got := response.Streams[0].URL; got != "https://media.example/api/v1/usenet/stream/token" {
		t.Fatalf("url = %q", got)
	}
}

func TestPrivateDirectStreamIsFiltered(t *testing.T) {
	handler := NewHandler(engine.New(directProvider{url: "http://127.0.0.1/video.mkv"}))
	req := httptest.NewRequest(http.MethodGet, "/default/stream/movie/tt0111161.json", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var response StreamResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode streams: %v", err)
	}
	if len(response.Streams) != 0 {
		t.Fatalf("streams = %d, want 0", len(response.Streams))
	}
}
