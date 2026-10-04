BEGIN;

DROP TRIGGER IF EXISTS account_events_notify ON account_events;
DROP FUNCTION IF EXISTS notify_account_event();

COMMIT;
