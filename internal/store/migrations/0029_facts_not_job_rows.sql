-- SPDX-License-Identifier: GPL-3.0-or-later
-- D231 sections 3, 4 and 7, and the architecture map's F1: what only the job queue held that is
-- the library's moves into library.db, so cache.db can be deleted at any moment at the cost of
-- timing alone. A film's or series' own failure to be fetched (fetch_failure) and a person's
-- request to fetch it again (refresh_request) are facts the fetch sweeps read. An image that
-- could not be stored keeps when and why, and whether its source said it was not there, from
-- which D213 section 1's one recheck is derived. A root an admin retired keeps when; a root, and
-- a disk mounted inside one, keep since when scans have found them missing, the "offline since"
-- an admin is reminded of (D41). The text made from a subtitle track moves here from cache.db,
-- which is not read by this step: a track is extracted again on its next play. The grant kind
-- 'invite' is 'issued_secret', which names what it covers, a registration invitation and a
-- device code; an older binary writes the old kind, so the step raises compatible_version.
-- orpheum:raises-compatible-version

-- A film's or series' own failure to be fetched, not its provider's being down (D231 section
-- 4): the fetch sweeps pass it over, its page shows it, and a person's refresh, which is "try
-- again", clears it, as a fetch that succeeds does.
CREATE TABLE fetch_failure (                               -- class F
  item_id   INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  failure   TEXT NOT NULL,               -- what the provider or the job said
  failed_at INTEGER NOT NULL
);

-- A person's request to fetch a film or series again (D231 section 4, D213 section 2): a sweep
-- queues its fetch until one that began after it succeeds, or fails as the item's own failure.
CREATE TABLE refresh_request (                             -- class A
  item_id      INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  requested_at INTEGER NOT NULL,
  -- Whether to ask again for the item's images that were missing or refused.
  should_fetch_images INTEGER NOT NULL CHECK (should_fetch_images IN (0, 1))
);

-- An image that could not be stored: when, why, and whether its source said it was not there,
-- a 404 that is rechecked once, a CDN's 404 lifetime later (D213 section 1). NULL on the images
-- refused before this step, which are not rechecked.
ALTER TABLE artwork ADD COLUMN refused_at INTEGER;
ALTER TABLE artwork ADD COLUMN refusal TEXT;
ALTER TABLE artwork ADD COLUMN is_missing_at_source INTEGER NOT NULL DEFAULT 0
  CHECK (is_missing_at_source IN (0, 1));

-- An admin retired the root (D231 section 7): its files are tombstones, with their history on
-- their titles (D173), its work is cancelled, and it is scanned no more.
ALTER TABLE library_root ADD COLUMN retired_at INTEGER;

-- Since when scans have found a root unavailable, and a disk inside one missing: the first scan
-- that did, kept until one finds it again (D41, D231 section 7).
ALTER TABLE root_state ADD COLUMN unavailable_since INTEGER;
ALTER TABLE root_mount ADD COLUMN missing_since INTEGER;
UPDATE root_state SET unavailable_since = last_scan_at WHERE last_scan_outcome <> 'complete';
UPDATE root_mount SET missing_since = (
  SELECT root_state.last_scan_at FROM root_state
  WHERE root_state.node_id = root_mount.node_id AND root_state.root_path = root_mount.root_path
) WHERE is_present = 0;

-- The text of a subtitle track, as the node made it (D217 section 6, D224 section 5): kept here
-- rather than in cache.db, since making it again reads a whole video (the architecture map's F1).
-- Kept by what it was made from: the payload hash of the file an embedded track's bytes are in,
-- as it was when extracted (D179), or the SHA-256 of a fetched subtitle's stored body_gzip. A row
-- outlives its file and serves the same bytes if they come back.
CREATE TABLE subtitle_text (                               -- class O: what the node made
  source_hash  BLOB NOT NULL,
  stream_index INTEGER NOT NULL,         -- the track's stream index in that file; 0 for a fetched subtitle
  ass          BLOB,                     -- for JASSUB; NULL when it could not be made
  vtt          BLOB,                     -- for the cast receiver; NULL when it could not be made
  failure      TEXT,                     -- why it could not be made: FFmpeg's exit and stderr tail
  made_at      INTEGER NOT NULL,
  PRIMARY KEY (source_hash, stream_index),
  CHECK ((failure IS NULL) = (ass IS NOT NULL AND vtt IS NOT NULL))
);

CREATE TABLE grant_new (                                   -- class A (D154)
  secret_sha256 BLOB PRIMARY KEY,        -- SHA-256 of the secret; never the secret
  -- issued_secret: a secret handed to a person to pass on, a registration invitation or a device
  -- code, which its scope names.
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','issued_secret')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent_secret_sha256 BLOB REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_used_at INTEGER,
  revoked_at  INTEGER,
  device_name TEXT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
) WITHOUT ROWID;
INSERT INTO grant_new (secret_sha256, kind, user_id, scope, parent_secret_sha256, issued_at, expires_at,
  last_used_at, revoked_at, device_name, device_id)
SELECT secret_sha256, CASE kind WHEN 'invite' THEN 'issued_secret' ELSE kind END, user_id, scope,
  parent_secret_sha256, issued_at, expires_at, last_used_at, revoked_at, device_name, device_id
FROM "grant";
DROP TABLE "grant";
ALTER TABLE grant_new RENAME TO "grant";
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;
