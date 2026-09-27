-- name: ListInstallations :many
SELECT id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id
FROM installations
ORDER BY name ASC, id ASC;

-- name: GetInstallation :one
SELECT id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id
FROM installations
WHERE id = $1;

-- name: GetInstallationByPublicToken :one
SELECT id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id
FROM installations
WHERE public_token = $1;

-- name: CreateInstallation :one
INSERT INTO installations (name, client_mode, resolution_mode, resolver_id, enabled)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id;


-- name: UpdateInstallationEnabledByPublicToken :one
UPDATE installations
SET enabled = $2,
    updated_at = now()
WHERE public_token = $1
RETURNING id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id;

-- name: RotateInstallationPublicToken :one
UPDATE installations
SET public_token = encode(gen_random_bytes(24), 'hex'),
    updated_at = now()
WHERE public_token = $1
RETURNING id, public_token, name, client_mode, enabled, policy, created_at, updated_at, resolution_mode, resolver_id;
