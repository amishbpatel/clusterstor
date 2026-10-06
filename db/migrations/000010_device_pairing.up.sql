BEGIN;

CREATE TABLE device_pairings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_code_hash bytea NOT NULL UNIQUE,
    poll_token_hash bytea NOT NULL UNIQUE,
    requested_name text NOT NULL,
    platform text NOT NULL,
    agent_version text,
    peer_contribution_enabled boolean NOT NULL DEFAULT false,
    peer_contribution_bytes bigint NOT NULL DEFAULT 0 CHECK (peer_contribution_bytes >= 0),
    approved_by_user_id uuid REFERENCES users(id),
    device_id uuid REFERENCES devices(id),
    delivery_ciphertext bytea,
    expires_at timestamptz NOT NULL,
    approved_at timestamptz,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (peer_contribution_enabled OR peer_contribution_bytes = 0)
);

CREATE INDEX device_pairings_expiry_idx
    ON device_pairings(expires_at)
    WHERE consumed_at IS NULL;

COMMIT;
