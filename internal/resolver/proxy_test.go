package resolver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"streamweave/internal/domain"
	"streamweave/internal/secretbox"
)

type fixedResolver struct{}

func (fixedResolver) ID() string { return "test" }
func (fixedResolver) Resolve(_ context.Context, _ domain.Candidate) (domain.Candidate, error) {
	return domain.Candidate{HTTP: &domain.HTTPStream{URL: "https://video.example/media", Headers: map[string]string{"Authorization": "Bearer secret"}}}, nil
}

func TestProxyResolverToken(t *testing.T) {
	box, err := secretbox.NewFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	r := proxyResolver{Resolver: fixedResolver{}, accountID: "account-1", box: box}
	candidate, err := r.Resolve(context.Background(), domain.Candidate{})
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "/api/v1/proxy/stream/"
	if len(candidate.HTTP.URL) < len(prefix) || candidate.HTTP.URL[:len(prefix)] != prefix {
		t.Fatal(candidate.HTTP.URL)
	}
	data, err := box.OpenURL(candidate.HTTP.URL[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}
	var token streamToken
	if err := json.Unmarshal(data, &token); err != nil {
		t.Fatal(err)
	}
	if token.URL != "https://video.example/media" || token.Headers["Authorization"] != "Bearer secret" || token.Expires <= time.Now().Unix() {
		t.Fatalf("unexpected token: %+v", token)
	}
	if len(candidate.HTTP.Headers) != 0 {
		t.Fatal("credentials escaped into protocol response")
	}
}

func TestProxyClientUsesProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "video.example" || r.Header.Get("Range") != "bytes=4-" {
			t.Errorf("unexpected proxy request: %s, %v", r.URL, r.Header)
		}
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer proxy.Close()
	client, err := proxyClient(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://video.example/file", nil)
	request.Header.Set("Range", "bytes=4-")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		t.Fatal(response.Status)
	}
}
