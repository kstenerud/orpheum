-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Realm keys: what a person's realm sign-in is proven by, both ways, one per person, made for the
-- master realm the core's host holds (D297 section 3).

-- name: SetRealmKey :exec
-- The person's key, replacing the one they had: a new password or a new master realm makes another.
INSERT INTO realm_key (user_id, master_realm, key, made_at)
VALUES (sqlc.arg(user_id), sqlc.arg(master_realm), sqlc.arg(key), sqlc.arg(made_at))
ON CONFLICT (user_id) DO UPDATE SET master_realm = excluded.master_realm, key = excluded.key, made_at = excluded.made_at;

-- name: ListLiveRealmKeys :many
-- Every live person's key made for the master realm, with their name: a sign-in names its person by
-- the whole realm and their name, which the core matches against each.
SELECT realm_key.user_id, user.name, realm_key.key FROM realm_key
JOIN user ON user.id = realm_key.user_id
WHERE realm_key.master_realm = sqlc.arg(master_realm) AND user.removed_at IS NULL
ORDER BY realm_key.user_id;

-- name: GetRealmKeyMasterOfUser :one
-- The master realm the person's key was made for: what tells whether it serves the realm the core has.
SELECT master_realm FROM realm_key WHERE user_id = sqlc.arg(user_id);
