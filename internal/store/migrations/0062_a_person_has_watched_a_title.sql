-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D370 section 1: whether a person has watched a title is kept, per person and per title, not read
-- from where they are in its files now, so starting a finished film again does not unfinish it. A
-- title becomes watched when its person plays one of its files past finished_after_percent of a
-- known length, or to its end, and when they mark it so by hand; marked unwatched, it is as never
-- watched. A mark by hand is also a fresh start: the positions in its files watched before it are
-- no longer where it resumes (section 4).

CREATE TABLE title_watched (                                -- class A (D370)
  user_id    INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  title_id   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  -- When it became watched, by playing or by hand; NULL while it is not.
  watched_at INTEGER,
  -- When its person last marked it by hand, watched or unwatched; NULL for never. A position in one
  -- of its files watched at or before it is not where it resumes.
  marked_at  INTEGER,
  -- A row says something: watched, or marked unwatched.
  CHECK (watched_at IS NOT NULL OR marked_at IS NOT NULL),
  PRIMARY KEY (user_id, title_id)
) WITHOUT ROWID, STRICT;

-- What was finished until now stays finished: a position past the finished threshold of a known
-- length, as the lists, Continue Watching and Next Up read it until this step (the setting's value,
-- else its starting 90); a file played to its end ('completed'); and what deleted files left as
-- completed on their versions (D306 section 8). A film's, an episode's or an other video's; never an
-- extra's (D370 section 2).
INSERT INTO title_watched (user_id, title_id, watched_at)
SELECT finished.user_id, finished.title_id, max(finished.watched_at)
FROM (
  SELECT play_position.user_id, version.title_id, play_position.position_read_at AS watched_at
  FROM play_position
  JOIN file_probe ON file_probe.file_id = play_position.file_id AND file_probe.duration_ms > 0
  JOIN file_filing ON file_filing.file_id = play_position.file_id
  JOIN version ON version.id = file_filing.version_id
  WHERE play_position.position_ms * 100 > file_probe.duration_ms * coalesce(
    (SELECT CAST(setting.value_json AS INTEGER) FROM setting WHERE setting.key = 'finished_after_percent'), 90)
  UNION ALL
  SELECT play_event.user_id, version.title_id, play_event.happened_at
  FROM play_event
  JOIN file_filing ON file_filing.file_id = play_event.file_id
  JOIN version ON version.id = file_filing.version_id
  WHERE play_event.kind = 'completed'
  UNION ALL
  SELECT purged_watching.user_id, version.title_id, purged_watching.last_watched_at
  FROM purged_watching
  JOIN version ON version.id = purged_watching.version_id
  WHERE purged_watching.completed_count > 0
) AS finished
JOIN title ON title.id = finished.title_id AND title.kind IN ('movie', 'episode', 'other')
GROUP BY finished.user_id, finished.title_id;
