BEGIN;

ALTER TABLE provider_accounts
    ADD COLUMN root_provider_item_id text;

COMMIT;
