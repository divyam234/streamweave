package torbox

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/resolver/common"
	"streamweave/internal/resolver/httpclient"
)

const (
	DefaultBaseURL = "https://api.torbox.app"
)

type Resolver struct {
	id     string
	apiKey string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("TorBox API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, apiKey: apiKey, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type envelope[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Detail  string `json:"detail"`
	Error   string `json:"error"`
}

type createTorrentData struct {
	TorrentID int    `json:"torrent_id"`
	Hash      string `json:"hash"`
}

type torrentFile struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Size      int64  `json:"size"`
}

type torrent struct {
	ID               int           `json:"id"`
	Hash             string        `json:"hash"`
	Name             string        `json:"name"`
	DownloadPresent  bool          `json:"download_present"`
	DownloadFinished bool          `json:"download_finished"`
	DownloadState    string        `json:"download_state"`
	Files            []torrentFile `json:"files"`
}

type createUsenetData struct {
	UsenetDownloadID int    `json:"usenetdownload_id"`
	Hash             string `json:"hash"`
}

type usenetDownload struct {
	ID               int           `json:"id"`
	Hash             string        `json:"hash"`
	Name             string        `json:"name"`
	DownloadPresent  bool          `json:"download_present"`
	DownloadFinished bool          `json:"download_finished"`
	DownloadState    string        `json:"download_state"`
	Files            []torrentFile `json:"files"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Usenet != nil && candidate.Usenet.NZBURL != "" {
		return r.resolveUsenet(ctx, candidate)
	}
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	var created envelope[createTorrentData]
	if err := r.client.Form(ctx, http.MethodPost, "/v1/api/torrents/createtorrent", nil, url.Values{
		"magnet":    {common.Magnet(candidate.Torrent.InfoHash)},
		"allow_zip": {"false"},
	}, &created); err != nil {
		return candidate, err
	}
	if !created.Success || created.Data.TorrentID == 0 {
		return candidate, errors.New(nonEmpty(created.Detail, created.Error, "TorBox failed to create torrent"))
	}

	var item torrent
	{
		var response envelope[torrent]
		query := url.Values{
			"id":           {strconv.Itoa(created.Data.TorrentID)},
			"bypass_cache": {"true"},
		}
		if err := r.client.Get(ctx, "/v1/api/torrents/mylist", query, &response); err != nil {
			return candidate, err
		}
		if !response.Success {
			return candidate, errors.New(nonEmpty(response.Detail, response.Error, "TorBox failed to read torrent"))
		}
		item = response.Data
		if !(item.DownloadPresent || item.DownloadFinished || item.DownloadState == "cached" || item.DownloadState == "completed") {
			return candidate, common.ErrNotReady
		}
	}

	files := make([]common.File, 0, len(item.Files))
	for _, file := range item.Files {
		name := file.ShortName
		if name == "" {
			name = file.Name
		}
		files = append(files, common.File{
			ID:   strconv.Itoa(file.ID),
			Name: name,
			Path: file.Name,
			Size: file.Size,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok {
		return candidate, errors.New("TorBox torrent has no selectable file")
	}

	var link envelope[string]
	query := url.Values{
		"token":      {r.apiKey},
		"torrent_id": {strconv.Itoa(created.Data.TorrentID)},
		"file_id":    {selected.ID},
		"zip_link":   {"false"},
	}
	if err := r.client.Get(ctx, "/v1/api/torrents/requestdl", query, &link); err != nil {
		return candidate, err
	}
	if !link.Success || strings.TrimSpace(link.Data) == "" {
		return candidate, errors.New(nonEmpty(link.Detail, link.Error, "TorBox returned an empty playback URL"))
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: link.Data}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func (r *Resolver) resolveUsenet(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	var created envelope[createUsenetData]
	if err := r.client.Form(ctx, http.MethodPost, "/v1/api/usenet/createusenetdownload", nil, url.Values{
		"link":      {candidate.Usenet.NZBURL},
		"as_queued": {"false"},
	}, &created); err != nil {
		return candidate, err
	}
	if !created.Success || created.Data.UsenetDownloadID == 0 {
		return candidate, errors.New(nonEmpty(created.Detail, created.Error, "TorBox failed to create Usenet download"))
	}

	var item usenetDownload
	{
		var response envelope[usenetDownload]
		query := url.Values{
			"id":           {strconv.Itoa(created.Data.UsenetDownloadID)},
			"bypass_cache": {"true"},
		}
		if err := r.client.Get(ctx, "/v1/api/usenet/mylist", query, &response); err != nil {
			return candidate, err
		}
		if !response.Success {
			return candidate, errors.New(nonEmpty(response.Detail, response.Error, "TorBox failed to read Usenet download"))
		}
		item = response.Data
		if !(item.DownloadPresent || item.DownloadFinished || item.DownloadState == "cached" || item.DownloadState == "completed") {
			return candidate, common.ErrNotReady
		}
	}

	files := make([]common.File, 0, len(item.Files))
	for _, file := range item.Files {
		name := file.ShortName
		if name == "" {
			name = file.Name
		}
		files = append(files, common.File{
			ID:   strconv.Itoa(file.ID),
			Name: name,
			Path: file.Name,
			Size: file.Size,
		})
	}
	selected, ok := common.SelectFile(files, nil)
	if !ok {
		return candidate, errors.New("TorBox Usenet download has no selectable file")
	}

	var link envelope[string]
	query := url.Values{
		"token":     {r.apiKey},
		"usenet_id": {strconv.Itoa(created.Data.UsenetDownloadID)},
		"file_id":   {selected.ID},
		"zip_link":  {"false"},
	}
	if err := r.client.Get(ctx, "/v1/api/usenet/requestdl", query, &link); err != nil {
		return candidate, err
	}
	if !link.Success || strings.TrimSpace(link.Data) == "" {
		return candidate, errors.New(nonEmpty(link.Detail, link.Error, "TorBox returned an empty Usenet playback URL"))
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: link.Data}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "upstream error"
}
