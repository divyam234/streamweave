-- name: ListProviders :many
SELECT id, name, kind, endpoint, enabled, config, secret_ciphertext, secret_nonce, created_at, updated_at
FROM /* TEMPLATE: schema */providers
ORDER BY name ASC, id ASC;

-- name: GetProvider :one
SELECT id, name, kind, endpoint, enabled, config, secret_ciphertext, secret_nonce, created_at, updated_at
FROM /* TEMPLATE: schema */providers
WHERE id = $1;

-- name: CreateProvider :one
INSERT INTO /* TEMPLATE: schema */providers (
    name,
    kind,
    endpoint,
    enabled,
    config,
    secret_ciphertext,
    secret_nonce
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, kind, endpoint, enabled, config, secret_ciphertext, secret_nonce, created_at, updated_at;


-- name: UpdateProviderEnabled :one
UPDATE /* TEMPLATE: schema */providers
SET enabled = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, name, kind, endpoint, enabled, config, secret_ciphertext, secret_nonce, created_at, updated_at;
