package stremio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"media-engine/internal/domain"
	"media-engine/internal/engine"
	"media-engine/internal/protocol/nuvio"
	"media-engine/internal/safehttp"
)

type Handler struct {
	engine      *engine.Engine
	secureLinks bool
}

func NewHandler(e *engine.Engine) *Handler {
	return NewHandlerWithSecureLinks(e, false)
}

func NewHandlerWithSecureLinks(e *engine.Engine, secure bool) *Handler {
	return &Handler{engine: e, secureLinks: secure}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/{installationID}/manifest.json", h.manifest)
	r.Get("/{installationID}/stream/{type}/{id}.json", h.streams)
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
		Name:        "Media Engine",
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
	candidates, err := h.engine.Search(r.Context(), engine.SearchRequest{
		InstallationID: chi.URLParam(r, "installationID"),
		Media: domain.MediaRef{
			Type: chi.URLParam(r, "type"),
			ID:   chi.URLParam(r, "id"),
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

	streams := make([]Stream, 0, len(candidates))
	for _, candidate := range candidates {
		item := Stream{
			Name:          candidate.SourceID,
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
			item.InfoHash = candidate.Torrent.InfoHash
			item.FileIdx = candidate.Torrent.FileIndex
		} else {
			continue
		}
		streams = append(streams, item)
	}

	writeJSON(w, http.StatusOK, StreamResponse{Streams: streams})
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
	if candidate.Filename == "" && candidate.SizeBytes <= 0 {
		return nil
	}
	return &BehaviorHints{
		Filename:  candidate.Filename,
		VideoSize: candidate.SizeBytes,
	}
}

func manifestID(installationID string) string {
	sum := sha256.Sum256([]byte(installationID))
	return "media.engine." + hex.EncodeToString(sum[:6])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
