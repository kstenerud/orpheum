-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D302: whether an admin administers from away from home too, their own setting: 'never' (from home
-- alone, D297 section 6) unless they chose 'always'. Passkeys add 'with_passkey' later.

ALTER TABLE user ADD COLUMN admin_away TEXT NOT NULL DEFAULT 'never'
  CHECK (admin_away IN ('never', 'always'));                -- class A (D302)
