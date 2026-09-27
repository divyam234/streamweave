package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"media-engine/internal/api/gen"
	dbgen "media-engine/internal/db/gen"
	"media-engine/internal/provider/presets"
	"media-engine/internal/provider/remoteaddon"
	"media-engine/internal/secretbox"
)

const version = "0.2.0"

var (
	errDatabaseDisabled = errors.New("database is disabled")
	errSecretsDisabled  = errors.New("secret encryption is disabled; configure MASTER_KEY")
)

type Handler struct {
	pool         *pgxpool.Pool
	queries      *dbgen.Queries
	secrets      *secretbox.Box
	allowPrivate bool
}

func NewHandler(pool *pgxpool.Pool, secrets *secretbox.Box, allowPrivate bool) *Handler {
	var queries *dbgen.Queries
	if pool != nil {
		queries = dbgen.New(pool)
	}
	return &Handler{
		pool:         pool,
		queries:      queries,
		secrets:      secrets,
		allowPrivate: allowPrivate,
	}
}

func (h *Handler) ControlApiGetStatus(ctx context.Context) (*gen.StatusResponse, error) {
	database := gen.StatusResponseDatabaseDisabled
	if h.pool != nil {
		database = gen.StatusResponseDatabaseDown
		if err := h.pool.Ping(ctx); err == nil {
			database = gen.StatusResponseDatabaseUp
		}
	}

	secrets := gen.StatusResponseSecretsDisabled
	if h.secrets != nil {
		secrets = gen.StatusResponseSecretsReady
	}

	return &gen.StatusResponse{
		Status:   gen.StatusResponseStatusOk,
		Version:  version,
		Database: database,
		Secrets:  secrets,
	}, nil
}

func (h *Handler) ControlApiListProviders(ctx context.Context) (*gen.ProviderListResponse, error) {
	if h.queries == nil {
		return &gen.ProviderListResponse{Items: []gen.Provider{}}, nil
	}

	rows, err := h.queries.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]gen.Provider, 0, len(rows))
	for _, row := range rows {
		items = append(items, providerDTO(row.ID, row.Name, row.Kind, row.Endpoint, row.Enabled))
	}
	return &gen.ProviderListResponse{Items: items}, nil
}

func (h *Handler) ControlApiCreateProvider(ctx context.Context, req *gen.CreateProviderRequest) (*gen.Provider, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}

	endpoint := strings.TrimSpace(req.Endpoint)
	if preset, ok := req.Preset.Get(); ok {
		if req.Kind != gen.ProviderKindRemoteAddon {
			return nil, errors.New("remote addon presets require kind remote-addon")
		}
		var err error
		endpoint, err = presets.Resolve(string(preset), endpoint)
		if err != nil {
			return nil, err
		}
	}
	if req.Kind == gen.ProviderKindRemoteAddon && endpoint == "" {
		return nil, errors.New("remote addon endpoint or preset is required")
	}
	if endpoint != "" {
		normalized, err := remoteaddon.ValidateEndpoint(endpoint, h.allowPrivate)
		if err != nil {
			return nil, err
		}
		endpoint = normalized
	}

	var ciphertext, nonce []byte
	if apiKey, ok := req.ApiKey.Get(); ok && strings.TrimSpace(apiKey) != "" {
		if h.secrets == nil {
			return nil, errSecretsDisabled
		}
		var err error
		ciphertext, nonce, err = h.secrets.Seal([]byte(strings.TrimSpace(apiKey)))
		if err != nil {
			return nil, err
		}
	}

	row, err := h.queries.CreateProvider(ctx, dbgen.CreateProviderParams{
		Name:             strings.TrimSpace(req.Name),
		Kind:             string(req.Kind),
		Endpoint:         endpoint,
		Enabled:          req.Enabled,
		Config:           []byte("{}"),
		SecretCiphertext: ciphertext,
		SecretNonce:      nonce,
	})
	if err != nil {
		return nil, err
	}

	result := providerDTO(row.ID, row.Name, row.Kind, row.Endpoint, row.Enabled)
	return &result, nil
}

func (h *Handler) ControlApiUpdateProvider(ctx context.Context, req *gen.UpdateProviderRequest, params gen.ControlApiUpdateProviderParams) (*gen.Provider, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}
	id, err := requiredUUID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.queries.UpdateProviderEnabled(ctx, dbgen.UpdateProviderEnabledParams{
		ID:      id,
		Enabled: req.Enabled,
	})
	if err != nil {
		return nil, err
	}
	result := providerDTO(row.ID, row.Name, row.Kind, row.Endpoint, row.Enabled)
	return &result, nil
}

func (h *Handler) ControlApiListResolverAccounts(ctx context.Context) (*gen.ResolverAccountListResponse, error) {
	if h.queries == nil {
		return &gen.ResolverAccountListResponse{Items: []gen.ResolverAccount{}}, nil
	}

	rows, err := h.queries.ListResolverAccounts(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]gen.ResolverAccount, 0, len(rows))
	for _, row := range rows {
		items = append(items, gen.ResolverAccount{
			ID:      uuidFromPG(row.ID),
			Name:    row.Name,
			Kind:    gen.ResolverKind(row.Kind),
			Enabled: row.Enabled,
		})
	}
	return &gen.ResolverAccountListResponse{Items: items}, nil
}

