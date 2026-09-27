-- +goose Up
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

-- +goose Down
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
