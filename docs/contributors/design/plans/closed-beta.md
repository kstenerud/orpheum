> **ABOUTME:** The closed beta's milestones B0–B5 (D313): what each holds, in what order, what each
> stands on, and the acceptance each ends with; then what is placed after the beta, what is a maybe,
> and what is not wanted. The plan that comes after Cut A's M0–M4.

# The closed beta

- **Status:** PLANNED — 2026-10-18, from [D313](../../decisions/working-notes.md#d313--the-closed-betas-milestones-joining-between-episodes-the-library-and-its-people-every-file-plays-well-the-ui-revamp-then-the-living-room),
  the owner's marks on every feature in the
  [milestone reassessment](../research/milestone-reassessment-2026-10-18.md), and D310–D312. Each
  milestone is phased, as [m4.md](m4.md) was, when it starts; the acceptance lines below are the
  agent's proposals, confirmed by the owner then.
- **Depends on:** [cut-a.md](cut-a.md) (M0–M4, built), [discovery-core.md](discovery-core.md),
  [discovery-mode.md](discovery-mode.md), [media-segments.md](media-segments.md),
  [play-state.md](play-state.md), [m5.md](m5.md), [tv-client.md](tv-client.md)

Who it is for (the owner, 2026-10-18): closed-beta households, each free for life (D310 §2), each
reaching its server through the paid service at app.orpheum-app.com (D310). B0 comes whole before
the first outside household, though the owner may open earlier as circumstances suggest. The UI
revamp waits until the features are in, and the TV comes last, "once we have the other features in
and can test properly". Music is left out until the closed beta ends.

Every phase, as in M4: behaviour changes carry tests that fail when reverted; a decision's Status
changes only once its clauses are read back against the code; `make check-pr` passes at each
phase's end.

## B0 — joining the beta

Phased in [b0.md](b0.md).

A household goes from an invitation to watching, alone. Nothing here is new to the owner's own
household; nothing after it reaches anyone else without it.

| Item | Source | Size | Notes |
| --- | --- | --- | --- |
| The service's accounts: username and password, sessions, the account's panel with its server and *Generate key* | D310 §3, §11, §13; orpheum-connect's service.md P2 | L | The password sign-in's own design is still to write: hashing, the breached-password check, throttling, the idle limit (D312 §6, §8, §9) |
| Passkeys for the service's accounts | D312 §7 | M | WebAuthn |
| The operator's panel and invitations | D310 §10, D312 §4 | M | Over WireGuard only, with a passkey; invitations as URLs and QR codes with notes |
| Enrolment by a one-use key | D310 §5, §8, §9; D311 §5; D312 §5 | M | 16 safe32 characters, an hour to use; a new key removes the old core |
| The accounts' safeguards | D312 §1–§3, §10–§17 | L | Re-confirmation and a record; the household told when its core changes; recovery done as recovery; timing-safe sign-in |
| Every code to one pattern | D311 | M | The core's invitations, Watch Together's links, device codes: what each made, and the escape hatch |
| A core that is very simple to install | D310 §5 | L | *Undesigned*: an image and steps a household follows without help, on a NAS, a mini PC or a VPS, needing no tunnel |
| The identity key's recovery code | D292 §4 (K8a) | M | |
| Connections that heal on network changes | discovery-core K3b, K4 | M | Replacing a failed connection is built (W2d); ICE restart and change events are not |
| The System page in plain words | discovery-core K12, K16 | M | First-run form, home address, counting where people get stuck |
| The paid service made whole | service.md M-S1–M-S4, M-S6 | L | Probe, names and certificates, caps, client integrity and kill switch, lapse |
| Bug reports from inside the app | [bug-reports.md](bug-reports.md); D74 | M | |
| Log levels changeable while running | the owner, 2026-10-15 | M | Design owed |
| Terms, privacy policy, Impressum | service.md P13 | ? | Drafts exist |

**Order:** the install and the service's accounts first, since everything else is reached through
them; then enrolment and the safeguards, which are one design; then the codes' pattern across the
core; then recovery, healing connections and the System page; bug reports and log levels whenever
a phase has room, as they help the rest be tested.

**Accepted when (proposed):** a household that has only an invitation link, on a machine it has not
set up before, gets from nothing to playing a film at app.orpheum-app.com with no help from the
operator. A second invitation for the same account hands it over and ends the first's sessions. A
new key removes the old core and the household's pages say so. A failed sign-in for an unknown
name, an unknown realm and a wrong password take the same time. A bug report from the app reaches
the operator with the logs that explain it.

## B1 — between episodes

| Item | Source | Size | Notes |
| --- | --- | --- | --- |
| Next Up: the Home row and a series' next episode | [play-state.md](play-state.md); cut-a §1 | S–M | |
| Intros and credits detected | [media-segments.md](media-segments.md); D82; the owner, 2026-10-18 | L | Chapters first, then audio fingerprints across a season |
| Skip intro, recap and credits | D82 | S | |
| The next-episode countdown, which a person can turn off | D82, D139; the owner's note | S | Autoplay is the countdown's ending, with its setting |
| Scrub-bar thumbnails (trickplay) | D102, D114 | M | No plan file yet |
| A Watch Together group carried from episode to episode | D121, D135 | M | Needs Next Up |
| The host's media keys and lock screen move the group | D268 §4 | M | |
| Chapters in the player | media-segments.md | S | Only if it comes easily, else the open beta (the owner) |

**Order:** Next Up, then the segment table and detection, then the controls that read it (skip,
countdown, chapters), then Watch Together's episode-to-episode, then thumbnails.

**Accepted when (proposed):** on a test season with known intro and credits times, detection finds
each within a second, and skip lands there. Watching an episode to its credits counts down and
plays the next, or does not, as the person's setting says. Next Up names the right episode after
watching out of order. A Watch Together group plays on into the next episode together.

## B2 — the library and its people

| Item | Source | Size | Notes |
| --- | --- | --- | --- |
| Genres on the title page and as a filter | D95 (the owner: genres first) | M | Cast, crew and ratings later |
| Browse filters and more sorts | the feature-standing audit | S–M | |
| Watched marks, and marking watched | the owner's note | S | One more icon in the poster's tray, which a person can toggle |
| The watchlist | D140 §3, D150 §3 | S | Its tables exist |
| Episode orderings | D96, D210 | M | |
| Missing episodes | D187 | S–M | |
| Disc images (ISO, VIDEO_TS) | D306 §4–§5 | M | Filed as not supported today |
| Profiles: who's watching, a PIN to switch | D150 | M | The profile picker is unplanned |
| A second factor on the core's own accounts | F47 | M | |

**Accepted when (proposed):** genres from TMDB filter the films page; a film marked watched shows
it in its tray and moves out of Continue Watching; a series in DVD order plays in that order; a
missing episode is listed as missing; an ISO plays; switching profile asks for its PIN; a core
account with a second factor cannot sign in by its password alone.

## B3 — every file plays well

| Item | Source | Size | Notes |
| --- | --- | --- | --- |
| The playback survey | [playback-survey.md](playback-survey.md) | M | First: it aims the rest |
| HDR tone mapping to SDR | transcoding.md | M | Scoped before it is committed to; deferred if large (the owner) |
| VAAPI and AMD encoding | transcoding.md | S–M | |
| Caption style controls | D90, D137 | M | |
| Language defaults per person; forced subtitles | D23, D24, D138 §7 | M | |
| Night mode | audio-presentation.md | M | Dynamic range compression, beside the dialogue boost |
| HEVC played as it is where the browser can | the owner's note | S–M | Prepares stay H.264 for the beta |

**Accepted when (proposed):** the survey's results are recorded and name the next format to support.
An HDR film on an SDR screen shows its test pattern's tones within a stated tolerance. A film plays
with its subtitles in the person's style and language without a choice. Night mode measurably
narrows the loudness range of a test film.

## B4 — the UI revamp

D245's M5 ([m5.md](m5.md)), over the features above: one skinned interface from the research
already done, a UI principles document, and an architecture that lets one screen change without the
rest. m5.md's open questions are answered when it starts.

## B5 — the living room

| Item | Source | Size | Notes |
| --- | --- | --- | --- |
| TV mode for a TV's own browser | m5.md Q5 | M–L | Remote-control navigation and a layout for across the room; Fire TV's browser already plays (the owner) |
| TV sign-in by a code the TV shows, through the paid service too | D93, D288; discovery-core K15 | M | |
| Google Cast | cut-a M6; D88, D135–D137; D226 §4 | L | A receiver page registered with Google, no app store |
| AirPlay from Safari | — | S | To be checked; the video element offers it |
| "Get this ready" and the session-start prepare | cut-a M6 | S | |
| Voice on a second device as the same person | watch-together.md; D120 | M | The film on the TV, voice on the phone |

**Accepted when (proposed):** cut-a's M6 acceptance, on the TV the beta's households actually use:
with the film on the TV and voice on the phone, a whole film completes with no manual
resynchronisation; a prepared rendition plays without stalling over the "congested" profile;
resuming at home continues from the remote position.

## After the closed beta

- **Before the open beta:** the discovery a household runs on its own VPS (D310 §7; possibly during
  the closed beta if a tester wants it).
- **Revisited when the closed beta ends:** music ([music-opensubsonic-2026-10-18.md](../research/music-opensubsonic-2026-10-18.md)).
- **Later** (the owner's marks): cast, crew and ratings; collections; external lists; NFO write;
  the metadata editor; play statistics; parental controls and the rules engine; limits per role;
  the sharing page; scoped API keys; bigger Watch Together groups and reactions; bitmap subtitles
  (sooner if people ask); fetched subtitles checked automatically; wider prepared outputs; an
  external player; audio description; playback speed; store apps for TVs; a Jellyfin-compatible
  API; DLNA; native phone apps; offline downloads; small player extras; a metrics endpoint; the
  update check; a second media node; several cores per account; billing; plugins; other languages;
  accessibility as a release gate; the dependency audit; and an optimisation sweep once most
  features are in (the owner, 2026-10-18: first loads after a deploy pay one-off costs, and the
  regular timings are acceptable).
- **Maybe:** OIDC or LDAP; recommendations; trailers; Trakt; Watch Together's text chat; each
  viewer's own subtitles and audio; our own relay.
- **Not wanted:** the API's compatibility gate for now (the owner: *"First we build out the
  functionality, then we can think about making an API"*); live TV; photos.
