package prowlarr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"media-engine/internal/domain"
	"media-engine/internal/engine"
)

const maxBody = 12 << 20

type Provider struct {
	id       string
	endpoint string
	apiKey   string
	client   *http.Client
}

func New(id, endpoint, apiKey string, client *http.Client) (*Provider, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	if err != nil {
		return nil, fmt.Errorf("parse Prowlarr endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Prowlarr endpoint must use http or https")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Prowlarr API key is required")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{id: id, endpoint: parsed.String(), apiKey: strings.TrimSpace(apiKey), client: client}, nil
}

func (p *Provider) ID() string { return p.id }

type indexer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
}

type searchItem struct {
	GUID        string  `json:"guid"`
	AgeHours    float64 `json:"ageHours"`
	Size        int64   `json:"size"`
	Indexer     string  `json:"indexer"`
	Title       string  `json:"title"`
	DownloadURL string  `json:"downloadUrl"`
	MagnetURL   string  `json:"magnetUrl"`
	InfoHash    string  `json:"infoHash"`
	Seeders     *int    `json:"seeders"`
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	indexers, err := p.indexers(ctx)
	if err != nil {
		return nil, err
	}
	torrentIDs := make([]int, 0)
	usenetIDs := make([]int, 0)
	for _, idx := range indexers {
		if !idx.Enable {
			continue
		}
		switch idx.Protocol {
		case "torrent":
			torrentIDs = append(torrentIDs, idx.ID)
		case "usenet":
			usenetIDs = append(usenetIDs, idx.ID)
		}
	}

	query := mediaQuery(req.Media)
	results := make([]domain.Candidate, 0)
	if len(torrentIDs) > 0 {
		items, searchErr := p.search(ctx, query, torrentIDs)
		if searchErr != nil {
			return nil, searchErr
		}
		for _, item := range items {
			if candidate, ok := p.torrentCandidate(req.Media, item); ok {
				results = append(results, candidate)
			}
		}
	}
	if len(usenetIDs) > 0 {
		items, searchErr := p.search(ctx, query, usenetIDs)
		if searchErr != nil {
			return nil, searchErr
		}
		for _, item := range items {
			if candidate, ok := p.usenetCandidate(req.Media, item); ok {
				results = append(results, candidate)
			}
		}
	}
	return results, nil
}

func (p *Provider) indexers(ctx context.Context) ([]indexer, error) {
	var result []indexer
	if err := p.get(ctx, "/api/v1/indexer", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) search(ctx context.Context, query string, ids []int) ([]searchItem, error) {
	values := url.Values{
		"query": {query},
		"type":  {"search"},
		"limit": {"2000"},
	}
	for _, id := range ids {
		values.Add("indexerIds", strconv.Itoa(id))
	}
	var result []searchItem
	if err := p.get(ctx, "/api/v1/search", values, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) get(ctx context.Context, path string, query url.Values, target any) error {
	endpoint, err := url.Parse(p.endpoint + path)
	if err != nil {
		return err
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Api-Key", p.apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Prowlarr returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Prowlarr response: %w", err)
	}
	return nil
}

func mediaQuery(media domain.MediaRef) string {
	parts := strings.Split(media.ID, ":")
	if len(parts) == 0 {
		return media.ID
	}
	if media.Type == "series" && len(parts) >= 3 {
		return fmt.Sprintf("%s S%02sE%02s", parts[0], parts[1], parts[2])
	}
	return parts[0]
}

func (p *Provider) torrentCandidate(media domain.MediaRef, item searchItem) (domain.Candidate, bool) {
	hash := normalizeHash(item.InfoHash)
	if hash == "" {
		hash = extractHash(item.MagnetURL)
	}
	if hash == "" {
		hash = extractHash(item.GUID)
	}
	if hash == "" {
		return domain.Candidate{}, false
	}
	candidate := domain.Candidate{
		ID:        stableID(p.id, item.Title, hash),
		SourceID:  p.id,
		Kind:      domain.CandidateTorrent,
		Media:     media,
		Title:     item.Title,
		SizeBytes: item.Size,
		Seeders:   item.Seeders,
		Torrent:   &domain.TorrentInfo{InfoHash: hash},
	}
	return candidate, true
}

func (p *Provider) usenetCandidate(media domain.MediaRef, item searchItem) (domain.Candidate, bool) {
	nzbURL := strings.TrimSpace(item.DownloadURL)
	if nzbURL == "" && strings.HasPrefix(item.GUID, "http") {
		nzbURL = item.GUID
	}
	if nzbURL == "" {
		return domain.Candidate{}, false
	}
	hash := stableID(nzbURL)
	return domain.Candidate{
		ID:        stableID(p.id, item.Title, hash),
		SourceID:  p.id,
		Kind:      domain.CandidateUsenet,
		Media:     media,
		Title:     item.Title,
		SizeBytes: item.Size,
		Usenet:    &domain.UsenetInfo{NZBURL: nzbURL, Hash: hash},
	}, true
}

func normalizeHash(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 40 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func extractHash(value string) string {
	lower := strings.ToLower(value)
	marker := "btih:"
	index := strings.Index(lower, marker)
	if index < 0 {
		return ""
	}
	value = lower[index+len(marker):]
	for i, r := range value {
		if !((r >= 'a' && r <= 'f') || (r >= '0' && r <= '9')) {
			value = value[:i]
			break
		}
	}
	return normalizeHash(value)
}

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}
