-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A person's devices (schema.md, device; D217 section 8): made at sign-in, named by the client.

-- name: InsertDevice :one
INSERT INTO device (user_id, name, platform, added_at, last_signed_in_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(platform), sqlc.arg(signed_in_at), sqlc.arg(signed_in_at))
RETURNING id;

-- name: ListLiveDevicesOfUser :many
-- A person's devices, most recently used first: when each was last used, by the latest use of any
-- of its sessions, whether any is still live, and whether a live one a code activated (the device
-- scope), which the person's account page names (the device code audit's M4), and its sound offset,
-- which a Watch Together viewer signed in on it starts from. A guest receiver, a Google Cast device
-- signed in for one cast (D465 section 1), is no device of theirs to list.
SELECT device.id, device.name, device.platform, device.added_at, device.sound_offset_ms,
  CAST(coalesce(max("grant".last_used_at), device.added_at) AS INTEGER) AS last_used_at,
  CAST(coalesce(max("grant".revoked_at IS NULL AND ("grant".expires_at IS NULL OR "grant".expires_at > sqlc.arg(now))), 0) AS INTEGER) AS is_signed_in,
  CAST(coalesce(max("grant".scope = 'device' AND "grant".revoked_at IS NULL), 0) AS INTEGER) AS is_by_code
FROM device
LEFT JOIN "grant" ON "grant".device_id = device.id AND "grant".kind = 'session'
WHERE device.user_id = sqlc.arg(user_id) AND device.removed_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM "grant" AS guest
    WHERE guest.device_id = device.id AND guest.kind = 'session' AND guest.scope = 'guestReceiver'
  )
GROUP BY device.id
ORDER BY last_used_at DESC, device.id DESC;

-- name: ListLiveDevicesOfSessionScope :many
-- The devices not removed that a session of the scope was signed in on: the guest receivers a core
-- that stopped left kept (D465 section 1), to remove as it starts.
SELECT DISTINCT device.id, device.user_id FROM device
JOIN "grant" ON "grant".device_id = device.id AND "grant".kind = 'session'
WHERE "grant".scope = sqlc.arg(scope) AND device.removed_at IS NULL
ORDER BY device.id;

-- name: RenameDevice :exec
UPDATE device SET name = sqlc.arg(name) WHERE id = sqlc.arg(id);

-- name: SetDeviceSoundOffset :exec
-- How far a device's sound is moved from its picture, in ms: later when positive, earlier when
-- negative, for a device that plays one of them late; its person sets it there.
UPDATE device SET sound_offset_ms = sqlc.arg(sound_offset_ms) WHERE id = sqlc.arg(id);

-- name: RemoveDevice :exec
-- A device revoked (D218 section 4): it leaves the list, and its sessions end.
UPDATE device SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id);

-- name: RevokeCredentialsOfDevice :exec
-- A removed device's sessions and its device secret: it is neither signed in nor resumed again.
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE device_id = sqlc.arg(device_id) AND revoked_at IS NULL
  AND (kind = 'session' OR (kind = 'issued_secret' AND scope = 'deviceSecret'));

-- name: RevokeDeviceSecretsOfDevice :exec
-- The device secrets a device held before a sign-in issued it a new one (the third audit's A8).
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE device_id = sqlc.arg(device_id) AND kind = 'issued_secret' AND scope = 'deviceSecret'
  AND revoked_at IS NULL;

-- name: GetLiveDeviceOfUser :one
-- One of a person's devices, when it is theirs and has not been removed.
SELECT id, name, platform, sound_offset_ms FROM device
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND removed_at IS NULL;

-- name: RecordDeviceSignIn :exec
-- A device signed in again (D221): its platform as it says now, and when.
UPDATE device SET platform = sqlc.arg(platform), last_signed_in_at = sqlc.arg(signed_in_at) WHERE id = sqlc.arg(id);
