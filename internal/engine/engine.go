package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"streamweave/internal/domain"
)

const (
	defaultMaxConcurrency    = 8
	defaultMetadataTimeout   = 3 * time.Second
	defaultProviderTimeout   = 8 * time.Second
	defaultResolverTimeout   = 18 * time.Second
	defaultMaxCandidates     = 100
	defaultMaxResolveTargets = 20
)

var ErrInstallationUnavailable = errors.New("installation is unavailable")

type SearchMetadata struct {
	Title   string
	Year    int
	IMDbID  string
	Season  int
	Episode int
	IsAnime bool
}

type SearchRequest struct {
	InstallationID string
	Media          domain.MediaRef
	Metadata       SearchMetadata
}

type MetadataSource interface {
	Lookup(context.Context, domain.MediaRef) (SearchMetadata, error)
}

type Provider interface {
	ID() string
	Search(context.Context, SearchRequest) ([]domain.Candidate, error)
}

type ProviderSource interface {
	Providers(context.Context) ([]Provider, error)
}

type Resolver interface {
	ID() string
	Resolve(context.Context, domain.Candidate) (domain.Candidate, error)
}

type ResolutionMode string

const (
	ResolutionClient ResolutionMode = "client"
	ResolutionServer ResolutionMode = "server"
	ResolutionHybrid ResolutionMode = "hybrid"
)

type Resolution struct {
	Mode     ResolutionMode
	Resolver Resolver
}

type InstallationProfile struct {
	ClientMode     string
	ResolutionMode ResolutionMode
}

type InstallationProfiler interface {
	InstallationProfile(context.Context, string) (InstallationProfile, error)
}

type ResolverSource interface {
	ResolutionFor(context.Context, string) (Resolution, error)
}

type InstallationValidator interface {
	ValidateInstallation(context.Context, string) error
}

type StaticSource []Provider

func (s StaticSource) Providers(context.Context) ([]Provider, error) {
	return []Provider(s), nil
}

type NoResolvers struct{}

func (NoResolvers) ResolutionFor(context.Context, string) (Resolution, error) {
	return Resolution{Mode: ResolutionClient}, nil
}

type Engine struct {
	providers      ProviderSource
	resolvers      ResolverSource
	maxConcurrency int
	metadata       MetadataSource
	logger         *slog.Logger
	discoveryCache *discoveryCache
	discoveryGroup singleflight.Group
	resolveGroup   singleflight.Group
}

func New(providers ...Provider) *Engine {
	return NewWithSources(StaticSource(providers), NoResolvers{}, defaultMaxConcurrency)
}

func NewWithSource(source ProviderSource, maxConcurrency int) *Engine {
	return NewWithSources(source, NoResolvers{}, maxConcurrency)
}

func NewWithSources(providers ProviderSource, resolvers ResolverSource, maxConcurrency int) *Engine {
	if providers == nil {
		providers = StaticSource{}
	}
	if resolvers == nil {
		resolvers = NoResolvers{}
	}
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}
	return &Engine{
		providers:      providers,
		resolvers:      resolvers,
		maxConcurrency: maxConcurrency,
		discoveryCache: newDiscoveryCache(defaultDiscoveryCacheTTL, defaultDiscoveryCacheMax),
	}
}

func (e *Engine) SetMetadataSource(source MetadataSource) {
	e.metadata = source
}

func (e *Engine) SetLogger(logger *slog.Logger) {
	e.logger = logger
}

func (e *Engine) log() *slog.Logger {
	if e.logger != nil {
		return e.logger
	}
	return slog.Default()
}

func (e *Engine) ValidateInstallation(ctx context.Context, installationID string) error {
	if validator, ok := e.resolvers.(InstallationValidator); ok {
		return validator.ValidateInstallation(ctx, installationID)
	}
	return nil
}

func (e *Engine) InstallationProfile(ctx context.Context, installationID string) (InstallationProfile, error) {
	if profiler, ok := e.resolvers.(InstallationProfiler); ok {
		return profiler.InstallationProfile(ctx, installationID)
	}
	if err := e.ValidateInstallation(ctx, installationID); err != nil {
		return InstallationProfile{}, err
	}
	return InstallationProfile{ClientMode: "universal", ResolutionMode: ResolutionClient}, nil
}

func (e *Engine) Search(ctx context.Context, req SearchRequest) ([]domain.Candidate, error) {
	return e.search(ctx, req, false)
}

// SearchUnresolved prepares results without contacting the resolver; playback resolves one item.
func (e *Engine) SearchUnresolved(ctx context.Context, req SearchRequest) ([]domain.Candidate, error) {
	return e.search(ctx, req, true)
}

func (e *Engine) search(ctx context.Context, req SearchRequest, lazy bool) ([]domain.Candidate, error) {
	if err := e.ValidateInstallation(ctx, req.InstallationID); err != nil {
		return nil, err
	}
	if e.metadata != nil && req.Metadata.Title == "" {
		metadataCtx, cancel := context.WithTimeout(ctx, defaultMetadataTimeout)
		metadata, metadataErr := e.metadata.Lookup(metadataCtx, req.Media)
		cancel()
		if metadataErr == nil {
			req.Metadata = metadata
		}
	}
	candidates, err := e.discoverCached(ctx, req)
	if err != nil {
		return nil, err
	}
	candidates = prepareCandidates(candidates)
	if len(candidates) > defaultMaxCandidates {
		candidates = candidates[:defaultMaxCandidates]
	}
	if lazy {
		return candidates, nil
	}

	resolution, err := e.resolvers.ResolutionFor(ctx, req.InstallationID)
	if err != nil {
		return nil, err
	}
	if resolution.Mode == ResolutionClient || resolution.Resolver == nil || len(candidates) == 0 {
		if resolution.Mode == ResolutionServer && resolution.Resolver == nil {
			return []domain.Candidate{}, nil
		}
		return candidates, nil
	}

	resolved, err := e.resolve(ctx, candidates, resolution)
	if err != nil {
		return nil, err
	}
	final := prepareCandidates(resolved)
	if len(final) > defaultMaxCandidates {
		final = final[:defaultMaxCandidates]
	}
	return final, nil
}

