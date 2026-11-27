-- compat: previous
-- HRN-1799 (HRN-1790, stage 7). Additive only: the previous binary never reads it.
-- A comment that calls the agent while the machine is off waits here, and goes to the machine when its link is back.
-- path is the platform route it came by; claimed_ms keeps two replicas from handing one on twice.
CREATE TABLE design_agent_comments (
    id         TEXT PRIMARY KEY,
    machine_id TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    project_id TEXT NOT NULL,
    comment_id TEXT NOT NULL,
    chat       TEXT NOT NULL,
    path       TEXT NOT NULL,
    created_ms BIGINT NOT NULL,
    claimed_ms BIGINT,
    tries      INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX design_agent_comments_once ON design_agent_comments (machine_id, project_id, comment_id);
CREATE INDEX design_agent_comments_machine ON design_agent_comments (machine_id, created_ms);
