BEGIN;

DROP INDEX IF EXISTS storage_objects_provider_revision_idx;

ALTER TABLE storage_objects
  DROP COLUMN IF EXISTS provider_revision_id;

COMMIT;
