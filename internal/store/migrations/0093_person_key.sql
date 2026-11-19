-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A person's away-from-home key, version 2 (D527 section 2.2): proof.PersonKey, salted by the core's
-- key salt and not by the household's name, so it is made whenever the core sees the person's
-- password, linked to a discovery or not, and a household's name given or changed later leaves it
-- working. It replaces the person's version-1 key in realm_key, which the core deletes once it has
-- made this one.
CREATE TABLE person_key (                                   -- class A (D527)
  user_id  INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  key      BLOB NOT NULL,           -- proof.PersonKey, 32 bytes
  made_at  INTEGER NOT NULL
) STRICT;
