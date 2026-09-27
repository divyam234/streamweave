package publicindexers

import (
	"bytes"
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

type Kind string

const (
	Knaben        Kind = "knaben"
	ThePirateBay  Kind = "the-pirate-bay"
	TheRARBG      Kind = "therarbg"
	TorrentGalaxy Kind = "torrent-galaxy"
)

var defaults = map[Kind]string{
	Knaben:        "https://api.knaben.org",
	ThePirateBay:  "https://apibay.org",
	TheRARBG:      "https://therarbg.to",
	TorrentGalaxy: "https://torrentgalaxy.one",
}

type Provider struct {
	id       string
	kind     Kind
	endpoint string
	client   *http.Client
}

func New(id string, kind Kind, endpoint string, client *http.Client) (*Provider, error) {
	if _, ok := defaults[kind]; !ok {
		return nil, errors.New("unsupported public indexer")
	}
	if endpoint == "" {
		endpoint = defaults[kind]
	}
	parsed, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid public indexer endpoint")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{id: id, kind: kind, endpoint: parsed.String(), client: client}, nil
}

func (p *Provider) ID() string { return p.id }

type compactResult struct {
	Name    string
	Hash    string
	Size    int64
	Seeders int
	User    string
	IMDbID  string
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	queries := buildQueries(req)
	if len(queries) == 0 {
		return []domain.Candidate{}, nil
	}
	seen := make(map[string]struct{})
	results := make([]domain.Candidate, 0)
	for _, query := range queries {
		items, err := p.search(ctx, query, req)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			hash := normalizeHash(item.Hash)
			if hash == "" {
				continue
			}
			if req.Metadata.IMDbID != "" && item.IMDbID != "" && item.IMDbID != req.Metadata.IMDbID {
				continue
			}
			if _, ok := seen[hash]; ok {
				continue
			}
			seen[hash] = struct{}{}
			seeders := item.Seeders
			results = append(results, domain.Candidate{
				ID: stableID(p.id, hash), SourceID: p.id, Kind: domain.CandidateTorrent,
				Media: req.Media, Title: item.Name, SizeBytes: item.Size, Seeders: &seeders,
				Torrent: &domain.TorrentInfo{InfoHash: hash},
			})
		}
	}
	return results, nil
}

func buildQueries(req engine.SearchRequest) []string {
	seen := map[string]struct{}{}
	add := func(values *[]string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		*values = append(*values, value)
	}
	queries := make([]string, 0, 3)
	title := req.Metadata.Title
	if title != "" {
		if req.Media.Type == "series" && req.Metadata.Season > 0 && req.Metadata.Episode > 0 {
			add(&queries, fmt.Sprintf("%s S%02dE%02d", title, req.Metadata.Season, req.Metadata.Episode))
		} else if req.Metadata.Year > 0 {
			add(&queries, fmt.Sprintf("%s %d", title, req.Metadata.Year))
		}
		add(&queries, title)
	}
	add(&queries, req.Metadata.IMDbID)
	if len(queries) == 0 {
		add(&queries, strings.Split(req.Media.ID, ":")[0])
	}
	return queries
}

func (p *Provider) search(ctx context.Context, query string, req engine.SearchRequest) ([]compactResult, error) {
	switch p.kind {
	case ThePirateBay:
		return p.searchTPB(ctx, query)
	case Knaben:
		return p.searchKnaben(ctx, query, req)
	case TheRARBG, TorrentGalaxy:
		return p.searchGalaxy(ctx, query, req)
	default:
		return nil, errors.New("unsupported public indexer")
	}
}

type tpbResult struct {
	Name     string      `json:"name"`
	InfoHash string      `json:"info_hash"`
	Seeders  json.Number `json:"seeders"`
	Size     json.Number `json:"size"`
	Username string      `json:"username"`
	IMDb     string      `json:"imdb"`
}

func (p *Provider) searchTPB(ctx context.Context, query string) ([]compactResult, error) {
	endpoint, _ := url.Parse(p.endpoint + "/q.php")
	values := endpoint.Query()
	values.Set("q", query)
	values.Set("cat", "200")
	endpoint.RawQuery = values.Encode()

	var payload []tpbResult
	if err := p.jsonRequest(ctx, http.MethodGet, endpoint.String(), nil, &payload); err != nil {
		return nil, err
	}
	results := make([]compactResult, 0, len(payload))
	for _, item := range payload {
		if item.InfoHash == strings.Repeat("0", 40) {
			continue
		}
		size, _ := item.Size.Int64()
		seeders64, _ := item.Seeders.Int64()
		results = append(results, compactResult{
			Name: item.Name, Hash: item.InfoHash, Size: size, Seeders: int(seeders64),
			User: item.Username, IMDbID: item.IMDb,
		})
	}
	return results, nil
}

