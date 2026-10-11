-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Sealed third-party secrets (schema.md, secret; D194 section 3). Every query's name carries
-- SealedSecret, and only internal/core/secrets may call them (forbidigo [sealed-secret]).
-- A server-scope row has a NULL user_id; `user_id IS` matches it with a nil parameter.

-- name: GetSealedSecret :one
SELECT scope, user_id, name, key_id, nonce, ciphertext, last_chars, set_by_user_id, set_at
FROM secret
WHERE scope = sqlc.arg(scope) AND user_id IS sqlc.arg(user_id) AND name = sqlc.arg(name);

-- name: ListSealedSecretsInScope :many
SELECT scope, user_id, name, key_id, last_chars, set_by_user_id, set_at
FROM secret
WHERE scope = sqlc.arg(scope) AND user_id IS sqlc.arg(user_id)
ORDER BY name;

-- name: ListAllSealedSecrets :many
SELECT scope, user_id, name, key_id
FROM secret
ORDER BY scope, user_id, name;

-- name: UpsertSealedSecret :exec
INSERT INTO secret (scope, user_id, name, key_id, nonce, ciphertext, last_chars, set_by_user_id, set_at)
VALUES (sqlc.arg(scope), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(key_id), sqlc.arg(nonce),
        sqlc.arg(ciphertext), sqlc.arg(last_chars), sqlc.arg(set_by_user_id), sqlc.arg(set_at))
ON CONFLICT (scope, ifnull(user_id, 0), name)
DO UPDATE SET key_id = excluded.key_id, nonce = excluded.nonce, ciphertext = excluded.ciphertext,
              last_chars = excluded.last_chars, set_by_user_id = excluded.set_by_user_id, set_at = excluded.set_at;

-- name: DeleteSealedSecret :exec
-- Clearing a secret removes its ciphertext, as DELETE /secrets/NAME does in D194 section 5. Nothing
-- references a secret row, so this is not the tombstone case of standards/sql.md section 6.
DELETE FROM secret
WHERE scope = sqlc.arg(scope) AND user_id IS sqlc.arg(user_id) AND name = sqlc.arg(name);
