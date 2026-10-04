BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE,
    display_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    platform text NOT NULL,
    agent_version text,
    status text NOT NULL DEFAULT 'registered',
    peer_contribution_enabled boolean NOT NULL DEFAULT false,
    peer_contribution_bytes bigint NOT NULL DEFAULT 0 CHECK (peer_contribution_bytes >= 0),
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE device_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id uuid NOT NULL REFERENCES devices(id),
    secret_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    revoked_at timestamptz
);

CREATE TABLE provider_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider IN ('google_drive','onedrive','dropbox','box')),
    external_account_id text,
    display_name text,
    token_ciphertext bytea,
    status text NOT NULL DEFAULT 'connected',
    quota_total_bytes bigint,
    quota_used_bytes bigint,
    quota_free_bytes bigint,
    last_synced_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    disconnected_at timestamptz,
    UNIQUE(user_id, provider)
);

CREATE TABLE nodes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    parent_id uuid REFERENCES nodes(id),
    node_type text NOT NULL CHECK (node_type IN ('file','folder')),
    name text NOT NULL,
    state text NOT NULL DEFAULT 'active',
    current_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX nodes_parent_idx ON nodes(user_id, parent_id) WHERE deleted_at IS NULL;

CREATE TABLE file_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id uuid NOT NULL REFERENCES nodes(id),
    version_number bigint NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    blake3_hash bytea,
    counts_toward_version_cap boolean NOT NULL DEFAULT true,
    conflict_of_version_id uuid REFERENCES file_versions(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    UNIQUE(node_id, version_number)
);
ALTER TABLE nodes ADD CONSTRAINT nodes_current_version_fk FOREIGN KEY (current_version_id) REFERENCES file_versions(id);

CREATE TABLE storage_objects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES file_versions(id),
    storage_class text NOT NULL,
    provider_account_id uuid REFERENCES provider_accounts(id),
    provider_object_id text,
    object_key text,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    encryption_mode text NOT NULL DEFAULT 'provider_native',
    wrapped_key_ref text,
    state text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT now(),
    verified_at timestamptz,
    deleted_at timestamptz
);

CREATE TABLE account_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    sequence bigint GENERATED ALWAYS AS IDENTITY,
    event_type text NOT NULL,
    resource_type text,
    resource_id uuid,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, sequence)
);
CREATE INDEX account_events_user_sequence_idx ON account_events(user_id, sequence);

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'queued',
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 10,
    run_after timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready_idx ON jobs(status, run_after);

CREATE TABLE entitlements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    entitlement_type text NOT NULL,
    source text NOT NULL,
    quantity_bytes bigint,
    active boolean NOT NULL DEFAULT true,
    starts_at timestamptz NOT NULL DEFAULT now(),
    ends_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL DEFAULT 'stripe',
    external_customer_id text,
    external_subscription_id text,
    plan_code text NOT NULL,
    status text NOT NULL,
    currency char(3) NOT NULL DEFAULT 'USD',
    amount_minor bigint,
    current_period_end timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(provider, external_subscription_id)
);

CREATE TABLE usage_ledger (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    category text NOT NULL,
    delta_bytes bigint NOT NULL,
    resource_type text,
    resource_id uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE feature_flags (
    key text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT false,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO feature_flags(key, enabled, config) VALUES
('peer_foundation_program', false, '{}'),
('peer_storage', false, '{}');

COMMIT;
