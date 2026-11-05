-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Browse (cut-a section 5, Browse): lists in a locale's order (D47), a title's detail and a
-- series' episodes. Search's three routes are in search.go: sqlc cannot parse FTS5's MATCH. Only what
-- a library lists is listed, a version with a live file at a place a live library of its kind covers
-- (version_library): a film whose files are all gone, or that no library covers, is kept, with its
-- history, but not shown. The check starts from the title's live versions, in that order by CROSS
-- JOIN: started from file, it read every file for every title, seconds at the reference scale
-- (audit-2026-09-25). Each list holds only the libraries its caller reaches: every one, or those in
-- library_ids_json (D243 section 4), and an item two folders, two libraries or two copies reach is
-- listed once (D363 section 6). A list searched holds, too, only the items its search matched: those
-- in item_ids_json, the ids search.go's matching statements found; NULL, a list not searched, holds
-- every item (one listing API with search a criterion of it, the owner 2026-10-30).

-- name: ListFilmsByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, item.year, item.identification_state, item.added_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'movie' AND item.removed_at IS NULL
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByAdded :many
-- Newest first: the scan that found it, then its file's own time (item.found_at, step 0058), by
-- item_kind_found from its top, stopping at its LIMIT; the row value is the page's keyset.
SELECT item.id AS item_id, item_text.name, item.year, item.identification_state, item.added_at,
  item.found_at, item.found_file_modified_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM item
LEFT JOIN item_text ON item_text.item_id = item.id AND item_text.is_original = 1
WHERE item.kind = 'movie' AND item.removed_at IS NULL
  AND (item.found_at, item.found_file_modified_at, item.id)
    < (sqlc.arg(before_found_at), sqlc.arg(before_modified_at), sqlc.arg(before_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY item.found_at DESC, item.found_file_modified_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, item.year, item.identification_state,
  item.added_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'series' AND item.removed_at IS NULL
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM sequence_member
    CROSS JOIN version ON version.work_id = sequence_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE sequence_member.sequence_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByAdded :many
-- Newest first: the scan that found its newest episode, then that episode's file's own time
-- (item.found_at, step 0058), by item_kind_found from its top, stopping at its LIMIT.
SELECT item.id AS item_id, item_text.name, item.year, item.identification_state, item.added_at,
  item.found_at, item.found_file_modified_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM item
LEFT JOIN item_text ON item_text.item_id = item.id AND item_text.is_original = 1
WHERE item.kind = 'series' AND item.removed_at IS NULL
  AND (item.found_at, item.found_file_modified_at, item.id)
    < (sqlc.arg(before_found_at), sqlc.arg(before_modified_at), sqlc.arg(before_id))
  AND EXISTS (
    SELECT 1 FROM sequence_member
    CROSS JOIN version ON version.work_id = sequence_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE sequence_member.sequence_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY item.found_at DESC, item.found_file_modified_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: GetWorkDetail :one
-- A title, with a failure of its own to be fetched and a person's refresh waiting (D231 section 4).
SELECT work.id, work.kind, item.year, work.runtime_ms, item.identification_state, item.followed_source, item.followed_source_id,
  item.added_at, item_text.name, item_text.overview, item_text.tagline,
  fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM work
JOIN item ON item.id = work.id
LEFT JOIN item_text ON item_text.item_id = work.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = work.id
LEFT JOIN refresh_request ON refresh_request.item_id = work.id
WHERE work.id = sqlc.arg(id) AND item.removed_at IS NULL;

-- name: ListWorkFiles :many
-- A title's versions and their live files, a version's parts in order, each file at each of its live
-- places a library of the title's kind covers that its caller reaches, its own place first (D363
-- section 8): a version none of whose files they reach is not listed, nor is a place they do not.
SELECT version.id AS version_id, file.id AS file_id, place.id AS place_id, place.path,
  file.size_bytes, file_filing.part_index
FROM version
CROSS JOIN file_filing ON file_filing.version_id = version.id
CROSS JOIN place AS own ON own.id = file_filing.file_id AND own.removed_at IS NULL
CROSS JOIN file ON file.id = own.id
CROSS JOIN place ON place.file_id = file.id AND place.removed_at IS NULL
WHERE version.work_id = sqlc.arg(work_id) AND version.removed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM library_place WHERE library_place.place_id = place.id
      AND library_place.library_kind = file_filing.library_kind
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR library_place.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT)))))
ORDER BY version.id, file_filing.part_index, file.id, place.id <> place.file_id, place.first_seen_at, place.id;

