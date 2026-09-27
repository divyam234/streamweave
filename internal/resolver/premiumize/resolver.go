package premiumize

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/common"
	"media-engine/internal/resolver/httpclient"
)

const DefaultBaseURL = "https://www.premiumize.me/api"

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Premiumize API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Query.Set("apikey", apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type directDLResponse struct {
	Status   string            `json:"status"`
	Message  string            `json:"message"`
	Location string            `json:"location"`
	Filename string            `json:"filename"`
	Filesize int64             `json:"filesize"`
	Content  []directDLContent `json:"content"`
}

type directDLContent struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Link       string `json:"link"`
	StreamLink string `json:"stream_link"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var response directDLResponse
	if err := r.client.Form(
		ctx,
		http.MethodPost,
		"/transfer/directdl",
		nil,
		url.Values{"src": {common.Magnet(candidate.Torrent.InfoHash)}},
		&response,
	); err != nil {
		return candidate, err
	}
	if response.Status != "" && response.Status != "success" {
		if response.Message == "" {
			response.Message = "Premiumize direct download failed"
		}
		return candidate, errors.New(response.Message)
	}

	files := make([]common.File, 0, len(response.Content))
	for index, item := range response.Content {
		link := item.StreamLink
		if link == "" {
			link = item.Link
		}
		files = append(files, common.File{
			ID:   strconv.Itoa(index),
			Name: filepath.Base(item.Path),
			Path: item.Path,
			Size: item.Size,
			Link: link,
		})
	}

	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok {
		if response.Location == "" {
			return candidate, errors.New("Premiumize returned no playable file")
		}
		selected = common.File{
			Name: response.Filename,
			Size: response.Filesize,
			Link: response.Location,
		}
	}
	if selected.Link == "" {
		return candidate, errors.New("Premiumize returned an empty playback URL")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: selected.Link}
	if selected.Name != "" {
		candidate.Filename = selected.Name
	}
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}
