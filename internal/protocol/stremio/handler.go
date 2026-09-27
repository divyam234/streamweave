package stremio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamweave/internal/domain"
	"streamweave/internal/engine"
	"streamweave/internal/protocol/nuvio"
	"streamweave/internal/safehttp"
	"streamweave/internal/secretbox"
)

type Handler struct {
	engine      *engine.Engine
	secureLinks bool
	secrets     *secretbox.Box
}

func NewHandler(e *engine.Engine) *Handler {
	return NewHandlerWithSecureLinks(e, false)
}

func NewHandlerWithSecureLinks(e *engine.Engine, secure bool) *Handler {
	return &Handler{engine: e, secureLinks: secure}
}

func (h *Handler) WithSecrets(box *secretbox.Box) *Handler { h.secrets = box; return h }

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/{installationID}/manifest.json", h.manifest)
	r.Get("/{installationID}/stream/{type}/{id}.json", h.streams)
	r.Get("/{installationID}/play/{token}", h.play)
	r.Head("/{installationID}/play/{token}", h.play)
	return r
}

func (h *Handler) manifest(w http.ResponseWriter, r *http.Request) {
	profile, err := h.engine.InstallationProfile(r.Context(), chi.URLParam(r, "installationID"))
	if err != nil {
		if errors.Is(err, engine.ErrInstallationUnavailable) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "installation lookup failed", http.StatusServiceUnavailable)
		return
	}

	manifest := Manifest{
		ID:          manifestID(chi.URLParam(r, "installationID")),
		Version:     "0.2.0",
		Name:        "StreamWeave",
		Description: "Universal media aggregation for Stremio and Nuvio.",
		Resources:   []string{"stream"},
		Types:       []string{"movie", "series"},
		Catalogs:    []Catalog{},
	}
	if profile.ClientMode == "nuvio" {
		hints := (nuvio.Adapter{}).Manifest(profile)
		manifest.IDPrefixes = hints.IDPrefixes
		manifest.BehaviorHints = &ManifestBehaviorHints{
			Configurable:    hints.Configurable,
			P2P:             hints.P2P,
			P2PNotSupported: hints.P2PNotSupported,
		}
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (h *Handler) streams(w http.ResponseWriter, r *http.Request) {
	candidates, err := h.engine.SearchUnresolved(r.Context(), engine.SearchRequest{
		InstallationID: chi.URLParam(r, "installationID"),
		Media: domain.MediaRef{
			Type: decodePathSegment(chi.URLParam(r, "type")),
			ID:   decodePathSegment(chi.URLParam(r, "id")),
		},
	})
	if err != nil {
		if errors.Is(err, engine.ErrInstallationUnavailable) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "stream lookup failed", http.StatusBadGateway)
		return
	}
	profile, err := h.engine.InstallationProfile(r.Context(), chi.URLParam(r, "installationID"))
	if err != nil {
		http.Error(w, "installation unavailable", http.StatusServiceUnavailable)
		return
	}

	streams := make([]Stream, 0, len(candidates))
	for _, candidate := range candidates {
		item := Stream{
			Name:          displayName(candidate),
			Title:         candidate.Title,
			BehaviorHints: candidateBehaviorHints(candidate),
		}
		if candidate.HTTP != nil {
			streamURL, err := h.streamURL(r, candidate.HTTP.URL)
			if err != nil {
				continue
			}
			item.URL = streamURL
			if len(candidate.HTTP.Headers) != 0 {
				headers := make(map[string]string, len(candidate.HTTP.Headers))
				for key, value := range candidate.HTTP.Headers {
					headers[key] = value
				}
				if item.BehaviorHints == nil {
					item.BehaviorHints = &BehaviorHints{}
				}
				item.BehaviorHints.ProxyHeaders = &ProxyHeaders{Request: headers}
				item.BehaviorHints.NotWebReady = true
			}
		} else if candidate.Torrent != nil {
			if profile.ResolutionMode != engine.ResolutionClient && h.secrets != nil {
				payload, marshalErr := json.Marshal(playbackToken{InstallationID: chi.URLParam(r, "installationID"), Candidate: candidate, Expires: time.Now().Add(4 * time.Hour).Unix()})
				if marshalErr != nil {
					continue
				}
				token, sealErr := h.secrets.SealURL(payload)
				if sealErr != nil {
					continue
				}
				item.URL, sealErr = h.streamURL(r, "/addon/"+chi.URLParam(r, "installationID")+"/play/"+token)
				if sealErr != nil {
					continue
				}
				streams = append(streams, item)
				continue
			}
			item.InfoHash = candidate.Torrent.InfoHash
			item.FileIdx = candidate.Torrent.FileIndex
		} else {
			continue
		}
		streams = append(streams, item)
	}

	writeJSON(w, http.StatusOK, StreamResponse{Streams: streams})
}

