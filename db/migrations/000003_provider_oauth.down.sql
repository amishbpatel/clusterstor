BEGIN;

DROP INDEX IF EXISTS provider_accounts_external_unique;
DROP INDEX IF EXISTS provider_oauth_states_active_idx;
DROP TABLE IF EXISTS provider_oauth_states;

COMMIT;
