package easynews

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/credentialjson"
)

type credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Resolver struct {
	id       string
	username string
	password string
	client   *http.Client
}

func NewResolver(id, value string, client *http.Client) (*Resolver, error) {
	var cfg credential
	if err := credentialjson.Decode(value, &cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Username) == "" || cfg.Password == "" {
		return nil, errors.New("Easynews credential requires username and password")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Resolver{id: id, username: cfg.Username, password: cfg.Password, client: &copyClient}, nil
}

func (r *Resolver) ID() string { return r.id }

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Usenet == nil || strings.TrimSpace(candidate.Usenet.EasynewsURL) == "" {
		return candidate, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.Usenet.EasynewsURL, nil)
	if err != nil {
		return candidate, err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(r.username+":"+r.password)))
	req.Header.Set("Range", "bytes=0-0")

	res, err := r.client.Do(req)
	if err != nil {
		return candidate, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusMovedPermanently && res.StatusCode != http.StatusFound &&
		res.StatusCode != http.StatusTemporaryRedirect && res.StatusCode != http.StatusPermanentRedirect {
		return candidate, errors.New("Easynews did not return a playback redirect")
	}
	location := strings.TrimSpace(res.Header.Get("Location"))
	if location == "" {
		return candidate, errors.New("Easynews returned an empty playback redirect")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: location}
	return candidate, nil
}
