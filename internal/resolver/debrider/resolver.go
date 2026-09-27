package debrider

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

const (
	DefaultBaseURL = "https://debrider.app/api"
)

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Debrider API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type taskFile struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	DownloadLink string `json:"download_link,omitempty"`
}

type task struct {
	ID       string     `json:"id"`
	Hash     string     `json:"hash"`
	Name     string     `json:"name"`
	Size     int64      `json:"size"`
	Files    []taskFile `json:"files"`
	Progress float64    `json:"progress"`
	Status   string     `json:"status"`
	Type     string     `json:"type"`
}

type taskEnvelope struct {
	Message string `json:"message,omitempty"`
	Data    task   `json:"data"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var created taskEnvelope
	if err := r.client.JSON(ctx, http.MethodPost, "/v1/tasks", nil, map[string]any{
		"type": "magnet",
		"data": common.Magnet(candidate.Torrent.InfoHash),
	}, &created); err != nil {
		return candidate, err
	}
	if created.Data.ID == "" {
		return candidate, errors.New(firstNonEmpty(created.Message, "Debrider returned an empty task id"))
	}

	item := created.Data
	if item.Status == "error" {
		return candidate, errors.New(firstNonEmpty(created.Message, "Debrider task failed"))
	}
	if item.Status != "completed" {
		return candidate, common.ErrNotReady
	}

	files := make([]common.File, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, common.File{
			Name: filepath.Base(file.Name),
			Path: file.Name,
			Size: file.Size,
			Link: file.DownloadLink,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("Debrider returned no playable file")
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "upstream error"
}
