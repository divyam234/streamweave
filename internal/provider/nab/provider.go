package nab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
)

type Kind string

const (
	Torznab Kind = "torznab"
	Newznab Kind = "newznab"
)

const maxResponseBytes = 12 << 20

var infoHashPattern = regexp.MustCompile(`(?i)(?:urn(?::|%3A)btih(?::|%3A))([a-f0-9]{40})`)

type Provider struct {
	id       string
	kind     Kind
	endpoint string
	apiKey   string
	client   *http.Client
}

func New(id string, kind Kind, endpoint, apiKey string, client *http.Client) (*Provider, error) {
	if kind != Torznab && kind != Newznab {
		return nil, errors.New("unsupported nab kind")
	}
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("parse nab endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("nab endpoint must use http or https")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("nab endpoint must include a host")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	return &Provider{
		id:       id,
		kind:     kind,
		endpoint: parsed.String(),
		apiKey:   strings.TrimSpace(apiKey),
		client:   client,
	}, nil
}

func (p *Provider) ID() string { return p.id }

type rss struct {
	Channel channel `xml:"channel"`
}

type channel struct {
	Items []item `xml:"item"`
}

type item struct {
	Title      string      `xml:"title"`
	GUID       string      `xml:"guid"`
	PubDate    string      `xml:"pubDate"`
	Size       int64       `xml:"size"`
	Type       string      `xml:"type"`
	Enclosures []enclosure `xml:"enclosure"`
	Attrs      []attribute `xml:"attr"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int64  `xml:"length,attr"`
}

type attribute struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func (p *Provider) Search(ctx context.Context, req engine.SearchRequest) ([]domain.Candidate, error) {
	query, err := searchQuery(req.Media)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		query.Set("apikey", p.apiKey)
	}
	query.Set("extended", "1")

	endpoint, err := url.Parse(p.endpoint)
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	for key, entries := range query {
		for _, entry := range entries {
			values.Set(key, entry)
		}
	}
	endpoint.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", p.kind, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned HTTP %d", p.kind, response.StatusCode)
	}

	var feed rss
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", p.kind, err)
	}

	results := make([]domain.Candidate, 0, len(feed.Channel.Items))
	for _, entry := range feed.Channel.Items {
		var candidate domain.Candidate
		var ok bool
		if p.kind == Torznab {
			candidate, ok = p.torrentCandidate(req.Media, entry)
		} else {
			candidate, ok = p.usenetCandidate(req.Media, entry)
		}
		if ok {
			results = append(results, candidate)
		}
	}
	return results, nil
}

func searchQuery(media domain.MediaRef) (url.Values, error) {
	id, season, episode := parseMediaID(media.ID)
	if id == "" {
		return nil, errors.New("media id is required")
	}

	query := make(url.Values)
	switch media.Type {
	case "movie":
		query.Set("t", "movie")
		query.Set("imdbid", id)
	case "series":
		query.Set("t", "tvsearch")
		query.Set("imdbid", id)
		if season != "" {
			query.Set("season", season)
		}
		if episode != "" {
			query.Set("ep", episode)
		}
	default:
		query.Set("t", "search")
		query.Set("q", id)
	}
	return query, nil
}

func parseMediaID(raw string) (id, season, episode string) {
	parts := strings.Split(raw, ":")
	if len(parts) > 0 {
		id = parts[0]
	}
	if len(parts) > 1 {
		season = parts[1]
	}
	if len(parts) > 2 {
		episode = parts[2]
	}
	return
}

func (p *Provider) torrentCandidate(media domain.MediaRef, entry item) (domain.Candidate, bool) {
	attrs := attributeMap(entry.Attrs)
	hash := normalizeInfoHash(attrs["infohash"])
	if hash == "" {
		hash = extractInfoHash(attrs["magneturl"])
	}
	for _, enclosed := range entry.Enclosures {
		if hash == "" && strings.Contains(strings.ToLower(enclosed.URL), "magnet:") {
			hash = extractInfoHash(enclosed.URL)
		}
	}
	if hash == "" {
		hash = extractInfoHash(entry.GUID)
	}
	if hash == "" {
		return domain.Candidate{}, false
	}

	size := entry.Size
	if size <= 0 {
		size, _ = strconv.ParseInt(attrs["size"], 10, 64)
	}
	seeders, hasSeeders := parseInt(attrs["seeders"])
	candidate := domain.Candidate{
		ID:        candidateID(p.id, entry.Title, hash),
		SourceID:  p.id,
		Kind:      domain.CandidateTorrent,
		Media:     media,
		Title:     strings.TrimSpace(entry.Title),
		SizeBytes: size,
		Torrent:   &domain.TorrentInfo{InfoHash: hash},
	}
	if hasSeeders && seeders >= 0 && seeders != 999 {
		candidate.Seeders = &seeders
	}
	if language := strings.TrimSpace(attrs["language"]); language != "" {
		candidate.Languages = splitLanguages(language)
	}
	return candidate, true
}

func (p *Provider) usenetCandidate(media domain.MediaRef, entry item) (domain.Candidate, bool) {
	var nzbURL string
	var size int64
	for _, enclosed := range entry.Enclosures {
		if strings.Contains(strings.ToLower(enclosed.Type), "nzb") || strings.HasSuffix(strings.ToLower(enclosed.URL), ".nzb") {
			nzbURL = strings.TrimSpace(enclosed.URL)
			size = enclosed.Length
			break
		}
	}
	if nzbURL == "" && strings.HasPrefix(entry.GUID, "http") {
		nzbURL = entry.GUID
	}
	if nzbURL == "" {
		return domain.Candidate{}, false
	}
	if entry.Size > 0 {
		size = entry.Size
	}
	attrs := attributeMap(entry.Attrs)
	if size <= 0 {
		size, _ = strconv.ParseInt(attrs["size"], 10, 64)
	}
	hash := urlHash(nzbURL)
	candidate := domain.Candidate{
		ID:        candidateID(p.id, entry.Title, hash),
		SourceID:  p.id,
		Kind:      domain.CandidateUsenet,
		Media:     media,
		Title:     strings.TrimSpace(entry.Title),
		SizeBytes: size,
		Usenet:    &domain.UsenetInfo{NZBURL: nzbURL, Hash: hash},
	}
	if language := strings.TrimSpace(attrs["language"]); language != "" {
		candidate.Languages = splitLanguages(language)
	}
	return candidate, true
}

func attributeMap(attrs []attribute) map[string]string {
	result := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		if attr.Name != "" {
			result[strings.ToLower(attr.Name)] = attr.Value
		}
	}
	return result
}

func normalizeInfoHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 40 {
		if _, err := hex.DecodeString(value); err == nil {
			return strings.ToLower(value)
		}
	}
	return extractInfoHash(value)
}

func extractInfoHash(value string) string {
	match := infoHashPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(match[1])
}

func parseInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil
}

func splitLanguages(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if normalized := strings.TrimSpace(field); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func urlHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func candidateID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:12])
}
