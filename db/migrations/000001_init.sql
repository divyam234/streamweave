-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE providers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('remote-addon', 'indexer', 'direct')),
    endpoint text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE installations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    client_mode text NOT NULL CHECK (client_mode IN ('stremio', 'nuvio', 'universal')),
    enabled boolean NOT NULL DEFAULT true,
    policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX providers_enabled_idx ON providers (enabled);
CREATE INDEX installations_enabled_idx ON installations (enabled);

-- +goose Down
DROP TABLE IF EXISTS installations;
DROP TABLE IF EXISTS providers;
