-- Changes of an organization's environment proposed by an agent from a machine (docs/hub/environment.md, «Proposals of an
-- agent»). A machine only files one; an administrator of the organization applies or declines it in the hub, under their
-- own session. A row waits until it is decided or its term ends. source names secrets and holds no value of one.
-- Additive only: the previous binary ignores the new table.
CREATE TABLE env_proposals (
    id           TEXT PRIMARY KEY,
    org_id       TEXT NOT NULL,
    machine_id   TEXT NOT NULL,
    machine_name TEXT NOT NULL DEFAULT '',
    owner_id     TEXT REFERENCES users (id) ON DELETE SET NULL,
    conv         TEXT NOT NULL DEFAULT '',
    conv_title   TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL,
    hint         TEXT NOT NULL DEFAULT '',
    remove       INTEGER NOT NULL DEFAULT 0,
    source       TEXT NOT NULL DEFAULT '{}',
    state        TEXT NOT NULL DEFAULT 'waiting',
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    decided_at   BIGINT,
    decided_by   TEXT REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX env_proposals_org ON env_proposals (org_id, state);
