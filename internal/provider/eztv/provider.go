package eztv

import (
	"context"
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

const (
	DefaultBaseURL = "https://eztvx.to"
	maxBody        = 8 << 20
	maxPages       = 5
)

type Provider struct {
	id       string
	endpoint string
	client   *http.Client
}

func New(id, endpoint string, client *http.Client) (*Provider, error) {
	if endpoint == "" {
		endpoint = DefaultBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("EZTV endpoint must use http or https")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{id: id, endpoint: parsed.String(), client: client}, nil
}

func (p *Provider) ID() string { return p.id }

type response struct {
	TorrentsCount int       `json:"torrents_count"`
	Limit         int       `json:"limit"`
	Torrents      []torrent `json:"torrents"`
}

type torrent struct {
	Hash             string `json:"hash"`
	Filename         string `json:"filename"`
	MagnetURL        string `json:"magnet_url"`
	Title            string `json:"title"`
	Season           string `json:"season"`
	Episode          string `json:"episode"`
	Seeds            int    `json:"seeds"`
	DateReleasedUnix int64  `json:"date_released_unix"`
	SizeBytes        string `json:"size_bytes"`
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	if req.Media.Type != "series" {
		return []domain.Candidate{}, nil
	}
	imdbID, season, episode := parseSeriesID(req.Media.ID)
	if imdbID == "" || season == "" || episode == "" {
		return []domain.Candidate{}, nil
	}
	imdbID = strings.TrimPrefix(strings.ToLower(imdbID), "tt")

	first, err := p.page(ctx, imdbID, 1)
	if err != nil {
		return nil, err
	}
	all := append([]torrent(nil), first.Torrents...)
	pages := 1
	if first.Limit > 0 && first.TorrentsCount > len(first.Torrents) {
		pages = (first.TorrentsCount + first.Limit - 1) / first.Limit
		if pages > maxPages {
			pages = maxPages
		}
	}
	for page := 2; page <= pages; page++ {
		next, pageErr := p.page(ctx, imdbID, page)
		if pageErr != nil {
			return nil, pageErr
		}
		all = append(all, next.Torrents...)
	}

	results := make([]domain.Candidate, 0)
	seen := make(map[string]struct{})
	for _, item := range all {
		if item.Season != season {
			continue
		}
		if item.Episode != episode && item.Episode != "0" {
			continue
		}
		hash := normalizeHash(item.Hash)
		if hash == "" {
			hash = extractHash(item.MagnetURL)
		}
		if hash == "" {
			continue
		}
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}

		size, _ := strconv.ParseInt(item.SizeBytes, 10, 64)
		seeders := item.Seeds
		title := item.Title
		if title == "" {
			title = item.Filename
		}
		results = append(results, domain.Candidate{
			ID:        p.id + ":" + hash,
			SourceID:  p.id,
			Kind:      domain.CandidateTorrent,
			Media:     req.Media,
			Title:     title,
			Filename:  item.Filename,
			SizeBytes: size,
			Seeders:   &seeders,
			Torrent:   &domain.TorrentInfo{InfoHash: hash},
		})
	}
	return results, nil
}

func (p *Provider) page(ctx context.Context, imdbID string, page int) (response, error) {
	endpoint, err := url.Parse(p.endpoint + "/api/get-torrents")
	if err != nil {
		return response{}, err
	}
	query := endpoint.Query()
	query.Set("imdb_id", imdbID)
	query.Set("limit", "100")
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return response{}, err
	}
	request.Header.Set("Accept", "application/json")
	res, err := p.client.Do(request)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return response{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return response{}, fmt.Errorf("EZTV returned HTTP %d", res.StatusCode)
	}
	var payload response
	if err := json.Unmarshal(body, &payload); err != nil {
		return response{}, fmt.Errorf("decode EZTV response: %w", err)
	}
	return payload, nil
}

func parseSeriesID(raw string) (id, season, episode string) {
	parts := strings.Split(raw, ":")
	if len(parts) > 0 {
		id = parts[0]
	}
	if len(parts) > 1 {
		season = strings.TrimLeft(parts[1], "0")
		if season == "" {
			season = "0"
		}
	}
	if len(parts) > 2 {
		episode = strings.TrimLeft(parts[2], "0")
		if episode == "" {
			episode = "0"
		}
	}
	return
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
	index := strings.Index(lower, "btih:")
	if index < 0 {
		return ""
	}
	value = lower[index+5:]
	for i, r := range value {
		if !((r >= 'a' && r <= 'f') || (r >= '0' && r <= '9')) {
			value = value[:i]
			break
		}
	}
	return normalizeHash(value)
}
