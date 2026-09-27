-- +goose Up
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

-- +goose Down
DROP INDEX IF EXISTS installations_public_token_enabled_idx;

ALTER TABLE installations
    DROP CONSTRAINT IF EXISTS installations_public_token_format,
    DROP CONSTRAINT IF EXISTS installations_public_token_unique,
    DROP COLUMN IF EXISTS public_token;
