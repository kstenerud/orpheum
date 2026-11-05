-- SPDX-License-Identifier: AGPL-3.0-or-later
-- persistent.db's invariants (D303, M7): each query names what must never be true and returns the
-- rows where it is, so a healthy database returns none. They run after every test that opens a
-- database through storetest, and on the running server at start and daily, which logs each one
-- broken as an error (cmd/orpheum's checkInvariants); the doctor is to run them too.
-- Each starts "-- invariant: <name>"; the text up to the next one is its query, which reads only.

-- invariant: no live version belongs to a removed title
SELECT version.id FROM version
JOIN item ON item.id = version.work_id
WHERE version.removed_at IS NULL AND item.removed_at IS NOT NULL AND EXISTS (
  SELECT 1 FROM file_filing CROSS JOIN place ON place.id = file_filing.file_id
  WHERE file_filing.version_id = version.id AND place.removed_at IS NULL);

-- invariant: every live title has a version
-- A pass makes a title for a file, and tombstones one it moves the last version out of or made and
-- did not use (identify's retireEmptied).
SELECT work.id FROM work
JOIN item ON item.id = work.id
WHERE item.removed_at IS NULL AND NOT EXISTS (SELECT 1 FROM version WHERE version.work_id = work.id);

-- invariant: every title and series is as new as its files say
-- found_at and found_file_modified_at, which newest first sorts by, are derived by step 0058's
-- triggers, on the item since step 0072, from the files; a write they missed leaves one out of date.
SELECT item.id FROM item
JOIN item_found ON item_found.item_id = item.id
WHERE item.kind != 'person' AND (item.found_at IS NOT item_found.found_at
  OR item.found_file_modified_at IS NOT item_found.found_file_modified_at);

-- invariant: no row meaning a file points at a file no place holds
-- A file goes with the last of its places (D426 section 4): what is kept of it, its tracks, its
-- probe, its NFO, its filings, what anyone watched of it and the subtitles fetched for it, goes
-- before it, and a confirmed copy's own is moved onto the file it is a copy of (copies.go's merge).
SELECT 'stream' AS kept, stream.id FROM stream
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = stream.bytes_file)
  OR NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = stream.video_file)
UNION ALL SELECT 'file_probe', file_id FROM file_probe
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = file_probe.file_id)
UNION ALL SELECT 'file_nfo', file_id FROM file_nfo
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = file_nfo.file_id)
UNION ALL SELECT 'file_filing', file_id FROM file_filing
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = file_filing.file_id)
UNION ALL SELECT 'play_position', file_id FROM play_position
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = play_position.file_id)
UNION ALL SELECT 'play_span', file_id FROM play_span
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = play_span.file_id)
UNION ALL SELECT 'play_event', file_id FROM play_event
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = play_event.file_id)
UNION ALL SELECT 'fetched_subtitle', id FROM fetched_subtitle
WHERE NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = fetched_subtitle.video_file);

-- invariant: a sidecar's place holds a sidecar, and the place it is attached to a video
-- What lies beside a video's place is that place's (D363 section 3): the file each place holds says
-- which it is (D426 section 4).
SELECT sidecar.id FROM place AS sidecar
JOIN place AS video ON video.id = sidecar.attached_to_place
WHERE NOT EXISTS (SELECT 1 FROM file WHERE file.id = sidecar.file_id AND file.sidecar_kind IS NOT NULL)
  OR NOT EXISTS (SELECT 1 FROM file WHERE file.id = video.file_id AND file.sidecar_kind IS NULL);

-- invariant: a group is its ordering's, and a season's poster its series' aired order's
-- A group is a row of one ordering (D426 section 5): a place in it is in that ordering, and a
-- season's poster names a group of its series' aired order (artwork's seasonGroup).
SELECT 'position' AS kept, position.work_id FROM position
JOIN grp ON grp.id = position.grp_id
WHERE grp.ordering_id <> position.ordering_id
UNION ALL SELECT 'artwork', artwork.id FROM artwork
JOIN grp ON grp.id = artwork.grp_id
JOIN ordering ON ordering.id = grp.ordering_id
WHERE ordering.sequence_id <> artwork.item_id OR ordering.name <> 'aired';
