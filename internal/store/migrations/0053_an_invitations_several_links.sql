-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D318, D311 section 4: an invitation may have several links outstanding, each its own grant, and
-- using any one ends the others; once used, it names the account it made, which its admin sees.
-- Its one link moves to invitation_link, and the invitation keeps when it was made, and what it made.
CREATE TABLE invitation_new (                              -- class A (D246 section 2, D318)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> ''),
  issued_at     INTEGER NOT NULL,
  made_user_id  INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- the account its use made
  made_at       INTEGER,
  CHECK ((made_user_id IS NULL) = (made_at IS NULL))
) STRICT;
INSERT INTO invitation_new (id, role_id, admin_note, issued_at)
SELECT invitation.id, invitation.role_id, invitation.admin_note, "grant".issued_at
FROM invitation JOIN "grant" ON "grant".secret_sha256 = invitation.secret_sha256;
CREATE TABLE invitation_link (                             -- class A (D318): one of an invitation's links
  secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  invitation_id INTEGER NOT NULL REFERENCES invitation(id) ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;
INSERT INTO invitation_link (secret_sha256, invitation_id) SELECT secret_sha256, id FROM invitation;
-- The ids of invitations deleted before are never issued again (D173): the copy takes the old
-- table's sequence, which the rows copied may be below.
INSERT INTO sqlite_sequence (name, seq)
SELECT 'invitation_new', seq FROM sqlite_sequence
WHERE name = 'invitation' AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'invitation_new');
UPDATE sqlite_sequence SET seq = max(seq, (SELECT seq FROM sqlite_sequence WHERE name = 'invitation'))
WHERE name = 'invitation_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'invitation');
DROP TABLE invitation;
ALTER TABLE invitation_new RENAME TO invitation;
CREATE INDEX invitation_role ON invitation(role_id);
CREATE INDEX invitation_made_user ON invitation(made_user_id) WHERE made_user_id IS NOT NULL;
CREATE INDEX invitation_link_invitation ON invitation_link(invitation_id);
