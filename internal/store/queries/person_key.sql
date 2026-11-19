-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Person keys: a person's away-from-home key, version 2, salted by the core's key salt, one per
-- person, made whenever the core sees their password (D527 section 2.2).

-- name: SetPersonKey :exec
-- The person's key, replacing the one they had: a new password makes another.
INSERT INTO person_key (user_id, key, made_at)
VALUES (sqlc.arg(user_id), sqlc.arg(key), sqlc.arg(made_at))
ON CONFLICT (user_id) DO UPDATE SET key = excluded.key, made_at = excluded.made_at
-- Only when it changes something (D303, DL9).
WHERE (person_key.key, person_key.made_at) IS NOT (excluded.key, excluded.made_at);

-- name: ListLivePersonKeys :many
-- Every live person's key, with their name: a sign-in names its person by the whole realm and their
-- name, which the core matches against each.
SELECT person_key.user_id, user.name, person_key.key FROM person_key
JOIN user ON user.id = person_key.user_id
WHERE user.removed_at IS NULL
ORDER BY person_key.user_id;

-- name: HasPersonKey :one
-- Whether the person has a key: a sign-in at home that finds one makes none again.
SELECT EXISTS (SELECT 1 FROM person_key WHERE user_id = sqlc.arg(user_id));

-- name: DeletePersonKeyOfRemovedUser :exec
DELETE FROM person_key WHERE user_id = sqlc.arg(user_id);
