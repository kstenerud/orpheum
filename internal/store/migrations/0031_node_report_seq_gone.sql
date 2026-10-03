-- SPDX-License-Identifier: GPL-3.0-or-later
-- orpheum:raises-compatible-version
-- D240: a node posts its status on its own, and its report stream, whose records the core numbered
-- as it ingested them, is gone; so is the number a node continued from.

ALTER TABLE node DROP COLUMN last_ingested_report_seq;
