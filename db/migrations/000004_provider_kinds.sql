-- +goose Up
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

-- +goose Down
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
