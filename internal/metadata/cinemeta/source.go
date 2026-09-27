package cinemeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

const DefaultBaseURL = "https://v3-cinemeta.strem.io"

type Source struct {
	baseURL string
	client  *http.Client
}

func New(baseURL string, client *http.Client) *Source {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Source{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type metaResponse struct {
	Meta struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		ReleaseInfo string   `json:"releaseInfo"`
		Year        string   `json:"year"`
		Genres      []string `json:"genres"`
	} `json:"meta"`
}

func (s *Source) Lookup(ctx context.Context, media domain.MediaRef) (engine.SearchMetadata, error) {
	if s.client == nil {
		return engine.SearchMetadata{}, errors.New("http client is required")
	}
	id, season, episode := splitID(media.ID)
	if id == "" {
		return engine.SearchMetadata{}, errors.New("media id is required")
	}
	endpoint := fmt.Sprintf("%s/meta/%s/%s.json", s.baseURL, url.PathEscape(media.Type), url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return engine.SearchMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return engine.SearchMetadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return engine.SearchMetadata{}, fmt.Errorf("Cinemeta returned HTTP %d", res.StatusCode)
	}
	var payload metaResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&payload); err != nil {
		return engine.SearchMetadata{}, err
	}
	year := parseYear(payload.Meta.Year)
	if year == 0 {
		year = parseYear(payload.Meta.ReleaseInfo)
	}
	isAnime := false
	for _, genre := range payload.Meta.Genres {
		if strings.EqualFold(strings.TrimSpace(genre), "anime") {
			isAnime = true
			break
		}
	}
	return engine.SearchMetadata{
		Title:   payload.Meta.Name,
		Year:    year,
		IMDbID:  id,
		Season:  season,
		Episode: episode,
		IsAnime: isAnime,
	}, nil
}

func splitID(raw string) (string, int, int) {
	parts := strings.Split(raw, ":")
	if len(parts) == 0 {
		return "", 0, 0
	}
	season, episode := 0, 0
	if len(parts) > 1 {
		season, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		episode, _ = strconv.Atoi(parts[2])
	}
	return parts[0], season, episode
}

func parseYear(value string) int {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if len(field) == 4 {
			year, err := strconv.Atoi(field)
			if err == nil && year >= 1800 && year <= 3000 {
				return year
			}
		}
	}
	return 0
}
