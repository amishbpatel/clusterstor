BEGIN;

CREATE TABLE provider_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_account_id uuid NOT NULL REFERENCES provider_accounts(id) ON DELETE CASCADE,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    provider_item_id text NOT NULL,
    provider_parent_item_id text,
    mime_type text,
    size_bytes bigint,
    modified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(provider_account_id, provider_item_id),
    UNIQUE(provider_account_id, node_id)
);

CREATE INDEX provider_items_parent_idx
    ON provider_items(provider_account_id, provider_parent_item_id);

COMMIT;
