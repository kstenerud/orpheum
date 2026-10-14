-- SPDX-License-Identifier: AGPL-3.0-or-later
-- discovery-core.md K7 (D292 section 4; the owner, 2026-10-14): a link's secret, sealed under
-- secret.key and bound to its grant, so that the person whose own link it becomes can be shown it
-- again on their page. The hashes beside it stay what a discovery finds the core by; this is never
-- sent anywhere but to the person. NULL for a grant that is no link's, and for a link made before
-- this step, whose secret the core never kept.

ALTER TABLE "grant" ADD COLUMN link_sealed BLOB;  -- the key's id, a nonce and the ciphertext of the link's secret (secrets.Store.SealBound); NULL as above
