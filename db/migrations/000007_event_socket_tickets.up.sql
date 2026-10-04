BEGIN;

CREATE TABLE event_socket_tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ticket_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz
);

CREATE INDEX event_socket_tickets_active_idx
    ON event_socket_tickets(expires_at)
    WHERE consumed_at IS NULL;

COMMIT;
