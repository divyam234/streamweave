package tsukihime

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

const (
	DefaultAPIURL     = "https://api.tsukihime.org"
	DefaultStorageURL = "https://storage.tsukihime.org"
)

type Provider struct {
	id         string
	apiURL     string
	storageURL string
	client     *http.Client
}

func New(id, apiURL, storageURL string, client *http.Client) (*Provider, error) {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	if storageURL == "" {
		storageURL = DefaultStorageURL
	}
	for _, raw := range []string{apiURL, storageURL} {
		parsed, err := url.Parse(strings.TrimRight(raw, "/"))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, errors.New("invalid Tsukihime URL")
		}
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{
		id: id, apiURL: strings.TrimRight(apiURL, "/"),
		storageURL: strings.TrimRight(storageURL, "/"), client: client,
	}, nil
}

func (p *Provider) ID() string { return p.id }

type response struct {
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Results []result `json:"results"`
}

type result struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Hash       string   `json:"btih"`
	Size       int64    `json:"totalsize"`
	AudioLangs []string `json:"audiolangs"`
	SubLangs   []string `json:"sublangs"`
	HasNZB     int      `json:"has_nzb"`
	AnimeTosho bool     `json:"animetosho"`
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	query := strings.TrimSpace(req.Metadata.Title)
	if query == "" {
		query = strings.TrimSpace(strings.Split(req.Media.ID, ":")[0])
	}
	if query == "" {
		return []domain.Candidate{}, nil
	}
	if req.Media.Type == "series" && req.Metadata.Season > 0 && req.Metadata.Episode > 0 {
		query = fmt.Sprintf("%s S%02dE%02d", query, req.Metadata.Season, req.Metadata.Episode)
	} else if req.Media.Type == "movie" && req.Metadata.Year > 0 {
		query = fmt.Sprintf("%s %d", query, req.Metadata.Year)
	}

	seen := make(map[string]struct{})
	candidates := make([]domain.Candidate, 0)
	for page := 1; page <= 5; page++ {
		payload, err := p.search(ctx, query, page)
		if err != nil {
			return nil, err
		}
		for _, item := range payload.Results {
			hash := normalizeHash(item.Hash)
			if hash == "" {
				continue
			}
			key := "torrent:" + hash
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				candidates = append(candidates, domain.Candidate{
					ID: stableID(p.id, key), SourceID: p.id, Kind: domain.CandidateTorrent,
					Media: req.Media, Title: item.Name, SizeBytes: item.Size,
					Languages: append([]string(nil), item.AudioLangs...),
					Torrent:   &domain.TorrentInfo{InfoHash: hash},
				})
			}
			if item.HasNZB == 1 && !item.AnimeTosho {
				nzbURL := p.nzbURL(item)
				key = "usenet:" + nzbURL
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				sum := sha256.Sum256([]byte(nzbURL))
				candidates = append(candidates, domain.Candidate{
					ID: stableID(p.id, key), SourceID: p.id, Kind: domain.CandidateUsenet,
					Media: req.Media, Title: item.Name, SizeBytes: item.Size,
					Languages: append([]string(nil), item.AudioLangs...),
					Usenet: &domain.UsenetInfo{
						NZBURL:  nzbURL,
						Hash:    hex.EncodeToString(sum[:]),
						Indexer: "Tsukihime",
					},
				})
			}
		}
		if payload.Limit <= 0 || page*payload.Limit >= payload.Total {
			break
		}
	}
	return candidates, nil
}

func (p *Provider) search(ctx context.Context, query string, page int) (response, error) {
	endpoint, _ := url.Parse(p.apiURL + "/v1/search/torrents")
	values := endpoint.Query()
	values.Set("q", query)
	values.Set("limit", "100")
	values.Set("offset", strconv.Itoa((page-1)*100))
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return response{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 MediaEngine")
	res, err := p.client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return response{}, fmt.Errorf("Tsukihime HTTP %d", res.StatusCode)
	}
	var payload response
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&payload); err != nil {
		return response{}, err
	}
	return payload, nil
}

func (p *Provider) nzbURL(item result) string {
	name := strings.ReplaceAll(item.Name, "/", "_") + ".nzb"
	return fmt.Sprintf("%s/nzbs/%d/%s", p.storageURL, item.ID, url.PathEscape(name))
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

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}
