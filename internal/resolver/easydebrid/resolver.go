package easydebrid

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/resolver/common"
	"streamweave/internal/resolver/httpclient"
)

const DefaultBaseURL = "https://easydebrid.com/api"

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("EasyDebrid API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type generateResponse struct {
	Error string         `json:"error"`
	Files []generateFile `json:"files"`
}

type generateFile struct {
	Filename  string   `json:"filename"`
	Directory []string `json:"directory"`
	Size      int64    `json:"size"`
	URL       string   `json:"url"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var response generateResponse
	if err := r.client.JSON(ctx, http.MethodPost, "/v1/link/generate", nil, map[string]string{
		"url": common.Magnet(candidate.Torrent.InfoHash),
	}, &response); err != nil {
		return candidate, err
	}
	if response.Error != "" {
		return candidate, errors.New(response.Error)
	}

	files := make([]common.File, 0, len(response.Files))
	for _, item := range response.Files {
		path := strings.Join(append(append([]string(nil), item.Directory...), item.Filename), "/")
		files = append(files, common.File{
			Name: item.Filename,
			Path: path,
			Size: item.Size,
			Link: item.URL,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("EasyDebrid returned no playable file")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: selected.Link}
	candidate.Filename = filepath.Base(selected.Name)
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}
