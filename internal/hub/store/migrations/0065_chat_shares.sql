-- A chat's snapshot a person published for the people of the space (HRN-1323). It is a resource of the access model
-- (resaccess kind "chat", as doc_meta is for a document): this row is what the model knows of it — whose it is and in
-- which space — and where its page lies: a blob of the space's `files` collection, so it opens from the hub while the
-- author's machine is off. It ends at expires_at or when its author revokes it. Additive only: a revision without it
-- never reads this table.

CREATE TABLE chat_shares (
    id          TEXT NOT NULL PRIMARY KEY,
    space_id    TEXT NOT NULL,
    machine_id  TEXT NOT NULL,
    owner_id    TEXT NOT NULL,
    owner_email TEXT NOT NULL DEFAULT '',
    sha256      TEXT NOT NULL,
    size        BIGINT NOT NULL DEFAULT 0,
    title       TEXT NOT NULL DEFAULT '',
    expires_at  BIGINT NOT NULL,
    revoked_at  BIGINT,
    created_at  BIGINT NOT NULL
);

CREATE INDEX chat_shares_machine ON chat_shares (machine_id);
