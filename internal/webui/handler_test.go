package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesEmbeddedSPA(t *testing.T) {
	if !embedded {
		t.Skip("embedded UI build tag not enabled")
	}

	handler := Handler()

	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusOK {
		t.Fatalf("root status = %d", root.Code)
	}
	if !strings.Contains(root.Body.String(), "<div id=\"root\">") {
		t.Fatalf("root did not serve Vite index")
	}
	if got := root.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("root cache-control = %q", got)
	}

	spa := httptest.NewRecorder()
	handler.ServeHTTP(spa, httptest.NewRequest(http.MethodGet, "/providers", nil))
	if spa.Code != http.StatusOK || !strings.Contains(spa.Body.String(), "<div id=\"root\">") {
		t.Fatalf("SPA fallback status/body invalid: %d", spa.Code)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", missing.Code)
	}

	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		t.Fatalf("sub dist: %v", err)
	}
	entries, err := fs.ReadDir(dist, "assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("read assets: %v entries=%d", err, len(entries))
	}
	asset := "/assets/" + entries[0].Name()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, asset, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("asset %s status = %d", asset, rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache-control = %q", got)
	}
}
