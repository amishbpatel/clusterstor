BEGIN;

ALTER TABLE provider_accounts
  ADD COLUMN desktop_tree_bootstrapped_at timestamptz;

ALTER TABLE provider_items
  ADD COLUMN provider_revision_id text;

COMMIT;