-- name: ListStreamsOfVideoFile :many
SELECT id, kind, codec, language, container_title, channel_count, disposition_flags
FROM stream
WHERE video_file = sqlc.arg(video_file)
ORDER BY kind, stream_index;

-- name: ListWorkExtras :many
SELECT extra_relation.extra_work, extra_relation.extra_type, item_text.name
FROM extra_relation
JOIN item ON item.id = extra_relation.extra_work
LEFT JOIN item_text ON item_text.item_id = extra_relation.extra_work AND item_text.is_original = 1
WHERE extra_relation.owner_work = sqlc.arg(owner_work) AND item.removed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = extra_relation.extra_work AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
ORDER BY extra_relation.extra_work;

-- name: GetEpisodePlace :one
-- An episode's series and its place in the aired order.
SELECT ordering.sequence_id, grp.number AS group_number, position.number_in_group, item_text.name AS series_name
FROM position
JOIN ordering ON ordering.id = position.ordering_id
LEFT JOIN grp ON grp.id = position.grp_id
LEFT JOIN item_text ON item_text.item_id = ordering.sequence_id AND item_text.is_original = 1
WHERE position.work_id = sqlc.arg(work_id) AND ordering.name = 'aired';

-- name: GetPlaceInOrdering :one
-- A member's place in one of its collection's named orderings (D96, D159): its group and number in
-- it, how many groups the ordering has, one for an ordering that is a single run (absolute), and its
-- group's name where the ordering names it, '' where it does not.
SELECT grp.number AS group_number, position.number_in_group,
  (SELECT count(DISTINCT coalesce(other_group.number, 0)) FROM position AS other
   LEFT JOIN grp AS other_group ON other_group.id = other.grp_id WHERE other.ordering_id = ordering.id)
    AS group_count,
  CAST(coalesce((
    SELECT group_text.name FROM group_text
    WHERE group_text.grp_id = position.grp_id AND group_text.name IS NOT NULL
    ORDER BY group_text.language LIMIT 1
  ), '') AS TEXT) AS group_name
FROM ordering
JOIN position ON position.ordering_id = ordering.id
LEFT JOIN grp ON grp.id = position.grp_id
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = sqlc.arg(ordering_name)
  AND position.work_id = sqlc.arg(work_id);

-- name: ListChosenOrderings :many
-- The ordering a person chose to watch each of some series in (D159 section 1), a preference at the
-- series' work scope, when the series still has it; a series they chose none for, or one it no
-- longer has, is not listed.
SELECT ordering.sequence_id, ordering.name
FROM json_each(CAST(sqlc.arg(sequence_ids_json) AS TEXT)) AS chosen
JOIN preference ON preference.user_id = sqlc.arg(user_id) AND preference.scope = 'item'
  AND preference.scope_id = chosen.value AND preference.axis = sqlc.arg(axis)
JOIN ordering ON ordering.sequence_id = preference.scope_id
  AND ordering.name = CAST(json_extract(preference.value_json, '$') AS TEXT);

-- name: GetSeriesDetail :one
-- A series, with a failure of its own to be fetched and a person's refresh waiting (D231 section 4).
SELECT sequence.id, item.year, item.identification_state, item.followed_source, item.followed_source_id,
  item_text.name, item_text.overview,
  fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sequence
JOIN item ON item.id = sequence.id
LEFT JOIN item_text ON item_text.item_id = sequence.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = sequence.id
LEFT JOIN refresh_request ON refresh_request.item_id = sequence.id
WHERE sequence.id = sqlc.arg(id) AND sequence.kind = 'series' AND item.removed_at IS NULL;

-- name: ListSeriesEpisodeDetails :many
-- A series' episodes in one of its named orderings (D159; the one its person watches in), each
-- with whether it has a live file.
SELECT position.work_id, grp.number AS group_number, position.number_in_group, item_text.name, item.year, work.runtime_ms,
  item.identification_state,
  EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = position.work_id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  ) AS available
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN work ON work.id = position.work_id
JOIN item ON item.id = work.id
LEFT JOIN item_text ON item_text.item_id = position.work_id AND item_text.is_original = 1
LEFT JOIN grp ON grp.id = position.grp_id
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = sqlc.arg(ordering_name) AND item.removed_at IS NULL
ORDER BY position.sort_order;

