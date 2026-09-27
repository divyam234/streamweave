-- +goose Up
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
        'torrent-galaxy',
        'seadex',
        'torbox-search',
        'easynews-search',
        'gdrive',
        'debrid-library',
        'direct'
    ));