func (e *Engine) ResolveSelected(ctx context.Context, installationID string, candidate domain.Candidate) (domain.Candidate, error) {
	resolution, err := e.resolvers.ResolutionFor(ctx, installationID)
	if err != nil {
		return candidate, err
	}
	if resolution.Mode == ResolutionClient || resolution.Resolver == nil {
		return candidate, errors.New("resolver unavailable")
	}
	return resolution.Resolver.Resolve(ctx, candidate)
}

func (e *Engine) discoverCached(ctx context.Context, req SearchRequest) ([]domain.Candidate, error) {
	key := req.InstallationID + ":" + req.Media.Type + ":" + req.Media.ID
	if candidates, ok := e.discoveryCache.get(key); ok {
		return candidates, nil
	}
	value, err, _ := e.discoveryGroup.Do(key, func() (any, error) {
		if candidates, ok := e.discoveryCache.get(key); ok {
			return candidates, nil
		}
		candidates, discoverErr := e.discover(ctx, req)
		if discoverErr != nil {
			return nil, discoverErr
		}
		// Empty results are never cached: they usually mean a transient
		// upstream failure, and caching them would keep serving
		// "no streams found" until the entry expires.
		if len(candidates) != 0 {
			e.discoveryCache.set(key, candidates)
		}
		return candidates, nil
	})
	if err != nil {
		return nil, err
	}
	candidates, ok := value.([]domain.Candidate)
	if !ok {
		return nil, errors.New("discovery returned unexpected result")
	}
	return cloneCandidates(candidates), nil
}

func (e *Engine) discover(ctx context.Context, req SearchRequest) ([]domain.Candidate, error) {
	providers, err := e.providers.Providers(ctx)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return []domain.Candidate{}, nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(e.maxConcurrency)

	perProvider := make([][]domain.Candidate, len(providers))
	for index, provider := range providers {
		index, provider := index, provider
		group.Go(func() error {
			providerCtx, cancel := context.WithTimeout(groupCtx, defaultProviderTimeout)
			defer cancel()
			candidates, providerErr := provider.Search(providerCtx, req)
			if providerErr != nil {
				e.log().WarnContext(ctx, "provider discovery failed",
					"provider", provider.ID(),
					"type", req.Media.Type,
					"id", req.Media.ID,
					"error", providerErr,
				)
				return nil
			}
			perProvider[index] = candidates
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	total := 0
	for _, candidates := range perProvider {
		total += len(candidates)
	}
	results := make([]domain.Candidate, 0, total)
	for _, candidates := range perProvider {
		results = append(results, candidates...)
	}
	return results, nil
}

func (e *Engine) resolve(ctx context.Context, candidates []domain.Candidate, resolution Resolution) ([]domain.Candidate, error) {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(e.maxConcurrency)

	resolved := make([]domain.Candidate, len(candidates))
	keep := make([]bool, len(candidates))
	resolveAllowed := make([]bool, len(candidates))
	resolveTargets := 0
	for index, candidate := range candidates {
		if candidate.Torrent == nil && candidate.Usenet == nil {
			continue
		}
		if resolveTargets < defaultMaxResolveTargets {
			resolveAllowed[index] = true
			resolveTargets++
		}
	}
	for index, candidate := range candidates {
		index, candidate := index, candidate
		group.Go(func() error {
			if candidate.Torrent == nil && candidate.Usenet == nil {
				resolved[index], keep[index] = candidate, true
				return nil
			}
			if !resolveAllowed[index] {
				if resolution.Mode == ResolutionHybrid {
					resolved[index], keep[index] = candidate, true
				}
				return nil
			}

			resolveCtx, cancel := context.WithTimeout(groupCtx, defaultResolverTimeout)
			defer cancel()
			result, err := e.resolveOne(resolveCtx, resolution.Resolver, candidate)
			if err != nil {
				if resolution.Mode == ResolutionHybrid {
					resolved[index], keep[index] = candidate, true
				}
				return nil
			}

			if resolution.Mode == ResolutionServer && result.HTTP == nil {
				return nil
			}
			resolved[index], keep[index] = result, true
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	results := make([]domain.Candidate, 0, len(candidates))
	for index := range resolved {
		if keep[index] {
			results = append(results, resolved[index])
		}
	}
	return results, nil
}

func (e *Engine) resolveOne(ctx context.Context, resolver Resolver, candidate domain.Candidate) (domain.Candidate, error) {
	key := resolver.ID() + ":" + candidate.ID
	value, err, _ := e.resolveGroup.Do(key, func() (any, error) {
		return resolver.Resolve(ctx, candidate)
	})
	if err != nil {
		return candidate, err
	}
	result, ok := value.(domain.Candidate)
	if !ok {
		return candidate, fmt.Errorf("resolver %s returned unexpected result", resolver.ID())
	}
	return result, nil
}
