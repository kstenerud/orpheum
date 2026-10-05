-- SPDX-License-Identifier: GPL-3.0-or-later
-- D247 section 10: each picture's timeline, its keyframes and its bytes in each second, which the
-- read that measures a video's loudness gathers too. Kept by payload hash beside loudness, not in
-- cache.db: making it again reads the whole file, which is not "only timing" (the architecture
-- map's F1, as subtitle_text and loudness).

CREATE TABLE video_timeline (                              -- class O: what the node read (D247 section 10)
  payload_hash BLOB NOT NULL,            -- the bytes it was read from, as file.payload_hash (D179)
  video_stream_index INTEGER NOT NULL,   -- the picture's stream, as ffprobe numbers every stream
  keyframes_ms_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its keyframes' times in ms, ascending: where a chunk can begin
  bytes_in_each_second_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its bytes in each second of the film, by the second
  failure     TEXT,                      -- FFmpeg's words when the file could not be read through; the JSON is then empty
  made_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, video_stream_index)
) WITHOUT ROWID;
