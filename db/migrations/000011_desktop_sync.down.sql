BEGIN;

ALTER TABLE provider_items
  DROP COLUMN IF EXISTS provider_revision_id;

ALTER TABLE provider_accounts
  DROP COLUMN IF EXISTS desktop_tree_bootstrapped_at;

COMMIT;
