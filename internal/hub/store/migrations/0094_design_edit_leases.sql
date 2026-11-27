-- compat: previous
-- The edit lease of a design project (HRN-1794): who edits it now, on which machine, until when. The open editor renews
-- it; the header shows it to everyone else. Advisory: a write is still guarded by its base revision (X-Store-Base).
-- Additive only: the previous binary never reads it.
CREATE TABLE design_edit_leases (
    space_id     TEXT NOT NULL,
    project_id   TEXT NOT NULL,
    person       TEXT NOT NULL,
    name         TEXT NOT NULL,
    machine_id   TEXT NOT NULL,
    machine_name TEXT NOT NULL,
    until_ms     BIGINT NOT NULL,
    PRIMARY KEY (space_id, project_id)
);
