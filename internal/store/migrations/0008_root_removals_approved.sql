-- SPDX-License-Identifier: GPL-3.0-or-later
-- An admin's approval of a root's removals (D208 section 3). A scan that would remove what looks
-- like a lost disk waits; when the admin says the files really are gone, or that a disk mounted
-- inside the root is gone for good, the root's next complete scan removes what it does not see
-- without waiting again, and clears the approval. The approval is for a fresh scan, not for the
-- withheld one, whose unreadable regions were not kept.
-- Additive: an older binary ignores the column, so compatible_version stays where it is.

ALTER TABLE root_state ADD COLUMN removals_approved INTEGER NOT NULL DEFAULT 0
  CHECK (removals_approved IN (0, 1));   -- class A
