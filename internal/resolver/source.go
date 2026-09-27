package resolver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dbgen "streamweave/internal/db/gen"
	"streamweave/internal/engine"
	"streamweave/internal/resolver/alldebrid"
	"streamweave/internal/resolver/altmount"
	"streamweave/internal/resolver/debrider"
	"streamweave/internal/resolver/debridlink"
	"streamweave/internal/resolver/easydebrid"
	"streamweave/internal/resolver/easynews"
	"streamweave/internal/resolver/nzbdav"
	"streamweave/internal/resolver/offcloud"
	"streamweave/internal/resolver/pikpak"
	"streamweave/internal/resolver/premiumize"
	"streamweave/internal/resolver/realdebrid"
	"streamweave/internal/resolver/torbox"
	"streamweave/internal/resolver/torrin"
	"streamweave/internal/secretbox"
	usenetnative "streamweave/internal/usenet/native"
)

type BaseURLs struct {
	AllDebrid   string
	RealDebrid  string
	Premiumize  string
	EasyDebrid  string
	TorBox      string
	DebridLink  string
	Offcloud    string
	Debrider    string
	Torrin      string
	PikPakUser  string
	PikPakDrive string
}

type Source struct {
	queries  *dbgen.Queries
	secrets  *secretbox.Box
	client   *http.Client
	baseURLs BaseURLs
	native   *usenetnative.Service
}

func NewSource(queries *dbgen.Queries, secrets *secretbox.Box, client *http.Client, baseURLs BaseURLs, native *usenetnative.Service) *Source {
	return &Source{
		queries:  queries,
		secrets:  secrets,
		client:   client,
		baseURLs: baseURLs,
		native:   native,
	}
}

func (s *Source) ValidateInstallation(ctx context.Context, installationID string) error {
	_, err := s.lookupInstallation(ctx, installationID)
	return err
}

func (s *Source) InstallationProfile(ctx context.Context, installationID string) (engine.InstallationProfile, error) {
	installation, err := s.lookupInstallation(ctx, installationID)
	if err != nil {
		return engine.InstallationProfile{}, err
	}
	return engine.InstallationProfile{
		ClientMode:     installation.ClientMode,
		ResolutionMode: engine.ResolutionMode(installation.ResolutionMode),
	}, nil
}

func (s *Source) lookupInstallation(ctx context.Context, installationID string) (dbgen.GetInstallationByPublicTokenRow, error) {
	if len(installationID) != 48 {
		return dbgen.GetInstallationByPublicTokenRow{}, engine.ErrInstallationUnavailable
	}
	installation, err := s.queries.GetInstallationByPublicToken(ctx, installationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.GetInstallationByPublicTokenRow{}, engine.ErrInstallationUnavailable
	}
	if err != nil {
		return dbgen.GetInstallationByPublicTokenRow{}, err
	}
	if !installation.Enabled {
		return dbgen.GetInstallationByPublicTokenRow{}, engine.ErrInstallationUnavailable
	}
	return installation, nil
}

func (s *Source) ResolutionFor(ctx context.Context, installationID string) (engine.Resolution, error) {
	installation, err := s.lookupInstallation(ctx, installationID)
	if err != nil {
		return engine.Resolution{}, err
	}

	mode := engine.ResolutionMode(installation.ResolutionMode)
	if mode == engine.ResolutionClient || !installation.ResolverID.Valid {
		return engine.Resolution{Mode: mode}, nil
	}
	if s.secrets == nil {
		if mode == engine.ResolutionHybrid {
			return engine.Resolution{Mode: mode}, nil
		}
		return engine.Resolution{}, errors.New("resolver secret encryption is unavailable")
	}

	account, err := s.queries.GetResolverAccount(ctx, installation.ResolverID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !account.Enabled) {
		return engine.Resolution{Mode: mode}, nil
	}
	if err != nil {
		return engine.Resolution{}, err
	}

	secret, err := s.secrets.Open(account.SecretCiphertext, account.SecretNonce)
	if err != nil {
		return engine.Resolution{}, err
	}
	accountID := uuid.UUID(account.ID.Bytes).String()
	apiKey := string(secret)
	client := s.client
	if len(account.ProxyCiphertext) != 0 {
		proxyURL, err := s.secrets.Open(account.ProxyCiphertext, account.ProxyNonce)
		if err != nil {
			return engine.Resolution{}, err
		}
		client, err = proxyClient(string(proxyURL))
		if err != nil {
			return engine.Resolution{}, err
		}
	}

	var selected engine.Resolver
	switch account.Kind {
	case "alldebrid", "all-debrid":
		client, err := alldebrid.NewClientWithBaseURL(apiKey, s.baseURLs.AllDebrid, client)
		if err != nil {
			return engine.Resolution{}, err
		}
		selected = alldebrid.NewResolver(accountID, client)
	case "realdebrid":
		selected, err = realdebrid.NewResolver(accountID, apiKey, s.baseURLs.RealDebrid, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "premiumize":
		selected, err = premiumize.NewResolver(accountID, apiKey, s.baseURLs.Premiumize, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "easydebrid":
		selected, err = easydebrid.NewResolver(accountID, apiKey, s.baseURLs.EasyDebrid, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "torbox":
		selected, err = torbox.NewResolver(accountID, apiKey, s.baseURLs.TorBox, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "debridlink":
		selected, err = debridlink.NewResolver(accountID, apiKey, s.baseURLs.DebridLink, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "offcloud":
		selected, err = offcloud.NewResolver(accountID, apiKey, s.baseURLs.Offcloud, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "debrider":
		selected, err = debrider.NewResolver(accountID, apiKey, s.baseURLs.Debrider, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "torrin":
		selected, err = torrin.NewResolver(accountID, apiKey, s.baseURLs.Torrin, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "pikpak":
		selected, err = pikpak.NewResolver(accountID, apiKey, s.baseURLs.PikPakUser, s.baseURLs.PikPakDrive, client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "nzbdav":
		selected, err = nzbdav.NewResolver(accountID, apiKey, s.client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "altmount":
		selected, err = altmount.NewResolver(accountID, apiKey, s.client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "easynews":
		selected, err = easynews.NewResolver(accountID, apiKey, s.client)
		if err != nil {
			return engine.Resolution{}, err
		}
	case "stremio_nntp", "native_nntp":
		selected, err = usenetnative.NewResolver(accountID, apiKey, s.native)
		if err != nil {
			return engine.Resolution{}, err
		}
	default:
		return engine.Resolution{}, errors.New("unsupported resolver kind")
	}

	if len(account.ProxyCiphertext) != 0 && selected != nil {
		selected = proxyResolver{Resolver: selected, accountID: accountID, box: s.secrets}
	}
	return engine.Resolution{Mode: mode, Resolver: selected}, nil
}
