BEGIN;

CREATE OR REPLACE FUNCTION notify_account_event() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify(
        'clusterstor_account_events',
        json_build_object(
            'user_id', NEW.user_id::text,
            'sequence', NEW.sequence
        )::text
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER account_events_notify
AFTER INSERT ON account_events
FOR EACH ROW
EXECUTE FUNCTION notify_account_event();

COMMIT;
