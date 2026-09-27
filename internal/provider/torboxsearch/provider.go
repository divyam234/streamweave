package torboxsearch

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
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

const (
	DefaultBaseURL = "https://search-api.torbox.app"
	maxBody        = 12 << 20
)

type Provider struct {
	id       string
	endpoint string
	apiKey   string
	client   *http.Client
}

func New(id, endpoint, apiKey string, client *http.Client) (*Provider, error) {
	if endpoint == "" {
		endpoint = DefaultBaseURL
	}
	u, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("TorBox Search endpoint must use http or https")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("TorBox Search API key is required")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{id: id, endpoint: u.String(), apiKey: strings.TrimSpace(apiKey), client: client}, nil
}
func (p *Provider) ID() string { return p.id }

type envelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Detail  string `json:"detail"`
	Message string `json:"message"`
	Data    *data  `json:"data"`
}
type data struct {
	Torrents []result `json:"torrents"`
	NZBs     []result `json:"nzbs"`
}
type result struct {
	Hash             string  `json:"hash"`
	RawTitle         string  `json:"raw_title"`
	Title            string  `json:"title"`
	Magnet           *string `json:"magnet"`
	LastKnownSeeders int     `json:"last_known_seeders"`
	Size             int64   `json:"size"`
	Tracker          string  `json:"tracker"`
	Type             string  `json:"type"`
	NZB              *string `json:"nzb"`
	Cached           *bool   `json:"cached"`
	Owned            *bool   `json:"owned"`
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	id, season, episode := parseID(req.Media.ID)
	if id == "" {
		return []domain.Candidate{}, nil
	}
	var all []domain.Candidate
	torrents, err := p.fetch(ctx, "/torrents/imdb_id:"+url.PathEscape(id), season, episode)
	if err != nil {
		return nil, err
	}
	for _, item := range torrents.Torrents {
		if c, ok := p.torrentCandidate(req.Media, item); ok {
			all = append(all, c)
		}
	}
	usenet, err := p.fetch(ctx, "/usenet/imdb_id:"+url.PathEscape(id), season, episode)
	if err != nil {
		return nil, err
	}
	for _, item := range usenet.NZBs {
		if c, ok := p.usenetCandidate(req.Media, item); ok {
			all = append(all, c)
		}
	}
	return all, nil
}

func (p *Provider) fetch(ctx context.Context, path, season, episode string) (data, error) {
	endpoint, err := url.Parse(p.endpoint + path)
	if err != nil {
		return data{}, err
	}
	q := endpoint.Query()
	q.Set("check_cache", "true")
	q.Set("check_owned", "true")
	q.Set("metadata", "false")
	if season != "" {
		q.Set("season", season)
	}
	if episode != "" {
		q.Set("episode", episode)
	}
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return data{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return data{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return data{}, err
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return data{}, fmt.Errorf("decode TorBox Search response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || !env.Success {
		msg := env.Detail
		if msg == "" {
			msg = env.Message
		}
		if msg == "" {
			msg = env.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return data{}, errors.New(msg)
	}
	if env.Data == nil {
		return data{}, nil
	}
	return *env.Data, nil
}

func (p *Provider) torrentCandidate(media domain.MediaRef, item result) (domain.Candidate, bool) {
	hash := normalizeHash(item.Hash)
	if hash == "" && item.Magnet != nil {
		hash = extractHash(*item.Magnet)
	}
	if hash == "" {
		return domain.Candidate{}, false
	}
	title := item.RawTitle
	if title == "" {
		title = item.Title
	}
	var seeders *int
	if item.LastKnownSeeders >= 0 {
		v := item.LastKnownSeeders
		seeders = &v
	}
	return domain.Candidate{
		ID: stableID(p.id, title, hash), SourceID: p.id, Kind: domain.CandidateTorrent,
		Media: media, Title: title, SizeBytes: item.Size, Seeders: seeders,
		Torrent: &domain.TorrentInfo{InfoHash: hash}, Cached: item.Cached,
	}, true
}

func (p *Provider) usenetCandidate(media domain.MediaRef, item result) (domain.Candidate, bool) {
	if item.NZB == nil || strings.TrimSpace(*item.NZB) == "" {
		return domain.Candidate{}, false
	}
	title := item.RawTitle
	if title == "" {
		title = item.Title
	}
	nzb := strings.TrimSpace(*item.NZB)
	hash := stableID(nzb)
	return domain.Candidate{
		ID: stableID(p.id, title, hash), SourceID: p.id, Kind: domain.CandidateUsenet,
		Media: media, Title: title, SizeBytes: item.Size, Cached: item.Cached,
		Usenet: &domain.UsenetInfo{NZBURL: nzb, Hash: hash},
	}, true
}

func parseID(raw string) (id, season, episode string) {
	parts := strings.Split(raw, ":")
	if len(parts) > 0 {
		id = parts[0]
	}
	if len(parts) > 1 {
		season = parts[1]
	}
	if len(parts) > 2 {
		episode = parts[2]
	}
	return
}
func normalizeHash(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) != 40 {
		return ""
	}
	if _, err := hex.DecodeString(v); err != nil {
		return ""
	}
	return v
}
func extractHash(v string) string {
	l := strings.ToLower(v)
	i := strings.Index(l, "btih:")
	if i < 0 {
		return ""
	}
	v = l[i+5:]
	for i, r := range v {
		if !((r >= 'a' && r <= 'f') || (r >= '0' && r <= '9')) {
			v = v[:i]
			break
		}
	}
	return normalizeHash(v)
}
func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}
