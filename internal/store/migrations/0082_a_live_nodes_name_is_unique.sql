-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A live node's name is unique (D432 section 6, the relocation audit's check 3): a hello finds its
-- node by name, so two live nodes of one name could not both be reached, and a rename could
-- collide. Enrolment finds or inserts in one write, so no database should hold two; should one,
-- the node a hello reaches, the earliest (GetLiveNodeByName), keeps the name, and each later one
-- is renamed "<name> (<id>)", kept with everything on it for an admin to see.
UPDATE node SET name = node.name || ' (' || node.id || ')'
WHERE node.removed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM node AS earlier
    WHERE earlier.name = node.name AND earlier.removed_at IS NULL AND earlier.id < node.id);

CREATE UNIQUE INDEX node_live_name ON node(name) WHERE removed_at IS NULL;
