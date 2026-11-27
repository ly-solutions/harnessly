-- compat: previous
-- A check or an export the render worker ran on a copy of a project from the store (HRN-1798, kinds 'check' and
-- 'export'): the Content-Disposition of the export's download, as the machine names it. Additive only: the previous
-- binary never reads it.
ALTER TABLE design_render_jobs ADD COLUMN disposition TEXT;
