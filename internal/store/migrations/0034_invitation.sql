-- SPDX-License-Identifier: GPL-3.0-or-later
-- orpheum:raises-compatible-version
-- D246 sections 1 and 2: there is no outsider kind of account; what a person may do is their
-- role's. Any account is set up in full by the admin, or made from an invitation the admin
-- issues under a role, with a note only the admin sees saying who it was meant for; the person
-- fills in their own name and password, and the note stays with the account.

ALTER TABLE user DROP COLUMN is_outsider;
ALTER TABLE user ADD COLUMN admin_note TEXT;    -- who the account is for, the admin's alone (D246 section 2)

-- An invitation not yet used: its link's grant (an issued secret, single use), the role the
-- account will have, and the admin's note. Registering through it makes the account and spends it.
CREATE TABLE invitation (                                  -- class A (D246 section 2)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  secret_sha256 BLOB NOT NULL UNIQUE REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> '')
);
CREATE INDEX invitation_role ON invitation(role_id);
