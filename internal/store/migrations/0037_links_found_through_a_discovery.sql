-- SPDX-License-Identifier: AGPL-3.0-or-later
-- discovery-core.md K7 (the discovery protocol's section 3.1; D292 section 4): what a discovery
-- finds the core by, kept so that neither reveals a secret. A link's grant keeps the hash the core
-- registers its secret by and the key an offer made through it is proven under; a session keeps its
-- device locator's hash. A used registration link becomes the person's own link, a grant of kind
-- 'link' and scope 'own', which needs no column of its own.

ALTER TABLE "grant" ADD COLUMN locate_hash BLOB;  -- SHA-256 over "orpheum-locate" and a link's secret, what the discovery locates by; NULL for a grant that is no link's
ALTER TABLE "grant" ADD COLUMN offer_key BLOB;    -- SHA-256 over "orpheum-offer" and a link's secret, what an offer through it is proven under; NULL as locate_hash
CREATE UNIQUE INDEX grant_locate_hash ON "grant"(locate_hash) WHERE locate_hash IS NOT NULL;

-- A device locator: what a signed-in device finds its core by at a discovery, a random token the
-- core hands it over the direct path and registers by its hash (C3, S11). One per session, a fresh
-- one replacing it on each visit; a session no longer live has none that counts.
CREATE TABLE locator (                                     -- class A (discovery-core.md K7)
  session_secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  hash        BLOB NOT NULL UNIQUE,      -- SHA-256 of the locator; never the locator
  issued_at   INTEGER NOT NULL
) WITHOUT ROWID;
