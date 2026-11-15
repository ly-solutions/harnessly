-- An answer the hub worked out from one revision of a design project (HRN-1320): a board's page, the list of the
-- boards, a file as it is served. The revision does not change, so the answer is kept once as a blob of the space's
-- `files` collection and named here by everything that decides it; every replica then gives it from the store without
-- putting the project into a folder. A row whose blob is gone is worked out again. Additive only: a revision without
-- it never reads this table.

CREATE TABLE design_derived (
    key        TEXT NOT NULL PRIMARY KEY,
    space_id   TEXT NOT NULL,
    project    TEXT NOT NULL,
    rev        BIGINT NOT NULL,
    sha256     TEXT NOT NULL,
    size       BIGINT NOT NULL DEFAULT 0,
    type       TEXT NOT NULL DEFAULT '',
    immutable  INTEGER NOT NULL DEFAULT 0,
    headers    TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL
);

CREATE INDEX design_derived_project ON design_derived (space_id, project, rev);
