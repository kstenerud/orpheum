-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D305 §1: removing a person deletes their devices and credentials, so each play and credential
-- that names a device, and each credential derived from one deleted, is found by it, as a deleted
-- parent's children are (D303, DL3): with no index, deleting one device read every play of every
-- person.
CREATE INDEX play_event_device ON play_event(device_id);
CREATE INDEX play_position_device ON play_position(device_id);
CREATE INDEX play_span_device ON play_span(device_id);
-- A credential is found by its device whatever it holds, and a derived one by the grant it came
-- from (D177), now that a removed person's are deleted: grant_device covered only those with one.
DROP INDEX grant_device;
CREATE INDEX grant_device ON "grant"(device_id);
CREATE INDEX grant_parent ON "grant"(parent_secret_sha256);
