package realdebrid

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
	DefaultBaseURL = "https://api.real-debrid.com/rest/1.0"
)

type Resolver struct {
	id     string
	client *httpclient.Client
}

func NewResolver(id, apiKey, baseURL string, client *http.Client) (*Resolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Real-Debrid API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := httpclient.New(baseURL, client)
	httpClient.Headers.Set("Authorization", "Bearer "+apiKey)
	return &Resolver{id: id, client: httpClient}, nil
}

func (r *Resolver) ID() string { return r.id }

type addMagnetResponse struct {
	ID  string `json:"id"`
	URI string `json:"uri"`
}

type torrentFile struct {
	ID       int    `json:"id"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Selected int    `json:"selected"`
}

type torrentInfo struct {
	ID       string        `json:"id"`
	Filename string        `json:"filename"`
	Hash     string        `json:"hash"`
	Status   string        `json:"status"`
	Files    []torrentFile `json:"files"`
	Links    []string      `json:"links"`
}

type unrestrictResponse struct {
	Filename string `json:"filename"`
	Filesize int64  `json:"filesize"`
	Download string `json:"download"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}
	magnet := common.Magnet(candidate.Torrent.InfoHash)

	var added addMagnetResponse
	if err := r.client.Form(ctx, http.MethodPost, "/torrents/addMagnet", nil, url.Values{"magnet": {magnet}}, &added); err != nil {
		return candidate, err
	}
	if added.ID == "" {
		return candidate, errors.New("Real-Debrid returned an empty torrent id")
	}

	info, err := r.getInfo(ctx, added.ID)
	if err != nil {
		return candidate, err
	}
	files := make([]common.File, 0, len(info.Files))
	for _, file := range info.Files {
		files = append(files, common.File{
			ID:   strconv.Itoa(file.ID),
			Name: strings.TrimPrefix(file.Path, "/"),
			Path: file.Path,
			Size: file.Bytes,
		})
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok {
		return candidate, errors.New("Real-Debrid torrent has no selectable file")
	}

	if !isFileSelected(info.Files, selected.ID) {
		if err := r.client.Form(ctx, http.MethodPost, "/torrents/selectFiles/"+added.ID, nil, url.Values{"files": {selected.ID}}, nil); err != nil {
			return candidate, err
		}
	}

	info, err = r.getInfo(ctx, added.ID)
	if err != nil {
		return candidate, err
	}
	if len(info.Links) == 0 {
		return candidate, common.ErrNotReady
	}

	link := info.Links[0]
	if len(info.Links) > 1 {
		index := selectedLinkIndex(info.Files, selected.ID)
		if index >= 0 && index < len(info.Links) {
			link = info.Links[index]
		}
	}

	var unrestricted unrestrictResponse
	if err := r.client.Form(ctx, http.MethodPost, "/unrestrict/link", nil, url.Values{"link": {link}}, &unrestricted); err != nil {
		return candidate, err
	}
	if unrestricted.Download == "" {
		return candidate, errors.New("Real-Debrid returned an empty download URL")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: unrestricted.Download}
	if unrestricted.Filename != "" {
		candidate.Filename = unrestricted.Filename
	} else if selected.Name != "" {
		candidate.Filename = selected.Name
	}
	if unrestricted.Filesize > 0 {
		candidate.SizeBytes = unrestricted.Filesize
	} else if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func (r *Resolver) getInfo(ctx context.Context, id string) (torrentInfo, error) {
	var info torrentInfo
	err := r.client.Get(ctx, "/torrents/info/"+id, nil, &info)
	return info, err
}

func isFileSelected(files []torrentFile, id string) bool {
	for _, file := range files {
		if strconv.Itoa(file.ID) == id {
			return file.Selected == 1
		}
	}
	return false
}

func selectedLinkIndex(files []torrentFile, selectedID string) int {
	index := 0
	for _, file := range files {
		if file.Selected != 1 {
			continue
		}
		if strconv.Itoa(file.ID) == selectedID {
			return index
		}
		index++
	}
	return -1
}
