package debridlink

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"streamweave/internal/domain"
	"streamweave/internal/resolver/common"
	"streamweave/internal/resolver/httpclient"
)

const (
	DefaultBaseURL = "https://debrid-link.com/api"
	pollInterval   = 2 * time.Second
	maxWait        = 60 * time.Second
)

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Debrid-Link API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type response[T any] struct {
	Success bool   `json:"success"`
	Value   T      `json:"value"`
	Error   string `json:"error"`
	ErrorID string `json:"error_id"`
	Desc    string `json:"error_description"`
}

type paginated[T any] struct {
	response[[]T]
	Pagination struct {
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"pagination"`
}

type torrentFile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DownloadURL     string `json:"downloadUrl"`
	Downloaded      bool   `json:"downloaded"`
	Size            int64  `json:"size"`
	DownloadPercent int    `json:"downloadPercent"`
}

type torrent struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	HashString      string        `json:"hashString"`
	Downloaded      bool          `json:"downloaded"`
	DownloadPercent int           `json:"downloadPercent"`
	TotalSize       int64         `json:"totalSize"`
	Files           []torrentFile `json:"files"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var added response[torrent]
	if err := r.client.JSON(ctx, http.MethodPost, "/v2/seedbox/add", nil, map[string]any{
		"url":           common.Magnet(candidate.Torrent.InfoHash),
		"wait":          false,
		"structureType": "list",
	}, &added); err != nil {
		return candidate, err
	}
	if !added.Success || added.Value.ID == "" {
		return candidate, errors.New(responseError(added.Error, added.Desc, "Debrid-Link failed to add torrent"))
	}

	item := added.Value
	deadline := time.Now().Add(maxWait)
	for item.DownloadPercent < 100 && !item.Downloaded {
		if time.Now().After(deadline) {
			cached := false
			candidate.Cached = &cached
			return candidate, common.ErrNotReady
		}
		select {
		case <-ctx.Done():
			return candidate, ctx.Err()
		case <-time.After(pollInterval):
		}

		var listed paginated[torrent]
		query := url.Values{
			"ids":       {item.ID},
			"structure": {"list"},
			"perPage":   {"20"},
			"page":      {"0"},
		}
		if err := r.client.Get(ctx, "/v2/seedbox/list", query, &listed); err != nil {
			return candidate, err
		}
		if !listed.Success {
			return candidate, errors.New(responseError(listed.Error, listed.Desc, "Debrid-Link failed to read torrent"))
		}
		if len(listed.Value) == 0 {
			continue
		}
		item = listed.Value[0]
	}

	files := make([]common.File, 0, len(item.Files))
	for index, file := range item.Files {
		files = append(files, common.File{
			ID:   firstNonEmpty(file.ID, strconv.Itoa(index)),
			Name: file.Name,
			Path: file.Name,
			Size: file.Size,
			Link: file.DownloadURL,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("Debrid-Link torrent has no playable file")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: selected.Link}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func responseError(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "upstream error"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
