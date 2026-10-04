BEGIN;

ALTER TABLE users ADD COLUMN password_hash text;
CREATE UNIQUE INDEX users_email_lower_uidx ON users (lower(email));

COMMIT;
