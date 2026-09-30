-- SPDX-License-Identifier: GPL-3.0-or-later
-- A person's devices (schema.md, device; D217 section 8): made at sign-in, named by the client.

-- name: InsertDevice :one
INSERT INTO device (user_id, name, platform, added_at, last_signed_in_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(platform), sqlc.arg(signed_in_at), sqlc.arg(signed_in_at))
RETURNING id;

-- name: ListLiveDevicesOfUser :many
-- A person's devices, most recently used first: when each was last used, by the latest use of any
-- of its sessions, and whether any is still live.
SELECT device.id, device.name, device.platform, device.audio_output, device.added_at,
  CAST(coalesce(max("grant".last_used_at), device.added_at) AS INTEGER) AS last_used_at,
  CAST(coalesce(max("grant".revoked_at IS NULL AND ("grant".expires_at IS NULL OR "grant".expires_at > sqlc.arg(now))), 0) AS INTEGER) AS is_signed_in
FROM device
LEFT JOIN "grant" ON "grant".device_id = device.id AND "grant".kind = 'session'
WHERE device.user_id = sqlc.arg(user_id) AND device.removed_at IS NULL
GROUP BY device.id
ORDER BY last_used_at DESC, device.id DESC;

-- name: RenameDevice :exec
UPDATE device SET name = sqlc.arg(name) WHERE id = sqlc.arg(id);

-- name: SetDeviceAudioOutput :exec
-- How a device makes sound (D222): asked the first time it plays, changeable after.
UPDATE device SET audio_output = sqlc.arg(audio_output) WHERE id = sqlc.arg(id);

-- name: RemoveDevice :exec
-- A device revoked (D218 section 4): it leaves the list, and its sessions end.
UPDATE device SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id);

-- name: RevokeSessionsOfDevice :exec
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE device_id = sqlc.arg(device_id) AND kind = 'session' AND revoked_at IS NULL;

-- name: RevokeSessionsOfUserExceptDevice :exec
-- Signing out everywhere else: every session of the person's but those of the device asking.
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE user_id = sqlc.arg(user_id) AND kind = 'session' AND revoked_at IS NULL
  AND device_id IS NOT sqlc.arg(keep_device_id);

-- name: GetLiveDeviceOfUser :one
-- One of a person's devices, when it is theirs and has not been removed.
SELECT id, name, platform, audio_output FROM device
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND removed_at IS NULL;

-- name: RecordDeviceSignIn :exec
-- A device signed in again (D221): its platform as it says now, and when.
UPDATE device SET platform = sqlc.arg(platform), last_signed_in_at = sqlc.arg(signed_in_at) WHERE id = sqlc.arg(id);
