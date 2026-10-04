BEGIN;
DROP INDEX IF EXISTS provider_items_parent_idx;
DROP TABLE IF EXISTS provider_items;
COMMIT;
