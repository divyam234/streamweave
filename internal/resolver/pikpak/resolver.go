package pikpak

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/common"
)

const (
	DefaultUserBaseURL  = "https://user.mypikpak.com"
	DefaultDriveBaseURL = "https://api-drive.mypikpak.com"

	clientID      = "YNxT9w7GMdWvEOKa"
	clientSecret  = "dbw2OtmVEeuUvIptb1Coyg"
	clientVersion = "1.47.1"
	packageName   = "com.pikcloud.pikpak"

	pollInterval = 5 * time.Second
	maxWait      = 60 * time.Second
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

var md5Salts = []string{
	"Gez0T9ijiI9WCeTsKSg3SMlx",
	"zQdbalsolyb1R/",
	"ftOjr52zt51JD68C3s",
	"yeOBMH0JkbQdEFNNwQ0RI9T3wU/v",
	"BRJrQZiTQ65WtMvwO",
	"je8fqxKPdQVJiy1DM6Bc9Nb1",
	"niV",
	"9hFCW2R1",
	"sHKHpe2i96",
	"p7c5E6AcXQ/IJUuAEC9W6",
	"",
	"aRv9hjc9P+Pbn+u3krN6",
	"BzStcgE8qVdqjEH16l4",
	"SqgeZvL5j9zoHP95xWHt",
	"zVof5yaJkPe3VFpadPof",
}

type authState struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	ExpiresAt    time.Time
	CaptchaToken string
	CaptchaUntil time.Time
}

type Resolver struct {
	id           string
	username     string
	password     string
	deviceID     string
	userBaseURL  string
	driveBaseURL string
	client       *http.Client
	mu           sync.Mutex
	auth         authState
	pollInterval time.Duration
	maxWait      time.Duration
}