-- name: ListSeasonEpisodeDetails :many
-- A page of one season's episodes, a group of one of the series' named orderings (D159; the one its
-- person watches in), after the episode at after_sort_order, each with whether it has a live file;
-- aired's season 0 holds the specials and the episodes with no season.
SELECT position.work_id, grp.number AS group_number, position.number_in_group, position.sort_order, item_text.name,
  item.year, work.runtime_ms, item.identification_state,
  EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = position.work_id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  ) AS available
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN work ON work.id = position.work_id
JOIN item ON item.id = work.id
LEFT JOIN item_text ON item_text.item_id = position.work_id AND item_text.is_original = 1
LEFT JOIN grp ON grp.id = position.grp_id
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = sqlc.arg(ordering_name) AND item.removed_at IS NULL
  AND coalesce(grp.number, 0) = CAST(sqlc.arg(season_number) AS INTEGER)
  AND position.sort_order > sqlc.arg(after_sort_order)
ORDER BY position.sort_order
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesOrderings :many
-- A series' orderings, as its page offers them (episode-orderings.md section 4): aired first, then each
-- provider's by its label, then the household's own; each with its kind, its provider's label and
-- how many episodes the provider lists in it, how many of the series' live episodes it holds, and
-- whether it is a provider's group whose contents are not fetched yet.
SELECT ordering.name, ordering.kind, ordering.label, ordering.listed_count,
  (SELECT count(*) FROM position JOIN item ON item.id = position.work_id AND item.removed_at IS NULL
   WHERE position.ordering_id = ordering.id) AS episode_count,
  CAST(ordering.source_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM provider_response
    WHERE provider_response.item_id = ordering.sequence_id AND provider_response.provider = ordering.provider
      AND provider_response.part = 'episode_group/' || ordering.source_id
  ) AS INTEGER) AS is_awaiting_fetch
FROM ordering
WHERE ordering.sequence_id = sqlc.arg(sequence_id)
ORDER BY ordering.name <> 'aired', ordering.provider IS NULL, coalesce(ordering.label, ordering.name), ordering.id;

-- name: ListOrderingGroupNames :many
-- The names an ordering of a series gives its groups ("Disc 2", "Volume 3"), each group's in the first
-- language it has one in; a group it names none of is not listed.
SELECT grp.number AS group_number, CAST(group_text.name AS TEXT) AS name
FROM ordering
JOIN grp ON grp.ordering_id = ordering.id
JOIN group_text ON group_text.grp_id = grp.id AND group_text.name IS NOT NULL
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = sqlc.arg(ordering_name)
ORDER BY grp.number, group_text.language;

-- name: ListEpisodesNotInOrdering :many
-- A page of a series' episodes that one of its orderings leaves out, in aired order, which every
-- episode is in, after the one at after_sort_order, each at its aired place and with whether it has
-- a live file: what its page lists under "Not in this order" (episode-orderings.md section 7, Q4).
SELECT aired.work_id, aired_group.number AS group_number, aired.number_in_group, aired.sort_order, item_text.name,
  item.year, work.runtime_ms, item.identification_state,
  EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = aired.work_id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  ) AS available
FROM ordering AS aired_ordering
JOIN position AS aired ON aired.ordering_id = aired_ordering.id
JOIN work ON work.id = aired.work_id
JOIN item ON item.id = work.id
LEFT JOIN item_text ON item_text.item_id = aired.work_id AND item_text.is_original = 1
LEFT JOIN grp AS aired_group ON aired_group.id = aired.grp_id
WHERE aired_ordering.sequence_id = sqlc.arg(sequence_id) AND aired_ordering.name = 'aired'
  AND item.removed_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM ordering AS chosen JOIN position AS there ON there.ordering_id = chosen.id
    WHERE chosen.sequence_id = aired_ordering.sequence_id AND chosen.name = sqlc.arg(ordering_name)
      AND there.work_id = aired.work_id
  )
  AND aired.sort_order > sqlc.arg(after_sort_order)
ORDER BY aired.sort_order
LIMIT sqlc.arg(max_rows);

-- name: CountEpisodesNotInOrdering :one
-- How many of a series' live episodes one of its orderings leaves out.
SELECT count(*)
FROM ordering AS aired_ordering
JOIN position AS aired ON aired.ordering_id = aired_ordering.id
JOIN item ON item.id = aired.work_id AND item.removed_at IS NULL
WHERE aired_ordering.sequence_id = sqlc.arg(sequence_id) AND aired_ordering.name = 'aired'
  AND NOT EXISTS (
    SELECT 1 FROM ordering AS chosen JOIN position AS there ON there.ordering_id = chosen.id
    WHERE chosen.sequence_id = aired_ordering.sequence_id AND chosen.name = sqlc.arg(ordering_name)
      AND there.work_id = aired.work_id
  );