type galaxyResponse struct {
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
	Results  []galaxyResult `json:"results"`
}

type galaxyResult struct {
	Name    string `json:"n"`
	Size    int64  `json:"s"`
	User    string `json:"u"`
	Seeders int    `json:"se"`
	IMDbID  string `json:"i"`
	Hash    string `json:"h"`
}

func (p *Provider) searchGalaxy(ctx context.Context, query string, req engine.SearchRequest) ([]compactResult, error) {
	categories := make([]string, 0, 2)
	if req.Media.Type == "movie" {
		categories = append(categories, "Movies")
	} else if req.Media.Type == "series" {
		categories = append(categories, "TV", "TV shows")
	}
	if req.Metadata.IsAnime {
		categories = append(categories, "Anime")
	}
	categoryPath := ""
	for _, category := range categories {
		categoryPath += ":category:" + url.PathEscape(category)
	}

	path := ""
	if p.kind == TheRARBG {
		path = "/get-posts/order:-a" + categoryPath + ":keywords:" + url.PathEscape(query) + ":format:json/"
	} else {
		path = "/get-posts/keywords:" + url.PathEscape(query) + categoryPath + ":format:json"
	}
	results := make([]compactResult, 0)
	for page := 1; page <= 5; page++ {
		endpoint, _ := url.Parse(p.endpoint + path)
		values := endpoint.Query()
		values.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = values.Encode()

		var payload galaxyResponse
		if err := p.jsonRequest(ctx, http.MethodGet, endpoint.String(), nil, &payload); err != nil {
			return nil, err
		}
		for _, item := range payload.Results {
			results = append(results, compactResult{
				Name: item.Name, Hash: item.Hash, Size: item.Size, Seeders: item.Seeders,
				User: item.User, IMDbID: item.IMDbID,
			})
		}
		if payload.PageSize <= 0 || page*payload.PageSize >= payload.Total {
			break
		}
	}
	return results, nil
}

type knabenResponse struct {
	Hits []knabenHit `json:"hits"`
}

type knabenHit struct {
	Bytes     int64   `json:"bytes"`
	Hash      *string `json:"hash"`
	MagnetURL *string `json:"magnetUrl"`
	Seeders   int     `json:"seeders"`
	Title     string  `json:"title"`
	Tracker   string  `json:"tracker"`
}

func (p *Provider) searchKnaben(ctx context.Context, query string, req engine.SearchRequest) ([]compactResult, error) {
	categories := []int{}
	if req.Media.Type == "movie" {
		categories = append(categories, 3000000)
	} else if req.Media.Type == "series" {
		categories = append(categories, 2000000)
	}
	if req.Metadata.IsAnime {
		categories = append(categories, 6000000)
	}
	body := map[string]any{
		"search_type": "100%", "search_field": "title", "query": query,
		"order_direction": "desc", "categories": categories, "from": 0, "size": 300,
		"hide_unsafe": false, "hide_xxx": true,
	}
	var payload knabenResponse
	if err := p.jsonRequest(ctx, http.MethodPost, p.endpoint+"/v1", body, &payload); err != nil {
		return nil, err
	}
	results := make([]compactResult, 0, len(payload.Hits))
	for _, item := range payload.Hits {
		hash := ""
		if item.Hash != nil {
			hash = *item.Hash
		}
		if hash == "" && item.MagnetURL != nil {
			hash = extractMagnetHash(*item.MagnetURL)
		}
		results = append(results, compactResult{
			Name: item.Title, Hash: hash, Size: item.Bytes, Seeders: item.Seeders, User: item.Tracker,
		})
	}
	return results, nil
}

func (p *Provider) jsonRequest(ctx context.Context, method, endpoint string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 MediaEngine")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s returned HTTP %d", p.kind, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 12<<20)).Decode(target); err != nil {
		return err
	}
	return nil
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

func extractMagnetHash(value string) string {
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

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}
