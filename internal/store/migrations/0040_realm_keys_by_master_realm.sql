-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D297 section 3: a realm sign-in lists the keys made for the master realm the core has
-- (ListLiveRealmKeys), which reads by master_realm; indexed, it reads that realm's people alone.

CREATE INDEX realm_key_master ON realm_key(master_realm);
