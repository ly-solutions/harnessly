-- compat: previous
-- The tenants that keep the folders of HUB_SECRETS_REF_PREFIXES (docs/safe.md, HRN-1785): every organization (by its id)
-- and every space outside any ("space:<id>") there was when the folders became per organization. Filled here only. Empty
-- on a new installation. Additive only.

CREATE TABLE safe_ref_tenants (
    tenant TEXT PRIMARY KEY
);

INSERT INTO safe_ref_tenants (tenant) SELECT id FROM orgs;
INSERT INTO safe_ref_tenants (tenant) SELECT 'space:' || id FROM spaces WHERE org_id IS NULL;
