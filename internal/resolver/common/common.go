package common

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotReady = errors.New("resolver item is not ready")

type File struct {
	ID   string
	Name string
	Path string
	Size int64
	Link string
}

func Magnet(infoHash string) string {
	return "magnet:?xt=urn:btih:" + strings.ToLower(strings.TrimSpace(infoHash))
}

func SelectFile(files []File, index *int) (File, bool) {
	if index != nil {
		if *index < 0 || *index >= len(files) {
			return File{}, false
		}
		return files[*index], true
	}

	var best File
	found := false
	for _, file := range files {
		if file.Link == "" && file.ID == "" {
			continue
		}
		if !IsVideo(file.Name) && !IsVideo(file.Path) {
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
		if !found || file.Size > best.Size {
			best, found = file, true
		}
	}
	return best, found
}

func IsVideo(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".3g2", ".3gp", ".avi", ".f4v", ".flv", ".m2ts", ".m4v", ".mkv",
		".mov", ".mp4", ".mpeg", ".mpg", ".ogv", ".ts", ".webm", ".wmv":
		return true
	default:
		return false
	}
}

func JoinURL(baseURL, path string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

func Poll(ctxDone <-chan struct{}, interval time.Duration, fn func() (bool, error)) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctxDone:
			return errors.New("resolver polling cancelled")
		case <-timer.C:
		}
		done, err := fn()
		if err != nil || done {
			return err
		}
		timer.Reset(interval)
	}
}