-- name: ListLiveFileLocationsOfWorks :many
-- Where each of some titles' live playable files is: each place its caller reaches, its node and the
-- path it reported, for whether any can be read now (D241 section 3, D363 section 8).
SELECT version.work_id AS item_id, place.node_id, place.reported_path_raw,
  CAST(file.payload_hash IS NULL AS INTEGER) AS is_unread
FROM version
CROSS JOIN version_library ON version_library.version_id = version.id
CROSS JOIN file ON file.id = version_library.file_id
CROSS JOIN place ON place.id = version_library.place_id
WHERE version.work_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND version.removed_at IS NULL AND file.sidecar_kind IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
ORDER BY version.work_id, place.id;

-- name: ListLiveFileLocationsOfSeries :many
-- Where each of some series' episodes' live playable files is, by the series: each place its caller
-- reaches.
SELECT ordering.sequence_id AS item_id, place.node_id, place.reported_path_raw,
  CAST(file.payload_hash IS NULL AS INTEGER) AS is_unread
FROM ordering
JOIN position ON position.ordering_id = ordering.id
JOIN version ON version.work_id = position.work_id AND version.removed_at IS NULL
CROSS JOIN version_library ON version_library.version_id = version.id
CROSS JOIN file ON file.id = version_library.file_id
CROSS JOIN place ON place.id = version_library.place_id
WHERE ordering.sequence_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND ordering.name = 'aired' AND file.sidecar_kind IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
ORDER BY ordering.sequence_id, place.id;

-- name: IsItemReached :one
-- Whether a library its caller reaches lists an item: a title one of whose versions it lists, or a
-- series one of whose episodes' it does (D243 section 4, D363 section 8).
SELECT CAST(EXISTS (
  SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
  WHERE version.work_id = sqlc.arg(item_id) AND version.removed_at IS NULL
    AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
) OR EXISTS (
  SELECT 1 FROM sequence_member
  CROSS JOIN version ON version.work_id = sequence_member.member_work
  CROSS JOIN version_library ON version_library.version_id = version.id
  WHERE sequence_member.sequence_id = sqlc.arg(item_id) AND version.removed_at IS NULL
    AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
) AS INTEGER) AS is_reached;

-- name: GetFilesWork :one
-- What a file is, for now playing and a cast's remote: its title, its kind and name when one is
-- filed, a film's or an episode's before an other video's (D363 section 5), its own place's path,
-- and how long it plays, 0 before it is probed.
SELECT place.path, work.id AS work_id, work.kind AS work_kind, item_text.name,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = file.id), 0) AS INTEGER)
    AS duration_ms
FROM file
CROSS JOIN place ON place.id = file.id
LEFT JOIN version ON version.id = (
  SELECT file_filing.version_id FROM file_filing WHERE file_filing.file_id = file.id AND file_filing.version_id IS NOT NULL
  ORDER BY file_filing.library_kind = 'other', file_filing.library_kind LIMIT 1)
LEFT JOIN work ON work.id = version.work_id
LEFT JOIN item_text ON item_text.item_id = work.id AND item_text.is_original = 1
WHERE file.id = sqlc.arg(id);

-- name: ListLibrariesOfFile :many
-- The live libraries that cover a live place of a file, of any kind: where a person who played it
-- may go back up to, of those they reach (D363 section 4).
-- Read from the file's places outward, by CROSS JOIN: its own and its copies', then what covers each.
SELECT DISTINCT library_place.library_id
FROM place
CROSS JOIN library_place ON library_place.place_id = place.id
WHERE place.file_id = CAST(sqlc.arg(file_id) AS INTEGER)
ORDER BY library_place.library_id;

-- name: ListUnreadFilesOfWork :many
-- A title's live videos its listing stored that are not read yet, by where each is, for a person
-- who opened its page to have them read first (D304 section 4).
SELECT place.id, place.node_id, place.reported_path_raw
FROM version
CROSS JOIN file_filing ON file_filing.version_id = version.id
CROSS JOIN place ON place.id = file_filing.file_id
CROSS JOIN file ON file.id = place.id
WHERE version.work_id = sqlc.arg(work_id) AND version.removed_at IS NULL AND place.removed_at IS NULL
  AND file.payload_hash IS NULL AND file.sidecar_kind IS NULL
ORDER BY place.id;

