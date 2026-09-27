-- +goose Up
SET LOCAL search_path TO /* TEMPLATE: schema */public, public;

-- 000001_init.sql
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

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

-- 000002_resolvers.sql
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

-- 000003_resolver_kinds.sql
ALTER TABLE resolver_accounts
    DROP CONSTRAINT IF EXISTS resolver_accounts_kind_check;

UPDATE resolver_accounts
SET kind = 'alldebrid'
WHERE kind = 'all-debrid';

ALTER TABLE resolver_accounts
    ADD CONSTRAINT resolver_accounts_kind_check
    CHECK (kind IN (
        'realdebrid',
        'debridlink',
        'premiumize',
        'alldebrid',
        'torbox',
        'easydebrid',
        'debrider',
        'pikpak',
        'offcloud',
        'nzbdav',
        'altmount',
        'stremio_nntp',
        'easynews',
        'native_nntp',
        'torrin'
    ));

-- 000004_provider_kinds.sql
ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check;

ALTER TABLE providers
    ADD COLUMN secret_ciphertext bytea,
    ADD COLUMN secret_nonce bytea;

ALTER TABLE providers
    ADD CONSTRAINT providers_secret_pair_check
    CHECK (
        (secret_ciphertext IS NULL AND secret_nonce IS NULL)
        OR
        (secret_ciphertext IS NOT NULL AND secret_nonce IS NOT NULL)
    );

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN (
        'remote-addon',
        'torznab',
        'newznab',
        'prowlarr',
        'jackett',
        'nzbhydra2',
        'bitmagnet',
        'zilean',
        'eztv',
        'knaben',
        'torrent-galaxy',
        'seadex',
        'torbox-search',
        'easynews-search',
        'gdrive',
        'debrid-library',
        'direct'
    ));

-- 000005_provider_builtin_kinds.sql
ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check;

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN (
        'remote-addon',
        'torznab',
        'newznab',
        'prowlarr',
        'jackett',
        'nzbhydra2',
        'bitmagnet',
        'zilean',
        'eztv',
        'knaben',
        'the-pirate-bay',
        'therarbg',
        'torrent-galaxy',
        'tsukihime',
        'seadex',
        'torbox-search',
        'easynews-search',
        'gdrive',
        'debrid-library',
        'direct'
    ));

-- 000006_supported_service_kinds.sql
DELETE FROM providers
WHERE kind NOT IN (
    'remote-addon',
    'torznab',
    'newznab',
    'prowlarr',
    'jackett',
    'nzbhydra2',
    'eztv',
    'knaben',
    'the-pirate-bay',
    'therarbg',
    'torrent-galaxy',
    'torbox-search'
);

ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check;

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN (
        'remote-addon',
        'torznab',
        'newznab',
        'prowlarr',
        'jackett',
        'nzbhydra2',
        'eztv',
        'knaben',
        'the-pirate-bay',
        'therarbg',
        'torrent-galaxy',
        'torbox-search'
    ));

DELETE FROM resolver_accounts
WHERE kind NOT IN (
    'realdebrid',
    'debridlink',
    'premiumize',
    'alldebrid',
    'torbox',
    'easydebrid',
    'debrider',
    'pikpak',
    'offcloud',
    'torrin'
);

ALTER TABLE resolver_accounts
    DROP CONSTRAINT IF EXISTS resolver_accounts_kind_check;

ALTER TABLE resolver_accounts
    ADD CONSTRAINT resolver_accounts_kind_check
    CHECK (kind IN (
        'realdebrid',
        'debridlink',
        'premiumize',
        'alldebrid',
        'torbox',
        'easydebrid',
        'debrider',
        'pikpak',
        'offcloud',
        'torrin'
    ));

-- 000007_public_installation_tokens.sql
ALTER TABLE installations
    ADD COLUMN public_token text;

UPDATE installations
SET public_token = encode(gen_random_bytes(24), 'hex')
WHERE public_token IS NULL;

ALTER TABLE installations
    ALTER COLUMN public_token SET NOT NULL,
    ALTER COLUMN public_token SET DEFAULT encode(gen_random_bytes(24), 'hex');

ALTER TABLE installations
    ADD CONSTRAINT installations_public_token_unique UNIQUE (public_token),
    ADD CONSTRAINT installations_public_token_format CHECK (public_token ~ '^[a-f0-9]{48}$');

