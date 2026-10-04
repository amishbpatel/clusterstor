BEGIN;

ALTER TABLE provider_accounts
    DROP COLUMN IF EXISTS root_provider_item_id;

COMMIT;
