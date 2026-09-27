package alldebrid

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"streamweave/internal/domain"
)

var ErrDelayed = errors.New("AllDebrid link is not immediately available")

type Resolver struct {
	id     string
	client *Client
}

func NewResolver(id string, client *Client) *Resolver {
	return &Resolver{id: id, client: client}
}

func (r *Resolver) ID() string {
	return r.id
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}

	magnet, err := r.client.UploadMagnet(ctx, candidate.Torrent.InfoHash)
	if err != nil {
		return candidate, err
	}
	if !magnet.Ready {
		cached := false
		candidate.Cached = &cached
		return candidate, errors.New("AllDebrid magnet is not cached")
	}

	files, err := r.client.MagnetFiles(ctx, magnet.ID)
	if err != nil {
		return candidate, err
	}
	leaves := flatten(files)
	selected, ok := selectFile(leaves, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("AllDebrid magnet has no selectable file")
	}

	unlocked, err := r.client.Unlock(ctx, selected.Link)
	if err != nil {
		return candidate, err
	}
	if unlocked.Delayed != 0 && unlocked.Link == "" {
		return candidate, ErrDelayed
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: unlocked.Link}
	if unlocked.Filename != "" {
		candidate.Filename = unlocked.Filename
	} else {
		candidate.Filename = selected.Name
	}
	if unlocked.Filesize > 0 {
		candidate.SizeBytes = unlocked.Filesize
	} else {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func flatten(files []File) []File {
	result := make([]File, 0)
	var visit func([]File)
	visit = func(items []File) {
		for _, item := range items {
			if len(item.Entries) != 0 {
				visit(item.Entries)
				continue
			}
			result = append(result, item)
		}
	}
	visit(files)
	return result
}

func selectFile(files []File, index *int) (File, bool) {
	if index != nil {
		if *index < 0 || *index >= len(files) {
			return File{}, false
		}
		return files[*index], true
	}

	var best File
	found := false
	for _, file := range files {
		if file.Link == "" || !isVideo(file.Name) {
			continue
		}
		if !found || file.Size > best.Size {
			best, found = file, true
		}
	}
	if found {
		return best, true
	}

	for _, file := range files {
		if file.Link != "" && (!found || file.Size > best.Size) {
			best, found = file, true
		}
	}
	return best, found
}

func isVideo(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv", ".mp4", ".avi", ".mov", ".m4v", ".webm", ".ts", ".m2ts":
		return true
	default:
		return false
	}
}
