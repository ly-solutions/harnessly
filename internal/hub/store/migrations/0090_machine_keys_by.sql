-- compat: previous
-- Whose consent the machine's current keys carry (docs/hub/machine.md, "Whose keys", HRN-1785): the person whose code
-- registered them last, or the owner who accepted them in the web. A person's own data is given to a machine only while
-- that is its owner. NULL means whoever connected the machine (connected_by, else created_by); "-" means nobody known.
-- A machine handed over before this migration keeps working: its owner of that moment is recorded as having accepted
-- its keys. Additive only.

ALTER TABLE machines ADD COLUMN keys_by TEXT;

UPDATE machines SET keys_by = COALESCE(created_by, '-')
    WHERE connected_by IS NOT NULL AND (created_by IS NULL OR created_by <> connected_by);
