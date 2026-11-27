-- compat: previous
-- A page of a guest link's snapshot the render worker drew (HRN-1798, kind 'page'): the sha of its JPEG, put as a blob
-- into the space's thumbs collection, which the hub that waits for the page reads. Additive only: the previous binary
-- never reads it.
ALTER TABLE design_render_jobs ADD COLUMN result TEXT;
