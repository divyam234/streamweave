-- +goose Up
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

-- +goose Down
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
