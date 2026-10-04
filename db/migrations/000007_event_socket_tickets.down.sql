BEGIN;

DROP INDEX IF EXISTS event_socket_tickets_active_idx;
DROP TABLE IF EXISTS event_socket_tickets;

COMMIT;
