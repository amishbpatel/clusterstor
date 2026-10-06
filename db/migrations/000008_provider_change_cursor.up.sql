BEGIN;

ALTER TABLE provider_accounts
  ADD COLUMN change_cursor text;

COMMIT;
