package provider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	dbgen "media-engine/internal/db/gen"
	"media-engine/internal/engine"
	"media-engine/internal/provider/eztv"
	"media-engine/internal/provider/nab"
	"media-engine/internal/provider/prowlarr"
	"media-engine/internal/provider/publicindexers"
	"media-engine/internal/provider/remoteaddon"
	"media-engine/internal/provider/torboxsearch"
	"media-engine/internal/provider/tsukihime"
	"media-engine/internal/secretbox"
)

type Source struct {
	queries      *dbgen.Queries
	secrets      *secretbox.Box
	client       *http.Client
	allowPrivate bool
}

func NewSource(queries *dbgen.Queries, secrets *secretbox.Box, client *http.Client, allowPrivate bool) *Source {
	return &Source{
		queries:      queries,
		secrets:      secrets,
		client:       client,
		allowPrivate: allowPrivate,
	}
}

func (s *Source) Providers(ctx context.Context) ([]engine.Provider, error) {
	rows, err := s.queries.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	providers := make([]engine.Provider, 0, len(rows))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}

		id := uuid.UUID(row.ID.Bytes).String()
		apiKey, ok := s.providerSecret(row.SecretCiphertext, row.SecretNonce)
		if !ok {
			continue
		}
		if row.Endpoint != "" {
			normalized, validationErr := remoteaddon.ValidateEndpoint(row.Endpoint, s.allowPrivate)
			if validationErr != nil {
				continue
			}
			row.Endpoint = normalized
		}

		var item engine.Provider
		switch row.Kind {
		case "remote-addon":
			item, err = remoteaddon.New(id, row.Endpoint, s.client, s.allowPrivate)
		case "torznab", "jackett":
			if _, validationErr := remoteaddon.ValidateEndpoint(row.Endpoint, s.allowPrivate); validationErr != nil {
				err = validationErr
				break
			}
			item, err = nab.New(id, nab.Torznab, row.Endpoint, apiKey, s.client)
		case "newznab", "nzbhydra2":
			if _, validationErr := remoteaddon.ValidateEndpoint(row.Endpoint, s.allowPrivate); validationErr != nil {
				err = validationErr
				break
			}
			item, err = nab.New(id, nab.Newznab, row.Endpoint, apiKey, s.client)
		case "prowlarr":
			if _, validationErr := remoteaddon.ValidateEndpoint(row.Endpoint, s.allowPrivate); validationErr != nil {
				err = validationErr
				break
			}
			item, err = prowlarr.New(id, row.Endpoint, apiKey, s.client)
		case "eztv":
			item, err = eztv.New(id, row.Endpoint, s.client)
		case "knaben":
			item, err = publicindexers.New(id, publicindexers.Knaben, row.Endpoint, s.client)
		case "the-pirate-bay":
			item, err = publicindexers.New(id, publicindexers.ThePirateBay, row.Endpoint, s.client)
		case "therarbg":
			item, err = publicindexers.New(id, publicindexers.TheRARBG, row.Endpoint, s.client)
		case "torrent-galaxy":
			item, err = publicindexers.New(id, publicindexers.TorrentGalaxy, row.Endpoint, s.client)
		case "tsukihime":
			var config struct {
				StorageURL string `json:"storageUrl"`
			}
			_ = json.Unmarshal(row.Config, &config)
			item, err = tsukihime.New(id, row.Endpoint, config.StorageURL, s.client)
		case "torbox-search":
			item, err = torboxsearch.New(id, row.Endpoint, apiKey, s.client)
		default:
			continue
		}
		if err != nil {
			continue
		}
		providers = append(providers, item)
	}
	return providers, nil
}

func (s *Source) providerSecret(ciphertext, nonce []byte) (string, bool) {
	if len(ciphertext) == 0 && len(nonce) == 0 {
		return "", true
	}
	if s.secrets == nil {
		return "", false
	}
	plaintext, err := s.secrets.Open(ciphertext, nonce)
	if err != nil {
		return "", false
	}
	return string(plaintext), true
}
