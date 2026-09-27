package offcloud

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/common"
	"media-engine/internal/resolver/httpclient"
)

const (
	DefaultBaseURL = "https://offcloud.com"
	pollInterval   = 3 * time.Second
	maxWait        = 60 * time.Second
)

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Offcloud API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type addResponse struct {
	Error        string `json:"error"`
	NotAvailable string `json:"not_available"`
	RequestID    string `json:"requestId"`
	FileName     string `json:"fileName"`
	Status       string `json:"status"`
	OriginalLink string `json:"originalLink"`
}

type historyItem struct {
	RequestID string `json:"requestId"`
	FileName  string `json:"fileName"`
	Status    string `json:"status"`
}

type exploreResponse struct {
	Error string        `json:"error"`
	Files []exploreFile `json:"files"`
}

type exploreFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var added addResponse
	if err := r.client.JSON(ctx, http.MethodPost, "/api/cloud", nil, map[string]string{
		"url": common.Magnet(candidate.Torrent.InfoHash),
	}, &added); err != nil {
		return candidate, err
	}
	if added.Error != "" {
		return candidate, errors.New(added.Error)
	}
	if added.NotAvailable != "" {
		return candidate, common.ErrNotReady
	}
	if added.RequestID == "" {
		return candidate, errors.New("Offcloud returned an empty request id")
	}

	deadline := time.Now().Add(maxWait)
	for added.Status != "downloaded" {
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

		var history []historyItem
		if err := r.client.Get(ctx, "/api/cloud/history", nil, &history); err != nil {
			return candidate, err
		}
		for _, item := range history {
			if item.RequestID == added.RequestID {
				added.Status = item.Status
				break
			}
		}
		if added.Status == "error" || added.Status == "canceled" {
			return candidate, errors.New("Offcloud cloud download failed")
		}
	}

	var explored exploreResponse
	if err := r.client.Get(ctx, "/api/cloud/explore/"+url.PathEscape(added.RequestID), url.Values{
		"format": {"detailed"},
	}, &explored); err != nil {
		return candidate, err
	}
	if explored.Error != "" {
		return candidate, errors.New(explored.Error)
	}

	files := make([]common.File, 0, len(explored.Files))
	for _, file := range explored.Files {
		name := file.Name
		if name == "" {
			name = filepath.Base(file.Path)
		}
		files = append(files, common.File{
			ID:   file.ID,
			Name: name,
			Path: file.Path,
			Size: file.Size,
			Link: file.URL,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("Offcloud returned no playable file")
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