-- name: ListOrderingMemberFilesWithPositions :many
-- A collection's members in one of its named orderings (D159; 'aired' for a series today), each
-- live file of each with a person's position in it and when it was watched, and the file's length
-- where its probe knows it: where the person is in the sequence, for its Next Up (D160) and the
-- members a look at its page raises (D324 section 3). Each member's group, a series' season, 0 for
-- the specials and those with none, which a look at one season keeps to, and its number in it and
-- name, which its Next Up shows; and when the person watched it, NULL while they have not, and last
-- marked it by hand (D370). A member's files come as its page lists them, so the first is the one its
-- page plays first.
SELECT position.work_id, CAST(coalesce(grp.number, 0) AS INTEGER) AS group_number,
  CAST(coalesce(position.number_in_group, 0) AS INTEGER) AS number_in_group,
  CAST(coalesce(item_text.name, '') AS TEXT) AS name,
  own.id AS file_id,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = own.id), 0) AS INTEGER)
    AS duration_ms,
  play_position.position_ms, play_position.position_read_at, work_watched.watched_at, work_watched.marked_at
FROM ordering
JOIN position ON position.ordering_id = ordering.id
JOIN item ON item.id = position.work_id AND item.removed_at IS NULL
JOIN version ON version.work_id = position.work_id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id AND file_filing.filing IN ('listed', 'unidentified')
JOIN place AS own ON own.id = file_filing.file_id AND own.removed_at IS NULL
LEFT JOIN item_text ON item_text.item_id = position.work_id AND item_text.is_original = 1
LEFT JOIN grp ON grp.id = position.grp_id
LEFT JOIN play_position ON play_position.file_id = own.id AND play_position.user_id = sqlc.arg(user_id)
LEFT JOIN work_watched ON work_watched.user_id = sqlc.arg(user_id) AND work_watched.work_id = position.work_id
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = sqlc.arg(ordering_name)
  AND EXISTS (
    SELECT 1 FROM version_library WHERE version_library.version_id = version.id
      AND version_library.file_id = own.id
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT)))))
ORDER BY position.sort_order, version.id, file_filing.part_index, own.id;

-- name: CountOrderingMembers :many
-- How many members each of some series' orderings has with a live file, as
-- ListOrderingMemberFilesWithPositions lists them, and how many of them a person watched (D370
-- section 5): a series' episodes in the ordering its person watches it in, which its progress counts.
-- Each ordering is named [collection id, name]; the page's orderings drive, in this order, by CROSS
-- JOIN, and each member's mark is read by its key.
SELECT ordering.sequence_id, count(*) AS member_count,
  CAST(sum(EXISTS (
    SELECT 1 FROM work_watched WHERE work_watched.user_id = sqlc.arg(user_id)
      AND work_watched.work_id = position.work_id AND work_watched.watched_at IS NOT NULL
  )) AS INTEGER) AS watched_count
FROM json_each(CAST(sqlc.arg(orderings_json) AS TEXT)) AS named
CROSS JOIN ordering ON ordering.sequence_id = json_extract(named.value, '$[0]')
  AND ordering.name = json_extract(named.value, '$[1]')
CROSS JOIN position ON position.ordering_id = ordering.id
CROSS JOIN item ON item.id = position.work_id AND item.removed_at IS NULL
WHERE EXISTS (
  SELECT 1 FROM version CROSS JOIN file_filing ON file_filing.version_id = version.id
  CROSS JOIN version_library ON version_library.version_id = version.id AND version_library.file_id = file_filing.file_id
  WHERE version.work_id = position.work_id AND version.removed_at IS NULL
    AND file_filing.filing IN ('listed', 'unidentified')
    AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
)
GROUP BY ordering.sequence_id;

-- name: ListOtherByName :many
-- Other videos by name in the locale (D349), as films by name are.
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, item.identification_state,
  item.added_at, other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM sort_key
