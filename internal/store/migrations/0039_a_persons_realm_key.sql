-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D297 section 3: the key a person's realm sign-in is proven by, both ways: PBKDF2-SHA256 of their
-- password, salted by the master realm the core's host holds and their name (discovery/proof.RealmKey),
-- made whenever the core sees their password. A subrealm set or changed later leaves it as it was; a
-- sign-in names the person by the whole realm and their name, which the core matches without it.

CREATE TABLE realm_key (                                   -- class A (D297)
  user_id       INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  master_realm  TEXT NOT NULL,           -- the master realm the key was made for (proof.MasterRealm)
  key           BLOB NOT NULL,           -- proof.RealmKey, 32 bytes
  made_at       INTEGER NOT NULL
);
