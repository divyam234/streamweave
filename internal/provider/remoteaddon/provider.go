package remoteaddon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
	"streamweave/internal/safehttp"
)

const maxResponseBytes = 4 << 20

type Provider struct {
	id       string
	name     string
	endpoint string
	client   *http.Client
}

type streamResponse struct {
	Streams []stream
}

type stream struct {
	Name     string
	Title    string
	URL      string
	InfoHash string
	FileIdx  *int
}

func New(id, name, endpoint string, client *http.Client, allowPrivate bool) (*Provider, error) {
	normalized, err := ValidateEndpoint(endpoint, allowPrivate)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{id: id, name: strings.TrimSpace(name), endpoint: normalized, client: client}, nil
}

func ValidateEndpoint(raw string, allowPrivate bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse provider endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("provider endpoint must use http or https")
	}
	if !allowPrivate && parsed.Scheme != "https" {
		return "", errors.New("public provider endpoints must use https")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("provider endpoint must include a host")
	}
	if parsed.User != nil {
		return "", errors.New("provider endpoint must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("provider endpoint must not include query parameters or fragments")
	}
	if !allowPrivate && isPrivateHost(parsed.Hostname()) {
		return "", errors.New("private provider endpoints are disabled")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/manifest.json") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/manifest.json")
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}
	return parsed.String(), nil
}

func isPrivateHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func (p *Provider) ID() string {
	return p.id
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	endpoint := fmt.Sprintf(
		"%s/stream/%s/%s.json",
		p.endpoint,
		escapePathSegment(req.Media.Type),
		escapeMediaID(req.Media.ID),
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create addon request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "StreamWeave/0.2.0")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request addon: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("addon returned status %d", response.StatusCode)
	}

	var payload streamResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode addon response: %w", err)
	}

	candidates := make([]domain.Candidate, 0, len(payload.Streams))
	for _, item := range payload.Streams {
		candidate, ok := p.toCandidate(req.Media, item)
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func (p *Provider) toCandidate(media domain.MediaRef, item stream) (domain.Candidate, bool) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = strings.TrimSpace(item.Name)
	}

	candidate := domain.Candidate{
		ID:           candidateID(p.id, item),
		SourceID:     p.id,
		SourceName:   p.name,
		UpstreamName: strings.TrimSpace(item.Name),
		Media:        media,
		Title:        title,
	}

	switch {
	case item.InfoHash != "":
		infoHash := strings.ToLower(strings.TrimSpace(item.InfoHash))
		if len(infoHash) != 40 {
			return domain.Candidate{}, false
		}
		if _, err := hex.DecodeString(infoHash); err != nil {
			return domain.Candidate{}, false
		}
		if item.FileIdx != nil && *item.FileIdx < 0 {
			return domain.Candidate{}, false
		}
		candidate.Kind = domain.CandidateTorrent
		candidate.Torrent = &domain.TorrentInfo{
			InfoHash:  infoHash,
			FileIndex: item.FileIdx,
		}
	case item.URL != "":
		if err := safehttp.ValidatePublicURL(item.URL); err != nil {
			return domain.Candidate{}, false
		}
		candidate.Kind = domain.CandidateDirect
		candidate.HTTP = &domain.HTTPStream{URL: item.URL}
	default:
		return domain.Candidate{}, false
	}

	return candidate, true
}

func candidateID(providerID string, item stream) string {
	sum := sha256.Sum256([]byte(providerID + "\x00" + item.InfoHash + "\x00" + item.URL + "\x00" + item.Title + "\x00" + item.Name))
	return hex.EncodeToString(sum[:12])
}

// escapePathSegment escapes a single URL path segment.
func escapePathSegment(value string) string {
	return url.PathEscape(value)
}

// escapeMediaID escapes a Stremio media ID while preserving the colons
// that separate a series ID from its season and episode
// (e.g. tt1234567:2:3). Upstream addons expect the colons literally;
// escaping them to %3A makes series lookups return nothing.
func escapeMediaID(id string) string {
	parts := strings.Split(id, ":")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, ":")
}
