-- compat: previous
-- A private read-only raster snapshot of selected boards, published by a project's owner for readers who already hold a
-- grant while the owner machine is offline. One per exact project (holder machine and project). Never a link, never a grant.
CREATE TABLE design_private_snapshots (
    machine_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    rev TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    PRIMARY KEY (machine_id, project_id)
);

CREATE TABLE design_private_snapshot_pages (
    machine_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    position BIGINT NOT NULL,
    board TEXT NOT NULL,
    title TEXT NOT NULL,
    jpeg TEXT NOT NULL,
    PRIMARY KEY (machine_id, project_id, position),
    FOREIGN KEY (machine_id, project_id) REFERENCES design_private_snapshots(machine_id, project_id) ON DELETE CASCADE
);
