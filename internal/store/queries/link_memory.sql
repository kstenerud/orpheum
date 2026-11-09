-- SPDX-License-Identifier: AGPL-3.0-or-later
-- What each player last said of its link on each network (D257 section 1, D352 Q2), kept across
-- restarts (bandwidth-sampling-2026-10-29.md P2).

-- name: GetLinkMemory :one
SELECT speed_kbps, speed_log_spread, speed_at, steadiness, steadiness_at
FROM link_memory
WHERE player = ? AND network = ?;

-- name: UpsertLinkSpeed :exec
-- A player's speed on a network, with the spread of its samples, in place of the last.
INSERT INTO link_memory (player, network, speed_kbps, speed_log_spread, speed_at, said_at)
VALUES (sqlc.arg(player), sqlc.arg(network), sqlc.arg(speed_kbps), sqlc.arg(speed_log_spread),
  sqlc.arg(speed_at), sqlc.arg(speed_at))
ON CONFLICT (player, network) DO UPDATE SET speed_kbps = excluded.speed_kbps,
  speed_log_spread = excluded.speed_log_spread, speed_at = excluded.speed_at,
  said_at = max(link_memory.said_at, excluded.said_at)
-- Only when it changes something (D303, DL9).
WHERE (link_memory.speed_kbps, link_memory.speed_log_spread, link_memory.speed_at)
  IS NOT (excluded.speed_kbps, excluded.speed_log_spread, excluded.speed_at);

-- name: UpsertLinkSteadiness :exec
-- A player's steadiness level on a network, in place of the last.
INSERT INTO link_memory (player, network, steadiness, steadiness_at, said_at)
VALUES (sqlc.arg(player), sqlc.arg(network), sqlc.arg(steadiness), sqlc.arg(steadiness_at),
  sqlc.arg(steadiness_at))
ON CONFLICT (player, network) DO UPDATE SET steadiness = excluded.steadiness,
  steadiness_at = excluded.steadiness_at, said_at = max(link_memory.said_at, excluded.said_at)
-- Only when it changes something (D303, DL9).
WHERE (link_memory.steadiness, link_memory.steadiness_at)
  IS NOT (excluded.steadiness, excluded.steadiness_at);

-- name: DeleteLinkMemoriesSaidBefore :exec
-- Forgets every link said nothing of since a time: those no longer fresh (linkSpeedFreshHours).
DELETE FROM link_memory WHERE said_at < ?;
