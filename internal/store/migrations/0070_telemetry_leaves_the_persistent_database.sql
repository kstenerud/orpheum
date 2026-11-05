-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- Telemetry leaves persistent.db (D426 section 9; sql.md section 13, M8): what the core and each node
-- used each minute, and what each player last said of its link, are kept in cache.db from now on.
-- A row written every minute meant the hourly backup never found persistent.db unchanged, and never
-- skipped one. Both are derived and short-lived, so their rows are not carried: cache.db starts them
-- anew, and a link is measured again at the next play.

DROP TABLE resource_use;
DROP TABLE link_memory;
