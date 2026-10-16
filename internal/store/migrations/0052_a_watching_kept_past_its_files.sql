-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D306 §8: what was watched follows the title and its edition, not the file, so a file removed long
-- enough ago can be deleted without taking what was watched of it. Before a removed file is
-- deleted, how often each person finished it and how long they watched it are added to the
-- version it was in; a version keeps its edition and moves with its title when titles merge.
CREATE TABLE purged_watching (                             -- class A (D306 section 8)
  user_id         INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  version_id      INTEGER NOT NULL REFERENCES version(id) ON DELETE RESTRICT,
  completed_count INTEGER NOT NULL CHECK (completed_count >= 0),  -- the 'completed' events of its deleted files
  watched_ms      INTEGER NOT NULL CHECK (watched_ms >= 0),       -- the time in their spans
  last_watched_at INTEGER NOT NULL,                               -- the latest of either
  PRIMARY KEY (user_id, version_id)
) WITHOUT ROWID, STRICT;
CREATE INDEX purged_watching_version ON purged_watching(version_id);
-- The removed files past their keeping are found by when they were removed, and what names one is
-- found by it, as a deleted parent's children are (D303, DL3).
CREATE INDEX file_removed_at ON file(removed_at) WHERE removed_at IS NOT NULL;
CREATE INDEX file_attached ON file(attached_to_file);
CREATE INDEX play_position_file ON play_position(file_id);
CREATE INDEX play_event_file ON play_event(file_id);
CREATE INDEX preference_scope ON preference(scope, scope_id);
