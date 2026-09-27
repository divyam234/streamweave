package nab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

func TestTorznabSearchBySeriesID(t *testing.T) {
	hash := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query()
		if query.Get("t") != "tvsearch" || query.Get("imdbid") != "tt1234567" || query.Get("season") != "2" || query.Get("ep") != "3" {
			t.Fatalf("unexpected query: %s", req.URL.RawQuery)
		}
		if query.Get("apikey") != "secret" {
			t.Fatalf("apikey = %q", query.Get("apikey"))
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>Show.S02E03.1080p.HEVC</title><guid>magnet:?xt=urn:btih:` + hash + `</guid><size>1234</size><torznab:attr name="seeders" value="42"/><torznab:attr name="language" value="eng,spa"/></item></channel></rss>`))
	}))
	defer server.Close()

	provider, err := New("t1", Torznab, server.URL, "secret", server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "series", ID: "tt1234567:2:3"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].Torrent == nil || results[0].Torrent.InfoHash != hash {
		t.Fatalf("torrent = %#v", results[0].Torrent)
	}
	if results[0].Seeders == nil || *results[0].Seeders != 42 {
		t.Fatalf("seeders = %#v", results[0].Seeders)
	}
	if len(results[0].Languages) != 2 {
		t.Fatalf("languages = %#v", results[0].Languages)
	}
}

func TestNewznabSearchProducesUsenetCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("t") != "movie" || req.URL.Query().Get("imdbid") != "tt7654321" {
			t.Fatalf("unexpected query: %s", req.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/"><channel><item><title>Movie.2160p</title><enclosure url="https://indexer.example/get/abc.nzb" type="application/x-nzb" length="9876"/><newznab:attr name="language" value="eng"/></item></channel></rss>`))
	}))
	defer server.Close()

	provider, err := New("n1", Newznab, server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	results, err := provider.Search(context.Background(), engine.SearchRequest{
		Media: domain.MediaRef{Type: "movie", ID: "tt7654321"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Usenet == nil {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Usenet.NZBURL != "https://indexer.example/get/abc.nzb" {
		t.Fatalf("nzb = %q", results[0].Usenet.NZBURL)
	}
	if results[0].SizeBytes != 9876 {
		t.Fatalf("size = %d", results[0].SizeBytes)
	}
}
