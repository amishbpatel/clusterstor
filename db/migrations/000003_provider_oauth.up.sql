BEGIN;

CREATE TABLE provider_oauth_states (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider IN ('google_drive','onedrive','dropbox','box')),
    state_hash bytea NOT NULL UNIQUE,
    redirect_uri text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz
);

CREATE INDEX provider_oauth_states_active_idx
    ON provider_oauth_states(user_id, provider, expires_at)
    WHERE consumed_at IS NULL;

CREATE UNIQUE INDEX provider_accounts_external_unique
    ON provider_accounts(provider, external_account_id)
    WHERE external_account_id IS NOT NULL;

COMMIT;
