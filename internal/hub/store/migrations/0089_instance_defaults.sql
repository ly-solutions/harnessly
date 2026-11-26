-- compat: previous
-- What an installation chose for a setting the operator leaves unset, kept at its first start with a version that has
-- the choice: a new installation takes the safe value, an older one keeps behaving as it did (HRN-1774).
CREATE TABLE instance_defaults (
    name TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
