BEGIN;

ALTER TABLE storage_objects
  ADD COLUMN provider_revision_id text;

CREATE INDEX storage_objects_provider_revision_idx
  ON storage_objects(provider_account_id, provider_object_id, provider_revision_id)
  WHERE deleted_at IS NULL;

COMMIT;
