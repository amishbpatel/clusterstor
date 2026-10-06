BEGIN;

ALTER TABLE provider_accounts
  DROP COLUMN IF EXISTS change_cursor;

COMMIT;
