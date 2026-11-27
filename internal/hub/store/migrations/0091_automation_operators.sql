-- compat: previous
-- The people who keep the rights of an automations operator without HUB_AUTOMATIONS_OPERATORS (docs/hub/automations.md,
-- HRN-1784): everybody who owned a rule when the hub learned to ask for the right. Filled here only: a rule made later
-- gives nothing. Additive only.

CREATE TABLE automation_operators (
    user_id TEXT PRIMARY KEY
);

INSERT INTO automation_operators (user_id) SELECT DISTINCT owner_id FROM automation_rules;
