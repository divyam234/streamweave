package alldebrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultBaseURL   = "https://api.alldebrid.com"
	maxResponseBytes = 4 << 20
)

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type Magnet struct {
	ID    int64
	Hash  string
	Name  string
	Size  int64
	Ready bool
}

type File struct {
	Name    string
	Size    int64
	Link    string
	Entries []File
}

type UnlockResult struct {
	Link     string
	Filename string
	Filesize int64
	Delayed  int64
}

func NewClient(apiKey string, client *http.Client) (*Client, error) {
	return NewClientWithBaseURL(apiKey, DefaultBaseURL, client)
}

func NewClientWithBaseURL(apiKey, baseURL string, client *http.Client) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("AllDebrid API key is required")
	}
	if client == nil {
		return nil, errors.New("HTTP client is required")
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid AllDebrid base URL")
	}
	return &Client{apiKey: strings.TrimSpace(apiKey), baseURL: parsed.String(), http: client}, nil
}

func (c *Client) UploadMagnet(ctx context.Context, hash string) (Magnet, error) {
	values := url.Values{}
	values.Add("magnets[]", strings.TrimSpace(hash))

	var response struct {
		Data struct {
			Magnets []struct {
				Hash  string
				Name  string
				Size  int64
				Ready bool
				ID    int64
				Error *apiErrorPayload
			}
		}
		Error *apiErrorPayload
	}

	if err := c.postForm(ctx, "/v4/magnet/upload", values, &response); err != nil {
		return Magnet{}, err
	}
	if response.Error != nil {
		return Magnet{}, response.Error.asError()
	}
	if len(response.Data.Magnets) != 1 {
		return Magnet{}, errors.New("AllDebrid returned no magnet result")
	}
	item := response.Data.Magnets[0]
	if item.Error != nil {
		return Magnet{}, item.Error.asError()
	}
	return Magnet{ID: item.ID, Hash: item.Hash, Name: item.Name, Size: item.Size, Ready: item.Ready}, nil
}

func (c *Client) MagnetFiles(ctx context.Context, id int64) ([]File, error) {
	values := url.Values{}
	values.Add("id[]", strconv.FormatInt(id, 10))

	var response struct {
		Data struct {
			Magnets []struct {
				Files []fileJSON
				Error *apiErrorPayload
			}
		}
		Error *apiErrorPayload
	}

	if err := c.postForm(ctx, "/v4/magnet/files", values, &response); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, response.Error.asError()
	}
	if len(response.Data.Magnets) != 1 {
		return nil, errors.New("AllDebrid returned no files result")
	}
	item := response.Data.Magnets[0]
	if item.Error != nil {
		return nil, item.Error.asError()
	}
	files := make([]File, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, file.toFile())
	}
	return files, nil
}

func (c *Client) Unlock(ctx context.Context, link string) (UnlockResult, error) {
	values := url.Values{}
	values.Set("link", link)

	var response struct {
		Data struct {
			Link     string
			Filename string
			Filesize int64
			Delayed  int64
		}
		Error *apiErrorPayload
	}

	if err := c.postForm(ctx, "/v4/link/unlock", values, &response); err != nil {
		return UnlockResult{}, err
	}
	if response.Error != nil {
		return UnlockResult{}, response.Error.asError()
	}
	if response.Data.Link == "" && response.Data.Delayed == 0 {
		return UnlockResult{}, errors.New("AllDebrid returned neither a link nor delayed ID")
	}
	return UnlockResult{
		Link: response.Data.Link, Filename: response.Data.Filename,
		Filesize: response.Data.Filesize, Delayed: response.Data.Delayed,
	}, nil
}

type DelayedResult struct {
	Status   int
	TimeLeft int
	Link     string
}

func (c *Client) Delayed(ctx context.Context, id int64) (DelayedResult, error) {
	values := url.Values{}
	values.Set("id", strconv.FormatInt(id, 10))

	var response struct {
		Data struct {
			Status   int    `json:"status"`
			TimeLeft int    `json:"time_left"`
			Link     string `json:"link"`
		} `json:"data"`
		Error *apiErrorPayload `json:"error"`
	}
	if err := c.postForm(ctx, "/v4/link/delayed", values, &response); err != nil {
		return DelayedResult{}, err
	}
	if response.Error != nil {
		return DelayedResult{}, response.Error.asError()
	}
	if response.Data.Status < 1 || response.Data.Status > 3 {
		return DelayedResult{}, errors.New("AllDebrid returned invalid delayed status")
	}
	return DelayedResult{
		Status:   response.Data.Status,
		TimeLeft: response.Data.TimeLeft,
		Link:     response.Data.Link,
	}, nil
}

type apiErrorPayload struct {
	Code    string
	Message string
}

func (e *apiErrorPayload) asError() error {
	return &APIError{Code: e.Code, Message: e.Message}
}

type fileJSON struct {
	N string
	S int64
	L string
	E []fileJSON
}

func (f fileJSON) toFile() File {
	result := File{Name: f.N, Size: f.S, Link: f.L}
	if len(f.E) != 0 {
		result.Entries = make([]File, 0, len(f.E))
		for _, entry := range f.E {
			result.Entries = append(result.Entries, entry.toFile())
		}
	}
	return result
}

func (c *Client) postForm(ctx context.Context, path string, values url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("create AllDebrid request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("AllDebrid request: %w", err)
	}
	defer response.Body.Close()

	body := io.LimitReader(response.Body, maxResponseBytes)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(io.LimitReader(body, 8<<10))
		return fmt.Errorf("AllDebrid HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if err := json.NewDecoder(body).Decode(target); err != nil {
		return fmt.Errorf("decode AllDebrid response: %w", err)
	}
	return nil
}
