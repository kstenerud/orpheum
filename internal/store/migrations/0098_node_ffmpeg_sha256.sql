-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The FFmpeg build a node last said in its hello that it runs, as the SHA-256 of its binary; NULL until
-- a hello says one (D535 section 9). A hello naming another build forgets the failures FFmpeg made of
-- the node's files, so the background passes try each again with the new one.
ALTER TABLE node ADD COLUMN ffmpeg_sha256 TEXT;
