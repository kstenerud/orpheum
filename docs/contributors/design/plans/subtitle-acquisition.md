> **ABOUTME:** Finding and fetching subtitles a file does not have — the empty case that
> [D11](../../decisions/working-notes.md)'s subtitle subsystem never covered, and the reason
> Bazarr exists.

# Subtitle acquisition

- **Status:** UNSPECIFIED — gap identified 2026-09-21 ([D140](../../decisions/working-notes.md)). The provider constraints are already researched ([tech-licensing.md](../research/tech-licensing.md), [tech-metadata.md](../research/tech-metadata.md)); nothing else is designed.
- **Depends on:** [auth.md](auth.md) (per-user provider credentials), [identification.md](identification.md) (what the file *is*), library-model.md (where a fetched track lives)

## Why this exists

[D11](../../decisions/working-notes.md) makes subtitles the project's headline claim — *"the
single largest defect area in the field"*, *"the cheapest place to be visibly better"* — and
everything built on it handles **subtitles that exist**: extraction at scan, font subsetting,
libass correctness, the overlay rendition, the statutory control set.

**A file with no subtitle track is an ordinary situation and nothing in the tree addresses
it.** That is a hole in the middle of the headline claim, and it is the reason **Bazarr**
exists as a separate program. The requests are on the trackers: *On-Demand Subtitle Search*
(+13) and *Downloadable Subtitles Like Plex in Jellyfin TV App* (+12) on `jellyfin-androidtv`.

## What is already known

The hardest constraint is researched and recorded in
[tech-licensing.md](../research/tech-licensing.md):

> **OpenSubtitles** — 5 downloads/day anonymous, 20/day with a free account, 1000/day VIP.
> *"A per-account budget. Must be the user's account."*

Three consequences fall straight out of that and shape everything else:

- **Credentials are per user, not per server.** That makes this an
  [auth.md](auth.md) concern before it is a subtitle concern, and it rhymes with
  [D13](../../decisions/working-notes.md)'s rule that provider API keys are the user's.
- **A rate budget is schema.** Counting downloads per user per day is a table, and a feature
  that silently exhausts someone's quota is worse than one that refuses.
- **Design for several providers** — [tech-metadata.md](../research/tech-metadata.md) already
  concludes this for metadata, and *"Bazarr — the tool everyone in this field uses"* is the
  evidence that one provider is not enough.

## The questions nobody has answered

- **What identifies the film to the provider?** OpenSubtitles matches on a **file hash** as
  well as on title. We have a sampling content hash
  ([D116](../../decisions/working-notes.md)) chosen for a completely different purpose and it
  is almost certainly **not** the hash they expect — so this needs its own, and whether that
  costs a second read of the file is a real question for the node pipeline
  ([D102](../../decisions/working-notes.md)'s nine passes, now ten).
- **Is a downloaded track actually in sync?** This is the failure everyone hits, and it is
  the one that would make the feature worse than nothing given
  [D11](../../decisions/working-notes.md)'s claim. Whether we can verify automatically — for
  instance by correlating speech boundaries against the audio the node has already decoded for
  [D115](../../decisions/working-notes.md)'s fingerprint — or must fall back to the user's
  offset control ([D90](../../decisions/working-notes.md)) is undecided and is the crux.
- **Automatic or on request?** Automatic fetching at scan multiplies the rate budget by the
  library; on-request is honest but means somebody discovers the gap at playback. Probably a
  per-library setting, which is the same shape as
  [D102](../../decisions/working-notes.md)'s per-pass deferral.
- **Where does the fetched file go?** A sidecar beside the media is what every other tool
  does and needs a writable path ([principles §2](../../principles/README.md)); the cache does
  not survive a rebuild. This is [D98](../../decisions/working-notes.md)'s question with a new
  subject, and a fetched subtitle is arguably *about the work*, so it may be the clearest
  sidecar candidate yet.
- **Who may do it?** A capability on the role ([D130](../../decisions/working-notes.md)) —
  a download consumes a shared budget and touches an external service.

## Done when

A film with no subtitles gains a correct one in the user's language without leaving the app; a
user's provider quota is visible before it is exhausted and refused cleanly afterwards; a
downloaded track that is out of sync is either corrected or flagged rather than silently
served; and nothing is fetched on behalf of a user who did not ask and holds no credential.
