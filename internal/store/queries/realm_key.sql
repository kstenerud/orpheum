-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Realm keys: what a person's realm sign-in is proven by, both ways, one per person, made for the
-- master realm the core's host holds (D297 section 3).

-- name: ListLiveRealmKeys :many
-- Every live person's key made for the master realm, with their name: a sign-in names its person by
-- the whole realm and their name, which the core matches against each.
SELECT realm_key.user_id, user.name, realm_key.key FROM realm_key
JOIN user ON user.id = realm_key.user_id
WHERE realm_key.master_realm = sqlc.arg(master_realm) AND user.removed_at IS NULL
ORDER BY realm_key.user_id;

