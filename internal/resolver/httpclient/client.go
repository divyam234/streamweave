package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxBody = 8 << 20

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    http.Header
	Query      url.Values
}

func New(baseURL string, client *http.Client) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: client,
		Headers:    make(http.Header),
		Query:      make(url.Values),
	}
}

func (c *Client) JSON(ctx context.Context, method, path string, query url.Values, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	return c.do(ctx, method, path, query, reader, "application/json", target)
}

func (c *Client) Form(ctx context.Context, method, path string, query, form url.Values, target any) error {
	if form == nil {
		form = make(url.Values)
	}
	return c.do(ctx, method, path, query, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", target)
}

func (c *Client) Get(ctx context.Context, path string, query url.Values, target any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", target)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, target any) error {
	endpoint, err := url.Parse(c.BaseURL + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return err
	}
	values := endpoint.Query()
	for key, items := range c.Query {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	for key, items := range query {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	for key, items := range c.Headers {
		for _, item := range items {
			req.Header.Add(key, item)
		}
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("upstream HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(payload)))
	}
	if target == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}
