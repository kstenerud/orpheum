-- SPDX-License-Identifier: AGPL-3.0-or-later
-- An admin may let a node whose kernel lacks a confinement mechanism run FFmpeg without it (D450
-- section 1): a node that requires confinement and lacks it starts locked, doing no media work, until
-- an admin accepts the risk in the server's pages. The acceptance is the node's, by its id, so a node
-- enrolled anew under the same name is asked again; withdrawing it deletes the row, and the log keeps
-- who did each, and when.

CREATE TABLE node_unconfined_leave (                       -- class A: an admin's acceptance (D450)
  node_id             INTEGER NOT NULL PRIMARY KEY REFERENCES node(id) ON DELETE RESTRICT,
  accepted_by_user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  accepted_at         INTEGER NOT NULL
) STRICT;