JOIN item ON item.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
LEFT JOIN other_video ON other_video.work_id = sort_key.item_id
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'other' AND item.removed_at IS NULL
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByAdded :many
-- Other videos newest added first, as films are (step 0058's found_at), by item_kind_found.
SELECT item.id AS item_id, item_text.name, item.identification_state, item.added_at,
  item.found_at, item.found_file_modified_at, other_video.recorded_at, other_video.recorded_from,
  other_video.file_name
FROM item
LEFT JOIN item_text ON item_text.item_id = item.id AND item_text.is_original = 1
LEFT JOIN other_video ON other_video.work_id = item.id
WHERE item.kind = 'other' AND item.removed_at IS NULL
  AND (item.found_at, item.found_file_modified_at, item.id)
    < (sqlc.arg(before_found_at), sqlc.arg(before_modified_at), sqlc.arg(before_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY item.found_at DESC, item.found_file_modified_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByRecorded :many
-- Other videos newest recorded first (D349 section 4), by other_video_recorded from its top: only an
-- other video has an other_video row.
SELECT item.id AS item_id, item_text.name, item.identification_state, item.added_at,
  other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM other_video
CROSS JOIN item ON item.id = other_video.work_id
LEFT JOIN item_text ON item_text.item_id = other_video.work_id AND item_text.is_original = 1
WHERE item.removed_at IS NULL
  AND (other_video.recorded_at, other_video.work_id) < (sqlc.arg(before_recorded_at), sqlc.arg(before_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY other_video.recorded_at DESC, other_video.work_id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByFileName :many
-- Other videos by their files' names in the locale (D349 section 4), by other_video_file_name.
SELECT item.id AS item_id, other_video.file_name_key, item_text.name, item.identification_state,
  item.added_at, other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM other_video
CROSS JOIN item ON item.id = other_video.work_id
LEFT JOIN item_text ON item_text.item_id = other_video.work_id AND item_text.is_original = 1
WHERE item.removed_at IS NULL
  AND (other_video.file_name_key, other_video.work_id)
    > (CAST(sqlc.arg(after_key) AS BLOB), CAST(sqlc.arg(after_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY other_video.file_name_key, other_video.work_id
LIMIT sqlc.arg(max_rows);

-- name: ListOtherVideosOfLibrary :many
-- An other library's videos with where their files are, for its folders (D349 section 5): each title
-- whose file the library covers, once, at the first of its places the library covers, by path (D363
-- section 6); one the pass has not named and dated yet waits for it.
SELECT work.id AS work_id, library_place.path, item_text.name, item.identification_state, item.added_at,
  other_video.recorded_at, other_video.recorded_from, other_video.file_name, other_video.file_name_key,
  item.found_at, item.found_file_modified_at
FROM library_place
CROSS JOIN file_filing ON file_filing.file_id = library_place.file_id
  AND file_filing.library_kind = library_place.library_kind
CROSS JOIN version ON version.id = file_filing.version_id AND version.removed_at IS NULL
CROSS JOIN work ON work.id = version.work_id
CROSS JOIN item ON item.id = work.id AND item.removed_at IS NULL
CROSS JOIN other_video ON other_video.work_id = work.id
JOIN item_text ON item_text.item_id = work.id AND item_text.is_original = 1
WHERE library_place.library_id = sqlc.arg(library_id)
  AND NOT EXISTS (
    SELECT 1 FROM library_place AS earlier
    WHERE earlier.file_id = library_place.file_id
      AND earlier.library_id = library_place.library_id
      AND (earlier.place_id <> earlier.file_id, earlier.first_seen_at, earlier.place_id)
        < (library_place.place_id <> library_place.file_id, library_place.first_seen_at, library_place.place_id))
ORDER BY library_place.path;

-- name: ListLibrariesOfItem :many
-- The live libraries that list an item, a title by one of its versions, a series by one of its
-- episodes' (D363 section 8), by name: the libraries a person goes up to from its page (D367
-- section 3).
SELECT library.id, library.name
FROM library
WHERE library.removed_at IS NULL AND library.id IN (
  SELECT version_library.library_id
  FROM version CROSS JOIN version_library ON version_library.version_id = version.id
  WHERE version.work_id = sqlc.arg(item_id) AND version.removed_at IS NULL
  UNION
  SELECT version_library.library_id
  FROM sequence_member
  CROSS JOIN version ON version.work_id = sequence_member.member_work
  CROSS JOIN version_library ON version_library.version_id = version.id
  WHERE sequence_member.sequence_id = sqlc.arg(item_id) AND version.removed_at IS NULL
)
ORDER BY library.name, library.id;

-- name: GetFilmOfExtra :one
-- The title an extra belongs to, the lowest where several hold it, with its name: the page an
-- extra's goes up to (D367 section 2).
SELECT extra_relation.owner_work, item_text.name
FROM extra_relation
JOIN item ON item.id = extra_relation.owner_work
LEFT JOIN item_text ON item_text.item_id = extra_relation.owner_work AND item_text.is_original = 1
WHERE extra_relation.extra_work = sqlc.arg(extra_work) AND item.removed_at IS NULL
ORDER BY extra_relation.owner_work
LIMIT 1;
