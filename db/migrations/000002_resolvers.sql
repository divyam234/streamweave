-- +goose Up
CREATE TABLE resolver_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('all-debrid')),
    enabled boolean NOT NULL DEFAULT true,
    secret_ciphertext bytea NOT NULL,
    secret_nonce bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE installations
    ADD COLUMN resolution_mode text NOT NULL DEFAULT 'hybrid'
        CHECK (resolution_mode IN ('client', 'server', 'hybrid')),
    ADD COLUMN resolver_id uuid REFERENCES resolver_accounts(id) ON DELETE SET NULL;

CREATE INDEX resolver_accounts_enabled_idx ON resolver_accounts (enabled);
CREATE INDEX installations_resolver_id_idx ON installations (resolver_id);

-- +goose Down
DROP INDEX IF EXISTS installations_resolver_id_idx;
DROP INDEX IF EXISTS resolver_accounts_enabled_idx;

ALTER TABLE installations
    DROP COLUMN IF EXISTS resolver_id,
    DROP COLUMN IF EXISTS resolution_mode;

DROP TABLE IF EXISTS resolver_accounts;
