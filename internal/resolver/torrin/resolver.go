package torrin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/resolver/common"
	"streamweave/internal/resolver/httpclient"
)

const (
	DefaultBaseURL = "https://api.torrin.app"
)

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Torrin API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type envelope[T any] struct {
	Data  T         `json:"data"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type magnetFile struct {
	Index int    `json:"index"`
	Link  string `json:"link"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

type magnetData struct {
	ID     string       `json:"id"`
	Hash   string       `json:"hash"`
	Magnet string       `json:"magnet"`
	Name   string       `json:"name"`
	Size   int64        `json:"size"`
	Status string       `json:"status"`
	Files  []magnetFile `json:"files"`
}

type generatedLink struct {
	Link string `json:"link"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var added envelope[magnetData]
	if err := r.client.JSON(ctx, http.MethodPost, "/v0/store/magnets", nil, map[string]string{
		"magnet": common.Magnet(candidate.Torrent.InfoHash),
	}, &added); err != nil {
		return candidate, err
	}
	if added.Error != nil {
		return candidate, errors.New(added.Error.Message)
	}
	if added.Data.ID == "" {
		return candidate, errors.New("Torrin returned an empty magnet id")
	}

	item := added.Data
	if item.Status != "cached" && item.Status != "downloaded" {
		return candidate, common.ErrNotReady
	}

	files := make([]common.File, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, common.File{
			ID:   file.Link,
			Name: file.Name,
			Path: file.Path,
			Size: file.Size,
			Link: file.Link,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("Torrin returned no playable file")
	}

	var generated envelope[generatedLink]
	if err := r.client.JSON(ctx, http.MethodPost, "/v0/store/link/generate", nil, map[string]string{
		"link": selected.Link,
	}, &generated); err != nil {
		return candidate, err
	}
	if generated.Error != nil {
		return candidate, errors.New(generated.Error.Message)
	}
	if generated.Data.Link == "" {
		return candidate, errors.New("Torrin returned an empty playback URL")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: generated.Data.Link}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}