func NewResolver(id, credential, userBaseURL, driveBaseURL string, client *http.Client) (*Resolver, error) {
	username, password, ok := strings.Cut(strings.TrimSpace(credential), ":")
	if !ok || strings.TrimSpace(username) == "" || password == "" {
		return nil, errors.New("PikPak credential must be username:password")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	if userBaseURL == "" {
		userBaseURL = DefaultUserBaseURL
	}
	if driveBaseURL == "" {
		driveBaseURL = DefaultDriveBaseURL
	}
	return &Resolver{
		id:           id,
		username:     username,
		password:     password,
		deviceID:     deviceID(credential),
		userBaseURL:  strings.TrimRight(userBaseURL, "/"),
		driveBaseURL: strings.TrimRight(driveBaseURL, "/"),
		client:       client,
		pollInterval: pollInterval,
		maxWait:      maxWait,
	}, nil
}

func (r *Resolver) ID() string { return r.id }

type responseError struct {
	Error       string `json:"error"`
	Code        int    `json:"error_code"`
	Description string `json:"error_description"`
}

func (e responseError) err() error {
	if e.Error == "" && e.Description == "" {
		return nil
	}
	if e.Description != "" {
		return errors.New(e.Description)
	}
	return errors.New(e.Error)
}

type captchaResponse struct {
	responseError
	CaptchaToken string `json:"captcha_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type signinResponse struct {
	responseError
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Sub          string `json:"sub"`
	ExpiresIn    int64  `json:"expires_in"`
}

type addFileResponse struct {
	responseError
	Task struct {
		ID     string `json:"id"`
		FileID string `json:"file_id"`
		Phase  string `json:"phase"`
	} `json:"task"`
}

type fileResponse struct {
	responseError
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     string `json:"size"`
	Kind     string `json:"kind"`
	Phase    string `json:"phase"`
	ParentID string `json:"parent_id"`
	Medias   []struct {
		IsOrigin bool `json:"is_origin"`
		Priority int  `json:"priority"`
		Link     struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"medias"`
}

type fileListResponse struct {
	responseError
	Files         []fileResponse `json:"files"`
	NextPageToken string         `json:"next_page_token"`
}

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Torrent == nil || candidate.Torrent.InfoHash == "" {
		return candidate, nil
	}
	if err := r.ensureAuth(ctx); err != nil {
		return candidate, err
	}

	var added addFileResponse
	if err := r.driveJSON(ctx, http.MethodPost, "/drive/v1/files", nil, map[string]any{
		"kind":        "drive#file",
		"url":         map[string]string{"url": common.Magnet(candidate.Torrent.InfoHash)},
		"upload_type": "UPLOAD_TYPE_URL",
		"folder_type": "DOWNLOAD",
	}, &added, "POST:/drive/v1/files"); err != nil {
		return candidate, err
	}
	if err := added.responseError.err(); err != nil {
		return candidate, err
	}
	if added.Task.FileID == "" {
		return candidate, errors.New("PikPak returned an empty file id")
	}

	deadline := time.Now().Add(r.maxWait)
	var root fileResponse
	for {
		if err := r.driveJSON(ctx, http.MethodGet, "/drive/v1/files/"+url.PathEscape(added.Task.FileID), nil, nil, &root, "POST:/config/v1/basic"); err != nil {
			return candidate, err
		}
		if err := root.responseError.err(); err != nil {
			return candidate, err
		}
		if root.Phase == "PHASE_TYPE_COMPLETE" {
			break
		}
		if root.Phase == "PHASE_TYPE_ERROR" {
			return candidate, errors.New("PikPak download task failed")
		}
		if time.Now().After(deadline) {
			cached := false
			candidate.Cached = &cached
			return candidate, common.ErrNotReady
		}
		select {
		case <-ctx.Done():
			return candidate, ctx.Err()
		case <-time.After(r.pollInterval):
		}
	}

	files, err := r.playableFiles(ctx, root)
	if err != nil {
		return candidate, err
	}
	selected, ok := common.SelectFile(files, candidate.Torrent.FileIndex)
	if !ok || selected.Link == "" {
		return candidate, errors.New("PikPak returned no playable file")
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{URL: selected.Link}
	candidate.Filename = selected.Name
	if selected.Size > 0 {
		candidate.SizeBytes = selected.Size
	}
	return candidate, nil
}

func (r *Resolver) playableFiles(ctx context.Context, root fileResponse) ([]common.File, error) {
	if root.Kind != "drive#folder" {
		link := bestMediaLink(root)
		size, _ := strconv.ParseInt(root.Size, 10, 64)
		return []common.File{{ID: root.ID, Name: root.Name, Path: root.Name, Size: size, Link: link}}, nil
	}

	files := make([]common.File, 0)
	pageToken := ""
	for {
		query := url.Values{
			"parent_id":      {root.ID},
			"limit":          {"500"},
			"thumbnail_size": {"SIZE_MEDIUM"},
			"filters":        {`{"trashed":{"eq":false},"phase":{"eq":"PHASE_TYPE_COMPLETE"}}`},
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var listed fileListResponse
		if err := r.driveJSON(ctx, http.MethodGet, "/drive/v1/files", query, nil, &listed, "POST:/config/v1/basic"); err != nil {
			return nil, err
		}
		if err := listed.responseError.err(); err != nil {
			return nil, err
		}
		for _, item := range listed.Files {
			if item.Kind == "drive#folder" {
				child, err := r.playableFiles(ctx, item)
				if err != nil {
					return nil, err
				}
				files = append(files, child...)
				continue
			}
			size, _ := strconv.ParseInt(item.Size, 10, 64)
			files = append(files, common.File{
				ID: item.ID, Name: item.Name, Path: item.Name, Size: size, Link: bestMediaLink(item),
			})
		}
		pageToken = listed.NextPageToken
		if pageToken == "" {
			return files, nil
		}
	}
}

func bestMediaLink(file fileResponse) string {
	best := ""
	bestPriority := -1
	for _, media := range file.Medias {
		if media.Link.URL == "" {
			continue
		}
		if media.IsOrigin {
			return media.Link.URL
		}
		if media.Priority > bestPriority {
			bestPriority = media.Priority
			best = media.Link.URL
		}
	}
	return best
}

func (r *Resolver) ensureAuth(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.auth.AccessToken != "" && time.Until(r.auth.ExpiresAt) > 10*time.Minute {
		return nil
	}
	if r.auth.RefreshToken != "" {
		if err := r.refreshLocked(ctx); err == nil {
			return nil
		}
		r.auth = authState{}
	}

	captcha, err := r.captchaLocked(ctx, "POST:"+r.userBaseURL+"/v1/auth/signin", true)
	if err != nil {
		return err
	}
	form := url.Values{
		"captcha_token": {captcha},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"password":      {r.password},
		"username":      {r.username},
	}
	var signed signinResponse
	if err := r.request(ctx, r.userBaseURL, http.MethodPost, "/v1/auth/signin", nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil, &signed); err != nil {
		return err
	}
	if err := signed.responseError.err(); err != nil {
		return err
	}
	if signed.AccessToken == "" {
		return errors.New("PikPak returned an empty access token")
	}
	r.auth.AccessToken = signed.AccessToken
	r.auth.RefreshToken = signed.RefreshToken
	r.auth.UserID = signed.Sub
	r.auth.ExpiresAt = time.Now().Add(time.Duration(signed.ExpiresIn) * time.Second)
	return nil
}

func (r *Resolver) refreshLocked(ctx context.Context) error {
	var response signinResponse
	err := r.request(ctx, r.userBaseURL, http.MethodPost, "/v1/auth/token", nil, jsonBody(map[string]string{
		"refresh_token": r.auth.RefreshToken,
		"client_id":     clientID,
		"grant_type":    "refresh_token",
	}), "application/json", nil, &response)
	if err != nil {
		return err
	}
	if err := response.responseError.err(); err != nil {
		return err
	}
	if response.AccessToken == "" {
		return errors.New("PikPak refresh returned an empty access token")
	}
	r.auth.AccessToken = response.AccessToken
	if response.RefreshToken != "" {
		r.auth.RefreshToken = response.RefreshToken
	}
	if response.Sub != "" {
		r.auth.UserID = response.Sub
	}
	r.auth.ExpiresAt = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	return nil
}

func (r *Resolver) ensureDriveCaptcha(ctx context.Context, action string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auth.CaptchaToken != "" && time.Until(r.auth.CaptchaUntil) > 30*time.Second {
		return r.auth.CaptchaToken, nil
	}
	return r.captchaLocked(ctx, action, false)
}

func (r *Resolver) captchaLocked(ctx context.Context, action string, login bool) (string, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	meta := map[string]any{}
	if login {
		switch {
		case emailPattern.MatchString(r.username):
			meta["email"] = r.username
		case regexp.MustCompile(`^\d{11,18}$`).MatchString(r.username):
			meta["phone_number"] = r.username
		default:
			meta["username"] = r.username
		}
	} else {
		meta["captcha_sign"] = captchaSign(r.deviceID, timestamp)
		meta["client_version"] = clientVersion
		meta["package_name"] = packageName
		meta["timestamp"] = timestamp
		meta["user_id"] = r.auth.UserID
	}

	var response captchaResponse
	if err := r.request(ctx, r.userBaseURL, http.MethodPost, "/v1/shield/captcha/init", nil, jsonBody(map[string]any{
		"action":    action,
		"client_id": clientID,
		"device_id": r.deviceID,
		"meta":      meta,
	}), "application/json", nil, &response); err != nil {
		return "", err
	}
	if err := response.responseError.err(); err != nil {
		return "", err
	}
	if response.CaptchaToken == "" {
		return "", errors.New("PikPak returned an empty captcha token")
	}
	if !login {
		r.auth.CaptchaToken = response.CaptchaToken
		r.auth.CaptchaUntil = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	}
	return response.CaptchaToken, nil
}

func (r *Resolver) driveJSON(ctx context.Context, method, path string, query url.Values, body any, target any, action string) error {
	if err := r.ensureAuth(ctx); err != nil {
		return err
	}
	captcha, err := r.ensureDriveCaptcha(ctx, action)
	if err != nil {
		return err
	}
	headers := http.Header{
		"Authorization":   {"Bearer " + r.auth.AccessToken},
		"X-Captcha-Token": {captcha},
		"X-Device-Id":     {r.deviceID},
	}
	var reader io.Reader
	contentType := ""
	if body != nil {
		reader = jsonBody(body)
		contentType = "application/json"
	}
	return r.request(ctx, r.driveBaseURL, method, path, query, reader, contentType, headers, target)
}

func (r *Resolver) request(ctx context.Context, base, method, path string, query url.Values, body io.Reader, contentType string, extra http.Header, target any) error {
	endpoint, err := url.Parse(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return err
	}
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Client-Id", clientID)
	req.Header.Set("X-Client-Version", clientVersion)
	req.Header.Set("User-Agent", "Mozilla/5.0 MediaEngine/PikPak")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, values := range extra {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("PikPak HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(payload)))
	}
	if target != nil && len(payload) != 0 {
		if err := json.Unmarshal(payload, target); err != nil {
			return fmt.Errorf("decode PikPak response: %w", err)
		}
	}
	return nil
}

func jsonBody(value any) io.Reader {
	data, _ := json.Marshal(value)
	return bytes.NewReader(data)
}

func deviceID(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	raw := append([]byte(nil), sum[:16]...)
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return hex.EncodeToString(raw)
}

func captchaSign(deviceID, timestamp string) string {
	value := clientID + clientVersion + packageName + deviceID + timestamp
	for _, salt := range md5Salts {
		sum := md5.Sum([]byte(value + salt))
		value = hex.EncodeToString(sum[:])
	}
	return "1." + value
}
