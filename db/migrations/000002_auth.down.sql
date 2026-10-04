BEGIN;

DROP INDEX IF EXISTS users_email_lower_uidx;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;

COMMIT;
