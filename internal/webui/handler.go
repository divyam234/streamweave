package webui

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

var zeroTime time.Time

func Handler() http.Handler {
	if !embedded || assets == nil {
		return http.NotFoundHandler()
	}

	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		body, contentType, ok := readAsset(dist, name)
		if !ok && !strings.Contains(path.Base(name), ".") {
			body, contentType, ok = readAsset(dist, "index.html")
		}
		if !ok {
			http.NotFound(w, r)
			return
		}

		if strings.HasPrefix(name, "assets/") && ok {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, name, zeroTime, bytes.NewReader(body))
	})
}

func readAsset(dist fs.FS, name string) ([]byte, string, bool) {
	body, err := fs.ReadFile(dist, name)
	if err != nil {
		return nil, "", false
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return body, contentType, true
}
