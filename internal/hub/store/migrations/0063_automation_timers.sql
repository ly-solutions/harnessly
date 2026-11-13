-- compat: previous
-- Timers of the automations' "wait" triggers (HRN-1280, docs/hub/automations.md): an event that starts a wait arms a
-- timer for its key, an event that stops it removes the timer, and a timer that comes due queues a run and is removed.
-- Only the hub writes this table. Additive only: a revision without it never reads it. Times are Unix milliseconds;
-- event_payload is the JSON of the event that armed the timer.

CREATE TABLE automation_timers (
    rule_id       TEXT NOT NULL,
    timer_key     TEXT NOT NULL,
    started_at    BIGINT NOT NULL,
    due_at        BIGINT NOT NULL,
    event_payload TEXT NOT NULL,
    PRIMARY KEY (rule_id, timer_key)
);
CREATE INDEX automation_timers_due ON automation_timers (due_at);
