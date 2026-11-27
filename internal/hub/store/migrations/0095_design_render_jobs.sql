-- compat: previous
-- The pictures the hub's render worker draws while a project's machine is off (HRN-1798, docs/hub/render-worker.md):
-- a board's gallery picture that the store lacks or that is older than the project's newest revision. The hub queues
-- one row per board and revision; the worker claims it, opens the board from the store under a keyed address of the
-- project's owner (minted at the claim) and sends the picture back. Additive only: the previous binary never reads it.
CREATE TABLE design_render_jobs (
    id          TEXT PRIMARY KEY,
    space_id    TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    board       TEXT NOT NULL,
    kind        TEXT NOT NULL,
    rev         BIGINT NOT NULL,
    machine_id  TEXT NOT NULL,
    owner_id    TEXT NOT NULL,
    prefix      TEXT NOT NULL,
    board_w     INTEGER NOT NULL DEFAULT 0,
    board_h     INTEGER NOT NULL DEFAULT 0,
    state       TEXT NOT NULL,
    attempts    INTEGER NOT NULL DEFAULT 0,
    lease_ms    BIGINT NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_ms  BIGINT NOT NULL,
    updated_ms  BIGINT NOT NULL
);
CREATE UNIQUE INDEX design_render_jobs_want ON design_render_jobs (space_id, project_id, board, kind, rev);
CREATE INDEX design_render_jobs_queue ON design_render_jobs (state, created_ms);
