-- name: ListResolverAccounts :many
SELECT id, name, kind, enabled, proxy_ciphertext IS NOT NULL AS proxy_enabled, created_at, updated_at
FROM /* TEMPLATE: schema */resolver_accounts
ORDER BY name ASC, id ASC;

-- name: GetResolverAccount :one
SELECT id, name, kind, enabled, secret_ciphertext, secret_nonce, proxy_ciphertext, proxy_nonce, created_at, updated_at
FROM /* TEMPLATE: schema */resolver_accounts
WHERE id = $1;

-- name: CreateResolverAccount :one
INSERT INTO /* TEMPLATE: schema */resolver_accounts (name, kind, enabled, secret_ciphertext, secret_nonce, proxy_ciphertext, proxy_nonce)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, kind, enabled, proxy_ciphertext IS NOT NULL AS proxy_enabled, created_at, updated_at;


-- name: UpdateResolverAccountEnabled :one
UPDATE /* TEMPLATE: schema */resolver_accounts
SET enabled = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, name, kind, enabled, proxy_ciphertext IS NOT NULL AS proxy_enabled, created_at, updated_at;
