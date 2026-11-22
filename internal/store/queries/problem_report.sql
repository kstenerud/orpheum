-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The problem reports, each kept as gathered for the household's admin alone (D524 sections 2 to 4).

-- name: AddProblemReport :one
-- A report someone made, as gathered, with the file and sound offset it suggests, if any (D537).
INSERT INTO problem_report (user_id, made_at, version, words, gathered_json, sound_file_id, sound_offset_ms)
VALUES (sqlc.arg(user_id), sqlc.arg(made_at), sqlc.arg(version), sqlc.arg(words), sqlc.arg(gathered_json),
  sqlc.narg(sound_file_id), sqlc.narg(sound_offset_ms))
RETURNING id;

-- name: ListProblemReports :many
-- Every report made since a time (60 days ago: one older is deleted by the next sweep), the newest
-- first, without what was gathered: the System page's list. Who made it is named by their account's
-- name, NULL once the account is deleted. A report suggesting a file's sound offset says the file
-- and the offset (D537), the file NULL once it is deleted.
SELECT problem_report.id, problem_report.user_id, user.name AS user_name, problem_report.made_at,
  problem_report.version, problem_report.words, problem_report.sound_file_id, problem_report.sound_offset_ms
FROM problem_report
LEFT JOIN user ON user.id = problem_report.user_id
WHERE problem_report.made_at >= sqlc.arg(since)
ORDER BY problem_report.made_at DESC, problem_report.id DESC;

-- name: GetProblemReport :one
-- One report made since a time, with everything gathered, to render a download.
SELECT problem_report.id, problem_report.user_id, user.name AS user_name, problem_report.made_at,
  problem_report.version, problem_report.words, problem_report.gathered_json
FROM problem_report
LEFT JOIN user ON user.id = problem_report.user_id
WHERE problem_report.id = sqlc.arg(id) AND problem_report.made_at >= sqlc.arg(since);

-- name: CountProblemReportsByAfter :one
-- How many reports one person has made after a time: the rate a person may make them at.
SELECT count(*) FROM problem_report
WHERE user_id = sqlc.arg(user_id) AND made_at > sqlc.arg(after);

-- name: DeleteProblemReport :execrows
-- One report the admin deleted.
DELETE FROM problem_report WHERE id = sqlc.arg(id);

-- name: DeleteProblemReportsMadeBefore :execrows
-- The reports made before a time: those past their 60 days (D524 section 4).
DELETE FROM problem_report WHERE made_at < sqlc.arg(before);

-- name: ListProblemReportKnownNames :many
-- Every name in the database that can say whose a report is, of its kind, marked when it is an other
-- video's (D349 section 10): each title in each language, each library's name and root, each
-- person's, device's and node's name. A download looks for them in a report's free text (D524).
SELECT CAST('item' AS TEXT) AS kind, item_text.name AS value,
  CAST(item.kind = 'other' OR coalesce(library.kind, '') = 'other' AS INTEGER) AS is_other
FROM item_text
JOIN item ON item.id = item_text.item_id
LEFT JOIN library ON library.id = item.filing_library_id
UNION ALL
SELECT CAST('path' AS TEXT), library_root.root_path, CAST(library.kind = 'other' AS INTEGER)
FROM library_root
JOIN library ON library.id = library_root.library_id
UNION ALL
SELECT CAST('text' AS TEXT), library.name, CAST(library.kind = 'other' AS INTEGER) FROM library
UNION ALL
SELECT CAST('user' AS TEXT), user.name, CAST(0 AS INTEGER) FROM user
UNION ALL
SELECT CAST('device' AS TEXT), device.name, CAST(0 AS INTEGER) FROM device
UNION ALL
SELECT CAST('node' AS TEXT), node.name, CAST(0 AS INTEGER) FROM node;