CREATE INDEX installations_public_token_enabled_idx
    ON installations (public_token, enabled);

-- 000008_resolver_proxy.sql
ALTER TABLE resolver_accounts
    ADD COLUMN proxy_ciphertext bytea,
    ADD COLUMN proxy_nonce bytea,
    ADD CONSTRAINT resolver_proxy_pair_check CHECK (
        (proxy_ciphertext IS NULL AND proxy_nonce IS NULL)
        OR (proxy_ciphertext IS NOT NULL AND proxy_nonce IS NOT NULL)
    );

-- +goose Down
SET LOCAL search_path TO /* TEMPLATE: schema */public, public;

-- 000008_resolver_proxy.sql
ALTER TABLE resolver_accounts
    DROP CONSTRAINT resolver_proxy_pair_check,
    DROP COLUMN proxy_ciphertext,
    DROP COLUMN proxy_nonce;

-- 000007_public_installation_tokens.sql
DROP INDEX IF EXISTS installations_public_token_enabled_idx;

ALTER TABLE installations
    DROP CONSTRAINT IF EXISTS installations_public_token_format,
    DROP CONSTRAINT IF EXISTS installations_public_token_unique,
    DROP COLUMN IF EXISTS public_token;

-- 000006_supported_service_kinds.sql
ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check;

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN (
        'remote-addon',
        'torznab',
        'newznab',
        'prowlarr',
        'jackett',
        'nzbhydra2',
        'bitmagnet',
        'zilean',
        'eztv',
        'knaben',
        'the-pirate-bay',
        'therarbg',
        'torrent-galaxy',
        'tsukihime',
        'seadex',
        'torbox-search',
        'easynews-search',
        'gdrive',
        'debrid-library',
        'direct'
    ));

ALTER TABLE resolver_accounts
    DROP CONSTRAINT IF EXISTS resolver_accounts_kind_check;

ALTER TABLE resolver_accounts
    ADD CONSTRAINT resolver_accounts_kind_check
    CHECK (kind IN (
        'realdebrid',
        'debridlink',
        'premiumize',
        'alldebrid',
        'torbox',
        'easydebrid',
        'debrider',
        'pikpak',
        'offcloud',
        'nzbdav',
        'altmount',
        'stremio_nntp',
        'easynews',
        'native_nntp',
        'torrin'
    ));

-- 000005_provider_builtin_kinds.sql
ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check;

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN (
        'remote-addon',
        'torznab',
        'newznab',
        'prowlarr',
        'jackett',
        'nzbhydra2',
        'bitmagnet',
        'zilean',
        'eztv',
        'knaben',
        'torrent-galaxy',
        'seadex',
        'torbox-search',
        'easynews-search',
        'gdrive',
        'debrid-library',
        'direct'
    ));

-- 000004_provider_kinds.sql
DELETE FROM providers
WHERE kind NOT IN ('remote-addon', 'indexer', 'direct');

ALTER TABLE providers
    DROP CONSTRAINT IF EXISTS providers_kind_check,
    DROP CONSTRAINT IF EXISTS providers_secret_pair_check,
    DROP COLUMN IF EXISTS secret_ciphertext,
    DROP COLUMN IF EXISTS secret_nonce;

ALTER TABLE providers
    ADD CONSTRAINT providers_kind_check
    CHECK (kind IN ('remote-addon', 'indexer', 'direct'));

-- 000003_resolver_kinds.sql
ALTER TABLE resolver_accounts
    DROP CONSTRAINT IF EXISTS resolver_accounts_kind_check;

UPDATE resolver_accounts
SET kind = 'all-debrid'
WHERE kind = 'alldebrid';

DELETE FROM resolver_accounts
WHERE kind <> 'all-debrid';

ALTER TABLE resolver_accounts
    ADD CONSTRAINT resolver_accounts_kind_check
    CHECK (kind IN ('all-debrid'));

-- 000002_resolvers.sql
DROP INDEX IF EXISTS installations_resolver_id_idx;
DROP INDEX IF EXISTS resolver_accounts_enabled_idx;

ALTER TABLE installations
    DROP COLUMN IF EXISTS resolver_id,
    DROP COLUMN IF EXISTS resolution_mode;

DROP TABLE IF EXISTS resolver_accounts;

-- 000001_init.sql
DROP TABLE IF EXISTS installations;
DROP TABLE IF EXISTS providers;
