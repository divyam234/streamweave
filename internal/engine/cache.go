package engine

import (
	"sync"
	"time"

	"streamweave/internal/domain"
)

const (
	defaultDiscoveryCacheTTL = 30 * time.Second
	defaultDiscoveryCacheMax = 512
)

const defaultDiscoveryStaleMax = 10 * time.Minute

type cacheEntry struct {
	expires    time.Time
	stored     time.Time
	candidates []domain.Candidate
}

type discoveryCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]cacheEntry
}

func newDiscoveryCache(ttl time.Duration, max int) *discoveryCache {
	if ttl <= 0 {
		ttl = defaultDiscoveryCacheTTL
	}
	if max <= 0 {
		max = defaultDiscoveryCacheMax
	}
	return &discoveryCache{ttl: ttl, max: max, entries: make(map[string]cacheEntry)}
}

func (c *discoveryCache) get(key string) ([]domain.Candidate, bool) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !entry.expires.After(now) {
		return nil, false
	}
	return cloneCandidates(entry.candidates), true
}

// getStale returns the last known entry even after its freshness TTL
// expired, so a transient upstream outage can serve slightly stale results
// instead of an empty list. Entries older than the stale horizon are
// dropped. Empty results are never stored, so stale entries always hold
// at least one candidate.
func (c *discoveryCache) getStale(key string) ([]domain.Candidate, bool) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.stored) > defaultDiscoveryStaleMax {
		delete(c.entries, key)
		return nil, false
	}
	return cloneCandidates(entry.candidates), true
}

func (c *discoveryCache) set(key string, candidates []domain.Candidate) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		var oldestKey string
		var oldest time.Time
		for key, entry := range c.entries {
			if now.Sub(entry.stored) > defaultDiscoveryStaleMax {
				delete(c.entries, key)
				continue
			}
			if oldestKey == "" || entry.stored.Before(oldest) {
				oldestKey, oldest = key, entry.stored
			}
		}
		if len(c.entries) >= c.max && oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = cacheEntry{expires: now.Add(c.ttl), stored: now, candidates: cloneCandidates(candidates)}
}

func cloneCandidates(input []domain.Candidate) []domain.Candidate {
	output := make([]domain.Candidate, len(input))
	for i, candidate := range input {
		output[i] = candidate
		if candidate.Languages != nil {
			output[i].Languages = append([]string(nil), candidate.Languages...)
		}
		if candidate.Torrent != nil {
			torrent := *candidate.Torrent
			if candidate.Torrent.FileIndex != nil {
				index := *candidate.Torrent.FileIndex
				torrent.FileIndex = &index
			}
			output[i].Torrent = &torrent
		}
		if candidate.Usenet != nil {
			usenet := *candidate.Usenet
			output[i].Usenet = &usenet
		}
		if candidate.HTTP != nil {
			stream := *candidate.HTTP
			if candidate.HTTP.Headers != nil {
				stream.Headers = make(map[string]string, len(candidate.HTTP.Headers))
				for key, value := range candidate.HTTP.Headers {
					stream.Headers[key] = value
				}
			}
			output[i].HTTP = &stream
		}
		if candidate.Cached != nil {
			value := *candidate.Cached
			output[i].Cached = &value
		}
		if candidate.Seeders != nil {
			value := *candidate.Seeders
			output[i].Seeders = &value
		}
	}
	return output
}