func (h *Handler) ControlApiCreateResolverAccount(ctx context.Context, req *gen.CreateResolverAccountRequest) (*gen.ResolverAccount, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}
	if h.secrets == nil {
		return nil, errSecretsDisabled
	}

	apiKey := strings.TrimSpace(req.ApiKey)
	if apiKey == "" {
		return nil, errors.New("apiKey is required")
	}
	ciphertext, nonce, err := h.secrets.Seal([]byte(apiKey))
	if err != nil {
		return nil, err
	}

	row, err := h.queries.CreateResolverAccount(ctx, dbgen.CreateResolverAccountParams{
		Name:             strings.TrimSpace(req.Name),
		Kind:             string(req.Kind),
		Enabled:          req.Enabled,
		SecretCiphertext: ciphertext,
		SecretNonce:      nonce,
	})
	if err != nil {
		return nil, err
	}

	return &gen.ResolverAccount{
		ID:      uuidFromPG(row.ID),
		Name:    row.Name,
		Kind:    gen.ResolverKind(row.Kind),
		Enabled: row.Enabled,
	}, nil
}

func (h *Handler) ControlApiUpdateResolverAccount(ctx context.Context, req *gen.UpdateResolverAccountRequest, params gen.ControlApiUpdateResolverAccountParams) (*gen.ResolverAccount, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}
	id, err := requiredUUID(params.ID)
	if err != nil {
		return nil, err
	}
	row, err := h.queries.UpdateResolverAccountEnabled(ctx, dbgen.UpdateResolverAccountEnabledParams{
		ID:      id,
		Enabled: req.Enabled,
	})
	if err != nil {
		return nil, err
	}
	return &gen.ResolverAccount{
		ID:      uuidFromPG(row.ID),
		Name:    row.Name,
		Kind:    gen.ResolverKind(row.Kind),
		Enabled: row.Enabled,
	}, nil
}

func (h *Handler) ControlApiListInstallations(ctx context.Context) (*gen.InstallationListResponse, error) {
	if h.queries == nil {
		return &gen.InstallationListResponse{Items: []gen.Installation{}}, nil
	}

	rows, err := h.queries.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]gen.Installation, 0, len(rows))
	for _, row := range rows {
		items = append(items, installationDTO(row.PublicToken, row.Name, row.ClientMode, row.ResolutionMode, row.ResolverID, row.Enabled))
	}
	return &gen.InstallationListResponse{Items: items}, nil
}

func (h *Handler) ControlApiCreateInstallation(ctx context.Context, req *gen.CreateInstallationRequest) (*gen.Installation, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}

	resolverID, err := optionalUUID(req.ResolverId)
	if err != nil {
		return nil, fmt.Errorf("resolverId: %w", err)
	}
	if req.ResolutionMode == gen.CreateInstallationRequestResolutionModeServer && !resolverID.Valid {
		return nil, errors.New("resolverId is required for server resolution mode")
	}

	row, err := h.queries.CreateInstallation(ctx, dbgen.CreateInstallationParams{
		Name:           strings.TrimSpace(req.Name),
		ClientMode:     string(req.ClientMode),
		ResolutionMode: string(req.ResolutionMode),
		ResolverID:     resolverID,
		Enabled:        req.Enabled,
	})
	if err != nil {
		return nil, err
	}

	result := installationDTO(row.PublicToken, row.Name, row.ClientMode, row.ResolutionMode, row.ResolverID, row.Enabled)
	return &result, nil
}

func (h *Handler) ControlApiUpdateInstallation(ctx context.Context, req *gen.UpdateInstallationRequest, params gen.ControlApiUpdateInstallationParams) (*gen.Installation, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}
	row, err := h.queries.UpdateInstallationEnabledByPublicToken(ctx, dbgen.UpdateInstallationEnabledByPublicTokenParams{
		PublicToken: params.ID,
		Enabled:     req.Enabled,
	})
	if err != nil {
		return nil, err
	}
	result := installationDTO(row.PublicToken, row.Name, row.ClientMode, row.ResolutionMode, row.ResolverID, row.Enabled)
	return &result, nil
}

func (h *Handler) ControlApiRotateInstallationToken(ctx context.Context, params gen.ControlApiRotateInstallationTokenParams) (*gen.Installation, error) {
	if h.queries == nil {
		return nil, errDatabaseDisabled
	}
	row, err := h.queries.RotateInstallationPublicToken(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	result := installationDTO(row.PublicToken, row.Name, row.ClientMode, row.ResolutionMode, row.ResolverID, row.Enabled)
	return &result, nil
}

func providerDTO(id pgtype.UUID, name, kind, endpoint string, enabled bool) gen.Provider {
	return gen.Provider{
		ID:       uuidFromPG(id),
		Name:     name,
		Kind:     gen.ProviderKind(kind),
		Endpoint: endpoint,
		Enabled:  enabled,
	}
}

func installationDTO(publicToken, name, clientMode, resolutionMode string, resolverID pgtype.UUID, enabled bool) gen.Installation {
	result := gen.Installation{
		ID:             publicToken,
		Name:           name,
		ClientMode:     gen.InstallationClientMode(clientMode),
		ResolutionMode: gen.InstallationResolutionMode(resolutionMode),
		Enabled:        enabled,
	}
	if resolverID.Valid {
		result.ResolverId = gen.NewOptString(uuidFromPG(resolverID))
	}
	return result
}

func requiredUUID(raw string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func optionalUUID(value gen.OptString) (pgtype.UUID, error) {
	raw, ok := value.Get()
	if !ok || strings.TrimSpace(raw) == "" {
		return pgtype.UUID{}, nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func uuidFromPG(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
