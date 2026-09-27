-- SPDX-License-Identifier: GPL-3.0-or-later
-- A person's devices are listed by their owner (D218 section 4, D221): the reference scale's
-- test found the list reading every device, so the live devices are indexed by user.

CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;
