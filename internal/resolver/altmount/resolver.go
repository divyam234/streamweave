package altmount

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/common"
	"media-engine/internal/resolver/sabdav"
)

type Resolver struct {
	id        string
	baseURL   string
	publicURL string
	apiKey    string
	client    *http.Client
	fallback  *sabdav.Resolver
}

type streamsResponse struct {
	Streams []stream `json:"streams"`
	Cached  bool     `json:"_cached"`
	Status  string   `json:"_queue_status"`
}

type stream struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Name  string `json:"name"`
}

func NewResolver(id, credential string, client *http.Client) (*Resolver, error) {
	var cfg sabdav.Credential
	if err := sabdav.ParseCredential(credential, &cfg); err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.AltMountURL), "/")
	if baseURL == "" || strings.TrimSpace(cfg.AltMountAPIKey) == "" {
		return nil, errors.New("AltMount credential requires altmountUrl and altmountApiKey")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(cfg.PublicAltMountURL), "/")
	if publicURL == "" {
		publicURL = baseURL
	}
	fallback, err := sabdav.New(id, sabdav.Config{
		Service:        "AltMount",
		BaseURL:        baseURL + "/webdav",
		PublicURL:      publicURL + "/webdav",
		APIURL:         baseURL + "/sabnzbd/api",
		APIKey:         cfg.AltMountAPIKey,
		WebDAVUser:     cfg.WebDAVUser,
		WebDAVPassword: cfg.WebDAVPassword,
		ContentPrefix:  "/complete",
	}, client)
	if err != nil {
		return nil, err
	}
	return &Resolver{
		id: id, baseURL: baseURL, publicURL: publicURL,
		apiKey: cfg.AltMountAPIKey, client: client, fallback: fallback,
	}, nil
}

func (r *Resolver) ID() string { return r.id }

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Usenet == nil || strings.TrimSpace(candidate.Usenet.NZBURL) == "" {
		return candidate, nil
	}

	form := url.Values{"nzb_url": {candidate.Usenet.NZBURL}}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		r.baseURL+"/api/nzb/streams",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return candidate, err
	}
	req.Header.Set("X-Api-Key", r.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := r.client.Do(req)
	if err != nil {
		return candidate, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusMethodNotAllowed {
		return r.fallback.Resolve(ctx, candidate)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return candidate, errors.New("AltMount native streams request failed")
	}

	var payload streamsResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&payload); err != nil {
		return candidate, err
	}
	files := make([]common.File, 0, len(payload.Streams))
	for index, item := range payload.Streams {
		link := strings.TrimSpace(item.URL)
		if r.publicURL != r.baseURL {
			link = strings.Replace(link, r.baseURL, r.publicURL, 1)
		}
		name := item.Name
		if name == "" {
			name = item.Title
		}
		files = append(files, common.File{
			ID:   strconv.Itoa(index),
			Name: name,
			Path: name,
			Link: link,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Usenet.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("AltMount returned no playable stream")
	}

	cached := payload.Cached
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: selected.Link}
	candidate.Filename = selected.Name
	return candidate, nil
}