type playbackToken struct {
	InstallationID string           `json:"installationId"`
	Candidate      domain.Candidate `json:"candidate"`
	Expires        int64            `json:"expires"`
}

func (h *Handler) play(w http.ResponseWriter, r *http.Request) {
	if h.secrets == nil || len(chi.URLParam(r, "token")) > 8192 {
		http.NotFound(w, r)
		return
	}
	data, err := h.secrets.OpenURL(chi.URLParam(r, "token"))
	var payload playbackToken
	if err != nil || json.Unmarshal(data, &payload) != nil || payload.InstallationID != chi.URLParam(r, "installationID") || payload.Expires < time.Now().Unix() || payload.Expires > time.Now().Add(4*time.Hour+time.Minute).Unix() || payload.Candidate.Torrent == nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	result, err := h.engine.ResolveSelected(ctx, chi.URLParam(r, "installationID"), payload.Candidate)
	if err != nil || result.HTTP == nil {
		http.Error(w, "stream not cached or unavailable; choose another result", http.StatusConflict)
		return
	}
	streamURL, err := h.streamURL(r, result.HTTP.URL)
	if err != nil {
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, streamURL, http.StatusTemporaryRedirect)
}

func (h *Handler) streamURL(r *http.Request, raw string) (string, error) {
	if strings.HasPrefix(raw, "/") {
		scheme := "http"
		if h.secureLinks || r.TLS != nil {
			scheme = "https"
		}
		raw = scheme + "://" + r.Host + raw
	}
	if err := safehttp.ValidatePublicURL(raw); err != nil {
		return "", err
	}
	return raw, nil
}

func candidateBehaviorHints(candidate domain.Candidate) *BehaviorHints {
	filename := candidate.Filename
	if filename == "" {
		filename = filenameFromTitle(candidate.Title)
	}
	if filename == "" && candidate.SizeBytes <= 0 {
		return nil
	}
	return &BehaviorHints{
		Filename:  filename,
		VideoSize: candidate.SizeBytes,
	}
}

// displayName prefers the upstream addon label (e.g. "Torrentio 4k HDR"),
// then the configured provider name, so Stremio never shows internal IDs.
func displayName(candidate domain.Candidate) string {
	if name := strings.TrimSpace(candidate.UpstreamName); name != "" {
		return name
	}
	if name := strings.TrimSpace(candidate.SourceName); name != "" {
		return name
	}
	return "StreamWeave"
}

var videoFilenameSuffix = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|mov|m4v|webm|ts|m2ts)\s*$`)

// filenameFromTitle extracts a video filename from the first line of an
// addon title. Upstream addons such as Torrentio put the release filename
// on the first line followed by details; anything that does not end in a
// known video extension is ignored so free-text titles are never
// misreported as filenames.
func filenameFromTitle(title string) string {
	first := strings.TrimSpace(title)
	if index := strings.Index(first, "\n"); index >= 0 {
		first = strings.TrimSpace(first[:index])
	}
	if len(first) == 0 || len(first) > 256 || !strings.Contains(first, ".") {
		return ""
	}
	if !videoFilenameSuffix.MatchString(first) {
		return ""
	}
	return first
}

// decodePathSegment decodes a route parameter that clients may have
// percent-encoded (Stremio sends series IDs as tt1234567%3A2%3A3).
// The canonical ID keeps literal colons so downstream requests and cache
// keys stay consistent regardless of how the client encoded the URL.
func decodePathSegment(value string) string {
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

func manifestID(installationID string) string {
	sum := sha256.Sum256([]byte(installationID))
	return "streamweave." + hex.EncodeToString(sum[:6])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
