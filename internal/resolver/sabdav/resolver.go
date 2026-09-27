package sabdav

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"streamweave/internal/domain"
	"streamweave/internal/resolver/common"
	"streamweave/internal/resolver/credentialjson"
)

const (
	defaultPollInterval = 2 * time.Second
	defaultMaxWait      = 2 * time.Minute
)

type Config struct {
	Service        string
	BaseURL        string
	PublicURL      string
	APIURL         string
	APIKey         string
	WebDAVUser     string
	WebDAVPassword string
	ContentPrefix  string
	PollInterval   time.Duration
	MaxWait        time.Duration
	ExpectedFolder func(domain.Candidate) string
}

type Resolver struct {
	id     string
	config Config
	client *http.Client
}

type Credential struct {
	NzbDavURL         string `json:"nzbdavUrl"`
	PublicNzbDavURL   string `json:"publicNzbdavUrl"`
	NzbDavAPIKey      string `json:"nzbdavApiKey"`
	AltMountURL       string `json:"altmountUrl"`
	PublicAltMountURL string `json:"publicAltmountUrl"`
	AltMountAPIKey    string `json:"altmountApiKey"`
	WebDAVUser        string `json:"webdavUser"`
	WebDAVPassword    string `json:"webdavPassword"`
}

func ParseCredential(value string, target *Credential) error {
	return credentialjson.Decode(value, target)
}

func New(id string, cfg Config, client *http.Client) (*Resolver, error) {
	if client == nil {
		return nil, errors.New("http client is required")
	}
	if cfg.BaseURL == "" || cfg.APIURL == "" || cfg.APIKey == "" {
		return nil, errors.New("SAB/WebDAV resolver requires base URL, API URL and API key")
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = cfg.BaseURL
	}
	if cfg.ContentPrefix == "" {
		cfg.ContentPrefix = "/content"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = defaultMaxWait
	}
	return &Resolver{id: id, config: cfg, client: client}, nil
}

func (r *Resolver) ID() string { return r.id }

type addURLResponse struct {
	Status bool     `json:"status"`
	NZOIDs []string `json:"nzo_ids"`
	Error  string   `json:"error"`
}

type historyResponse struct {
	Status  bool   `json:"status"`
	Error   string `json:"error"`
	History struct {
		Slots []historySlot `json:"slots"`
	} `json:"history"`
}

type historySlot struct {
	NZOID       string `json:"nzo_id"`
	Status      string `json:"status"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Storage     string `json:"storage"`
	FailMessage string `json:"fail_message"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Usenet == nil || strings.TrimSpace(candidate.Usenet.NZBURL) == "" {
		return candidate, nil
	}

	category := "Movies"
	if strings.Contains(candidate.Media.ID, ":") {
		category = "TV"
	}
	folder := ""
	if r.config.ExpectedFolder != nil {
		folder = strings.TrimSpace(r.config.ExpectedFolder(candidate))
	}
	if folder == "" {
		folder = expectedFolder(candidate)
	}

	contentPath := path.Join(r.config.ContentPrefix, category, folder)
	exists, _ := r.webDAVExists(ctx, contentPath)
	if !exists {
		nzoID, err := r.addURL(ctx, candidate.Usenet.NZBURL, category, folder)
		if err != nil {
			return candidate, err
		}
		slot, err := r.waitHistory(ctx, nzoID, category)
		if err != nil {
			return candidate, err
		}
		jobName := folder
		if slot.Storage != "" {
			jobName = filepath.Base(slot.Storage)
		} else if slot.Name != "" {
			jobName = slot.Name
		}
		jobCategory := category
		if slot.Category != "" {
			jobCategory = slot.Category
		}
		contentPath = path.Join(r.config.ContentPrefix, jobCategory, jobName)
	}

	files, err := r.collectFiles(ctx, contentPath, 0)
	if err != nil {
		return candidate, err
	}
	selected, ok := common.SelectFile(files, candidate.Usenet.FileIndex)
	if !ok {
		return candidate, errors.New("SAB/WebDAV download has no playable file")
	}

	publicURL := joinPublic(r.config.PublicURL, selected.Path)
	headers := map[string]string{}
	if r.config.WebDAVUser != "" || r.config.WebDAVPassword != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(r.config.WebDAVUser+":"+r.config.WebDAVPassword))
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: publicURL, Headers: headers}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func expectedFolder(candidate domain.Candidate) string {
	if candidate.Filename != "" {
		return strings.TrimSuffix(filepath.Base(candidate.Filename), filepath.Ext(candidate.Filename))
	}
	if parsed, err := url.Parse(candidate.Usenet.NZBURL); err == nil {
		name := filepath.Base(parsed.Path)
		if name != "." && name != "/" && name != "" {
			return strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
	if candidate.Title != "" {
		return candidate.Title
	}
	return candidate.Usenet.Hash
}

func (r *Resolver) addURL(ctx context.Context, nzbURL, category, folder string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(r.config.APIURL, "/"))
	if err != nil {
		return "", err
	}
	q := endpoint.Query()
	q.Set("mode", "addurl")
	q.Set("apikey", r.config.APIKey)
	q.Set("name", nzbURL)
	q.Set("cat", category)
	q.Set("nzbname", folder)
	q.Set("output", "json")
	endpoint.RawQuery = q.Encode()

	var response addURLResponse
	if err := r.getJSON(ctx, endpoint.String(), &response); err != nil {
		return "", err
	}
	if !response.Status {
		return "", errors.New(firstNonEmpty(response.Error, "SAB addurl failed"))
	}
	if len(response.NZOIDs) == 0 || response.NZOIDs[0] == "" {
		return "", errors.New("SAB addurl returned no nzo id")
	}
	return response.NZOIDs[0], nil
}

func (r *Resolver) waitHistory(ctx context.Context, nzoID, category string) (historySlot, error) {
	deadline := time.Now().Add(r.config.MaxWait)
	for {
		endpoint, err := url.Parse(strings.TrimRight(r.config.APIURL, "/"))
		if err != nil {
			return historySlot{}, err
		}
		q := endpoint.Query()
		q.Set("mode", "history")
		q.Set("apikey", r.config.APIKey)
		q.Set("nzo_ids", nzoID)
		q.Set("category", category)
		q.Set("output", "json")
		endpoint.RawQuery = q.Encode()

		var response historyResponse
		if err := r.getJSON(ctx, endpoint.String(), &response); err != nil {
			return historySlot{}, err
		}
		if !response.Status && response.Error != "" {
			return historySlot{}, errors.New(response.Error)
		}
		for _, slot := range response.History.Slots {
			if slot.NZOID != nzoID {
				continue
			}
			switch strings.ToLower(slot.Status) {
			case "completed":
				return slot, nil
			case "failed":
				return historySlot{}, errors.New(firstNonEmpty(slot.FailMessage, "SAB download failed"))
			}
		}
		if time.Now().After(deadline) {
			return historySlot{}, common.ErrNotReady
		}
		select {
		case <-ctx.Done():
			return historySlot{}, ctx.Err()
		case <-time.After(r.config.PollInterval):
		}
	}
}

func (r *Resolver) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", r.config.APIKey)
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s API returned HTTP %d", r.config.Service, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(target)
}

type multistatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop davProp `xml:"prop"`
}

type davProp struct {
	DisplayName   string       `xml:"displayname"`
	ContentLength string       `xml:"getcontentlength"`
	ResourceType  resourceType `xml:"resourcetype"`
}

type resourceType struct {
	Collection *struct{} `xml:"collection"`
}

func (r *Resolver) webDAVExists(ctx context.Context, resourcePath string) (bool, error) {
	req, err := r.newPROPFIND(ctx, resourcePath, "0")
	if err != nil {
		return false, err
	}
	res, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return false, fmt.Errorf("WebDAV stat returned HTTP %d", res.StatusCode)
	}
	return true, nil
}

func (r *Resolver) collectFiles(ctx context.Context, resourcePath string, depth int) ([]common.File, error) {
	if depth > 6 {
		return nil, nil
	}
	req, err := r.newPROPFIND(ctx, resourcePath, "1")
	if err != nil {
		return nil, err
	}
	res, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("WebDAV list returned HTTP %d", res.StatusCode)
	}
	var status multistatus
	if err := xml.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&status); err != nil {
		return nil, err
	}

	baseEscaped := ensureLeadingSlash(resourcePath)
	files := make([]common.File, 0)
	for _, item := range status.Responses {
		href, _ := url.PathUnescape(item.Href)
		if strings.TrimSuffix(href, "/") == strings.TrimSuffix(baseEscaped, "/") {
			continue
		}
		prop := davProp{}
		for _, ps := range item.Propstat {
			if ps.Prop.DisplayName != "" || ps.Prop.ContentLength != "" || ps.Prop.ResourceType.Collection != nil {
				prop = ps.Prop
				break
			}
		}
		itemPath := href
		if parsed, err := url.Parse(href); err == nil && parsed.Path != "" {
			itemPath = parsed.Path
		}
		if prop.ResourceType.Collection != nil {
			children, childErr := r.collectFiles(ctx, itemPath, depth+1)
			if childErr != nil {
				return nil, childErr
			}
			files = append(files, children...)
			continue
		}
		name := prop.DisplayName
		if name == "" {
			name = filepath.Base(itemPath)
		}
		size, _ := strconv.ParseInt(prop.ContentLength, 10, 64)
		files = append(files, common.File{Name: name, Path: itemPath, Size: size, Link: itemPath})
	}
	return files, nil
}

func (r *Resolver) newPROPFIND(ctx context.Context, resourcePath, depth string) (*http.Request, error) {
	endpoint := joinPublic(r.config.BaseURL, resourcePath)
	body := "<?xml version=\"1.0\"?><propfind xmlns=\"DAV:\"><prop><displayname/><getcontentlength/><resourcetype/></prop></propfind>"
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", depth)
	req.Header.Set("Content-Type", "application/xml")
	if r.config.WebDAVUser != "" || r.config.WebDAVPassword != "" {
		req.SetBasicAuth(r.config.WebDAVUser, r.config.WebDAVPassword)
	}
	return req, nil
}

func joinPublic(base, resourcePath string) string {
	base = strings.TrimRight(base, "/")
	parts := strings.Split(strings.TrimLeft(resourcePath, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return base + "/" + strings.Join(parts, "/")
}

func ensureLeadingSlash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "upstream error"
}
