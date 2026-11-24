-- Keep link membership distinct from a direct grant to the same recipient.
CREATE TABLE design_invite_acceptances (
    invite_id TEXT NOT NULL REFERENCES design_invites(id) ON DELETE CASCADE,
    person_email TEXT NOT NULL,
    granted_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    PRIMARY KEY (invite_id, person_email)
);
CREATE INDEX design_invite_recipient ON design_invite_acceptances(person_email);

INSERT INTO design_invite_acceptances(invite_id, person_email, granted_at, expires_at)
SELECT i.id, m.person_email, m.granted_at,
       CASE WHEN m.expires_at < i.expires_at THEN m.expires_at ELSE i.expires_at END
FROM design_members m JOIN design_invites i ON i.id=m.via_invite
AND i.machine_id=m.machine_id AND i.project_id=m.project_id;
DELETE FROM design_members WHERE via_invite IN (SELECT id FROM design_invites);
