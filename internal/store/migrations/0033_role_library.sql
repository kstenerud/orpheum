-- SPDX-License-Identifier: GPL-3.0-or-later
-- D243 section 4: the libraries a role's members may browse and play, each with whether they may
-- rebroadcast it (section 5, stored now and offered when it is built). The built-in admin role
-- reaches every library and has no rows; any other role reaches only those it lists. The
-- built-in user role is given every live library, so that no one loses what they could see. An
-- image is served by its hash to whoever reaches an item that holds it, which artwork_source finds.

CREATE TABLE role_library (                                -- class A (D243 section 4)
  role_id          INTEGER NOT NULL REFERENCES role(id) ON DELETE CASCADE,
  library_id       INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  may_rebroadcast  INTEGER NOT NULL DEFAULT 0 CHECK (may_rebroadcast IN (0,1)),
  PRIMARY KEY (role_id, library_id)
) WITHOUT ROWID;
CREATE INDEX role_library_library ON role_library(library_id);
CREATE INDEX artwork_source ON artwork(source_sha256) WHERE source_sha256 IS NOT NULL;

INSERT INTO role_library (role_id, library_id)
SELECT role.id, library.id FROM role, library
WHERE role.name = 'user' AND role.is_builtin = 1 AND library.removed_at IS NULL;
