-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (the access audit's A7): what waits for a person's match is counted by an index of the few
-- titles it can be. The count fetched every unidentified title, every unmatched episode among them,
-- to keep the handful of films with no source. CountWaitingForAPerson repeats this index's terms.
CREATE INDEX title_waiting ON title(id)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';
