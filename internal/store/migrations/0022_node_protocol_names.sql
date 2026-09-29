-- SPDX-License-Identifier: GPL-3.0-or-later
-- The node protocol's names say what they hold (D227), and library.db keeps some of its JSON as
-- the node sent it: a file's probe, its error's detail, and a node's last status. Each is
-- rewritten with the new names, as step 0012 did, so the core reads the old rows as it reads new.

UPDATE file SET probe = replace(replace(replace(replace(probe,
    '"index":', '"streamIndex":'), '"language":', '"containerLanguage":'),
    '"profile":', '"codecProfile":'), '"channels":', '"channelCount":')
WHERE probe IS NOT NULL;
UPDATE file SET error_detail = replace(replace(error_detail,
    '"mode":', '"modeText":'), '"repeatCount":', '"occurrenceCount":')
WHERE error_detail IS NOT NULL;
UPDATE node SET status = replace(replace(replace(replace(replace(replace(replace(status,
    '"clockMs":', '"sentAt":'), '"doing":', '"activity":'), '"prepared":', '"preparedUsage":'),
    '"bytes":', '"usedBytes":'), '"diskPercent":', '"diskUsedPercent":'),
    '"diskLimitPercent":', '"diskUsedLimitPercent":'), '"freeBytes":', '"availableBytes":')
WHERE status IS NOT NULL;
