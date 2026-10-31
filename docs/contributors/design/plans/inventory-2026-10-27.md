> **ABOUTME:** The inventory the owner asked for once the overlap batch was done: everything decided and
> not built, everything queued, every device check owed and every open owner question, by area, with
> its size and what it waits on, so the owner can decide what happens when.

# Inventory of the unbuilt and the queued (2026-10-27)

- **Status:** PLANNED — written 2026-10-27 at HEAD an earlier commit, after the overlap batch reached nas.example.
  The owner: *"Once the overlap batch is done I'll want an inventory of unimplemented features so I
  can decide"*, and *"Give me everything queued alongside the inventory when the time comes and I'll
  decide what happens when."* Nothing here is scheduled until the owner says so; the "next batch"
  marks are the lead's proposal (the standing orders' *Next batch*, filed 2026-10-27).
- **Depends on:** [closed-beta.md](closed-beta.md), the plans and decisions each row names.

**Read from:** every Status in the decision log; the lead's standing orders (the owner's queue,
kept in the lead's session, not in the repository); every plan's Status and its unbuilt steps;
[code-debt.md](../../code-debt.md); the owner-question sections of today's three reports
([navigation](../research/navigation-back-up-2026-10-27.md),
[widgets](../research/ui-widgets-audit-2026-10-27.md),
[screens and guest rejoin](../research/screens-and-guest-rejoin-2026-10-27.md)) and the owner's
answers to two of them ([navigation-and-widgets-answers-2026-10-27.md](navigation-and-widgets-answers-2026-10-27.md));
the unticked items of [verification-queue.md](../../verification-queue.md); `make handoff`; and
[cut-a.md](cut-a.md), [m5.md](m5.md) and [closed-beta.md](closed-beta.md) for what they put after
Cut A or the beta. Where a status line disagreed with the code, the code was read (a grep, a commit),
and the row says so (§13).

**How to read a row.** *State*: **decided** (decided, not built), **partly** (some of it built),
**queued** (asked for or filed, no decision written yet), **question** (waits on the owner's answer),
**device** (built, waits on a device check), **debt** (a code-debt entry). *Size*, a rough guess of
agent work: **S** under a day, **M** a day or two, **L** more. **NB** marks the lead's proposed next
batch.

## 1. Summary

| Area | Rows | Of them NB | Unticked VQ items | Rows waiting on an owner's answer |
| --- | --- | --- | --- | --- |
| Playback and the player | 20 | 2 | 11 | 0 |
| Casting and Watch Together | 18 | 1 | 17 | 5 |
| Libraries, scanning and matching | 18 | 3 | 3 | 1 |
| Navigation and UI widgets | 13 | 1 | 4 | 3 |
| Accounts, devices and remote access | 17 | 0 | 5 | 1 |
| Admin and diagnostics | 5 | 0 | 0 | 1 |
| Infrastructure, process and gates | 14 | 0 | 10 | 2 |
| Code debt | 27 | 0 | — | 6 |
| **Total** | **132** | **7** | **50** | **19** |

Plus the owner's own *Later*, *Maybe* and *Not wanted* lists for after the closed beta (§12), kept
as the owner marked them, and status lines in 12 decisions and 10 plans that no longer match the
code (§13).

### The proposed next batch (the standing orders, filed 2026-10-27)

1. **A play whose file is gone at one place falls through to the next place** (from VQ81) — a bug, S–M.
2. **A vanished library folder is marked unavailable** when its scan's listing is refused (from VQ81) — a bug, S.
3. **The top rung of the ladder plays the film's own sound, copied**; lower rungs the stereo — a quality bug, M.
4. **Casting as a session** ([D364](../../decisions/working-notes.md#d364--casting-is-a-session-with-a-device-while-it-lasts-whatever-the-person-plays-plays-there-and-the-sending-device-shows-it-in-the-tray)) — the owner's design of 2026-10-27, L.
5. **Our own player controls on computers**, replacing Chrome's, reachable by arrows — M–L; built on the widget audit's player set (row D-9).
6. **Library choices move to a Configure dialog** on each library's browse page — S–M.
7. **Up and Back everywhere** ([D367](../../decisions/working-notes.md#d367--up-goes-a-level-up-from-any-page-to-its-parent-back-goes-back-the-way-the-person-came), with the owner's answers of 2026-10-27) — M.

Items 3 to 7 share the player and the page shell, so the order matters: the widget set's player
steps (D-9) before items 4 and 5, which then build on one remote and one control bar.

### What the lead thinks most urgent or most valuable

Bugs that hurt use come first:

1. **Rows 1 and 2 of the next batch** (C-1, C-2): a film that exists at another place fails to
   play, and a library folder that is gone still says "complete". Both seen on staging by the owner.
2. **The top rung's sound** (A-1): a chunked play at the top quality re-encodes sound it could copy
   (nas.example's Chrome play heard "AAC stereo 96 kb/s, transcoded from AAC stereo"), and a 5.1 film
   loses its channels where the device could play them.
3. **A changed audio track restarts making chunks from 0:00** (H-12): the person waits longer after
   choosing a track or the boost, and the node encodes minutes nobody watches. Small; the owner's
   call because it gives `startsAtMs` a meaning.
4. **A folder retired by mistake cannot be added back** (H-17): the admin gets an internal error and
   must make a new library. Small.
5. **Two flaky tests** (H-10, H-20): each red run that a re-run clears costs a full gate run and
   trains everyone to re-run. Small.
6. **The pre-migration snapshot can be a partial copy** (H-9): the way back from a failed
   migration ([D181](../../decisions/working-notes.md#d181--the-migration-policy-two-integers-our-own-small-runner-a-snapshot-that-outlives-success-refuse-what-we-cannot-read-and-re-derive-rather-than-transform)) may be damaged after an interrupted start. Small, and it guards production's
   data.

Then the most valuable unbuilt features:

7. **Casting as a session and our own controls** (B-1, A-2): the owner's own asks; the TV is where
   the household watches.
8. **Next Up on Home, and intros and credits detected** (A-4, A-5): the closed beta's B1, "between
   episodes"; skip intro and the credits-timed Up next (A-6) wait on detection.
9. **Watched marks** (C-8): small, asked for, and every library list gains from it.
10. **Device checks waiting, some since September** (§10): 50 items, ten of them for things since
    retired or made optional (§13); a pass with the owner to tick, retarget or drop them shortens
    the list to what still tells us something.

## 2. Playback and the player

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| A-1 | **Top rung's own sound** (NB) | On the best quality the film's own sound is copied, 5.1 where the device plays it, instead of being re-encoded to 96 kb/s stereo; lower rungs stay stereo; one sound group per quality; and the play line stops saying `delivery=direct` on a chunked play | standing orders (owner, 2026-10-27, nas.example's Chrome play) | queued | M | a test of a quality switch's sound blip on devices |
| A-2 | **Our own player controls on computers** (NB) | The player draws its own controls (play, timeline, volume, tracks, full screen) instead of Chrome's, reachable by arrows; also gives subtitles their style in full screen ([D254](../../decisions/working-notes.md#d254--in-the-browsers-own-fullscreen-the-chosen-subtitle-track-is-shown-as-the-browsers-captions)'s "a fullscreen of our own", left for later) | standing orders (owner chose); [ui-widgets-audit](../research/ui-widgets-audit-2026-10-27.md) §4.2 | queued | M–L | the widget set's player steps (D-9); shares its control bar with B-1's remote |
| A-3 | **Night mode** | Narrowing the loudness range (quiet dialogue, loud explosions) beside the dialogue boost | [D336 §2](../../decisions/working-notes.md#d336--a-films-sound-plays-as-it-is-unless-the-device-cannot-play-it-or-the-person-asks-for-a-feature-that-needs-it-made-no-one-is-asked-how-their-device-makes-sound), [audio-presentation.md](audio-presentation.md); closed-beta B3 | decided | M | — |
| A-4 | **Next Up on Home** | A Home row of each series' next episode to watch (the series page already shows its own) | closed-beta B1; [play-state.md](play-state.md) | decided | S–M | — |
| A-5 | **Intros and credits detected** | Find each episode's intro and credits, from chapters first, then by matching sound across a season | [D82](../../decisions/working-notes.md#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance), [media-segments.md](media-segments.md); closed-beta B1 | decided | L | blocks A-6 and Up next's credits timing |
| A-6 | **Skip intro, recap and credits; Up next at the credits** | A Skip button over a detected segment; Up next appears at the credits instead of 30 s before the end | [D82](../../decisions/working-notes.md#d82--media-segments-are-typed-attach-at-the-version-and-carry-provenance), [D339](../../decisions/working-notes.md#d339--up-next-keeps-its-30-s-lead-until-credits-are-detected-a-run-of-three-autoplays-with-no-press-asks-still-watching-and-a-hosts-autoplay-moves-their-group-on) point 1, [up-next.md](up-next.md) §9 | decided | S | A-5 |
| A-7 | **Chapters in the player** | Jump between a film's chapters (the probe already reads them) | closed-beta B1 ("only if it comes easily") | decided | S | — |
| A-8 | **Scrub-bar thumbnails** | Small pictures over the timeline while seeking | [D102](../../decisions/working-notes.md#d102--the-nodes-work-is-a-pipeline-of-independently-deferrable-passes); closed-beta B1 (no plan file yet) | decided | M | work-scheduling's pieces (G-1) for making them in the background |
| A-9 | **Playback survey** | Measure what share of the library plays directly on the household's devices, to aim the format work | [playback-survey.md](playback-survey.md); closed-beta B3 ("first: it aims the rest") | decided | M | — |
| A-10 | **HDR shown right on an SDR screen** | Tone-map HDR films to SDR when the screen cannot show HDR | [transcoding.md](transcoding.md); closed-beta B3 ("scoped first; deferred if large") | decided | M–L | — |
| A-11 | **Caption style controls** | A person chooses the size, colour and background of subtitles | [D90](../../decisions/working-notes.md#d90--playback-presentation-is-a-persisted-per-user-preference-set-and-captions-have-a-statutory-floor), [D137](../../decisions/working-notes.md#d137--the-cast-receiver-ships-no-subtitle-renderer-casts-own-is-the-statutory-one-and-the-cost-is-on-the-server); closed-beta B3 | decided | M | A-2 for full screen |
| A-12 | **Language defaults per person; forced subtitles** | Each person's default audio and subtitle languages, and foreign-dialogue-only tracks turned on by themselves (today the choice follows a series, not a person) | [D24](../../decisions/working-notes.md#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series), [D138](../../decisions/working-notes.md#d138--the-ui-is-judged-on-click-depth-and-focus-latency-theming-is-a-seam-we-ship-or-one-they-force); closed-beta B3 | decided | M | — |
| A-13 | **Choosing among several GPUs** | A machine with two cards uses the one chosen | [D332](../../decisions/working-notes.md#d332--the-node-encodes-on-an-amd-card-through-vaapi-tried-after-quick-sync-and-nvenc) ("not built") | decided | S–M | — |
| A-14 | **A hidden tab's interest** | A tab hidden over 30 s stops raising work unless it plays; a finished background play does not autoplay; a controller whose receiver plays keeps its interest (owner's rules, 2026-10-26) | [D341](../../decisions/working-notes.md#d341--reserved-a-hidden-tabs-interest) (reserved); audit-2026-10-25 W2 | queued (decision to write) | M | — |
| A-15 | **ABR constants measured** | The player's quality controller's numbers (latch time, step-up, steadiness) measured on emulated links and the owner's devices | [D352](../../decisions/working-notes.md#d352--the-chunked-player-trusts-its-link-as-far-as-the-evidence-goes-its-first-seconds-latch-nothing-it-climbs-as-far-as-consistent-samples-carry-and-it-grows-steadier-when-it-swings-or-its-link-flakes) Status | partly | M | VQ24-style network shaping |
| A-16 | **TV Bro sound late on the TCL** | The sound ran about 0.5 s late on the Berlin TCL; a signed offset was built (2026-10-24) and waits for the owner's next visit with the 4-second sync clip | the lead's notes, [idiosyncrasies.md](../../idiosyncrasies.md) | device | — | the owner at the TCL; Diagnostics (F-1) |
| A-17 | Device checks: phones | Subtitles in a phone's full screen (VQ25); a playing film keeps the screen awake (VQ35); an iPhone's silent switch and levelled sound (VQ13); a measured film's loudness estimate falls (VQ50) | [verification-queue.md](../../verification-queue.md) | device | — | VQ13's second half tests a retired prepared copy (§13) |
| A-18 | Device checks: retired prepare | VQ14, VQ15 and VQ18 play or validate a whole-film prepared copy, which [D344](../../decisions/working-notes.md#d344--the-whole-film-prepare-is-retired-every-playback-the-device-cannot-play-as-it-is-plays-from-its-ladder-cut-on-time-where-its-keyframes-cannot-be-had) retired | verification-queue.md | device | — | to rewrite for the ladder, or drop (§13) |
| A-19 | Device checks: disc images | A DVD image, a one-clip Blu-ray and a multi-clip Blu-ray play (VQ73–VQ75) | [D350](../../decisions/working-notes.md#d350--a-disc-image-plays-its-main-title-read-by-the-node-from-the-images-own-structure-and-served-to-ffmpeg-through-fdproxy) | device | — | the owner's own decrypted images |
| A-20 | Device check: new device name | A Fire TV signed in afresh is named "Silk on Fire TV" (VQ84) | verification-queue.md | device | — | — |

## 3. Casting and Watch Together

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| B-1 | **Casting as a session** (NB) | Cast from the top bar; the receiving device asks once; while the session lasts whatever the person plays plays there; a tray icon on the sender; the remote a page of its own; one place for controls beside Watch Together | [D364](../../decisions/working-notes.md#d364--casting-is-a-session-with-a-device-while-it-lasts-whatever-the-person-plays-plays-there-and-the-sending-device-shows-it-in-the-tray); the owner's `CastIcon` in `status-tray.tsx` | decided | L | D-9's player set; settles B-2's step 8 and parts of B-4 |
| B-2 | **Cast: the person's choices sent to the receiver** | Subtitles, audio track, boost and volume chosen on the remote show on the TV | [cast-to-device.md](cast-to-device.md) §8 step 5 | decided | M | B-1 (the remote page it lives on) |
| B-3 | **Cast inside Watch Together** | A host casts mid-session and the group follows the TV; the phone keeps voice | cast-to-device.md §8 step 8; [D338](../../decisions/working-notes.md#d338--a-cast-goes-to-the-devices-tab-in-front-its-receiver-keeps-a-televisions-controls-and-leaves-by-back-or-a-double-ok-and-stop-casting-returns-it-to-the-page-it-was-on) ("hidden while hosting, fine for now"); [D364 §11](../../decisions/working-notes.md#d364--casting-is-a-session-with-a-device-while-it-lasts-whatever-the-person-plays-plays-there-and-the-sending-device-shows-it-in-the-tray) | decided, reshaped | L | B-1; [D364 §11](../../decisions/working-notes.md#d364--casting-is-a-session-with-a-device-while-it-lasts-whatever-the-person-plays-plays-there-and-the-sending-device-shows-it-in-the-tray) changes its shape |
| B-4 | **Watch Together and Cast as pages** | Leaving the Play page no longer stops the film for the group; the tray's icon goes to Watch Together's page, which gains a remote; Cast goes to the Casting page; the pop-up goes; a signed-in member's tray icon | [screens report](../research/screens-and-guest-rejoin-2026-10-27.md) §1.1, Q1–Q4 | question | M | the owner's Q1–Q4; one remote with B-1 |
| B-5 | **A guest's words for coming back** | The guest's page says how to come back; Leave says it is final; a used link in another browser says so plainly (the mechanism itself is built, [D267](../../decisions/working-notes.md#d267--a-watch-together-membership-is-kept-by-the-browser-that-joined-or-by-the-account-of-whoever-is-signed-in-and-one-member-may-be-on-several-devices)) | screens report §1.3, Q6 | question | S | — |
| B-6 | **"Still watching?" on a cast's receiver** | The receiver keeps counting autoplays because its presses are take-back presses | [up-next.md](up-next.md) §9 | decided | S | B-1 |
| B-7 | **A signed-in member's viewing recorded as theirs** | A signed-in guest's Watch Together viewing counts in their own Continue Watching | [D246](../../decisions/working-notes.md#d246--what-a-person-may-do-is-their-roles-an-invite-names-who-it-is-for-and-a-watch-together-viewer-is-an-ephemeral-account-that-sees-only-the-hosts-playback) §4, [D261](../../decisions/working-notes.md#d261--the-m4-audits-questions-answered) ("later by decision") | decided (later) | M | — |
| B-8 | **The host's media keys and lock screen move the group** | The host's keyboard and lock-screen controls act as the host's commands for everyone | [D268](../../decisions/working-notes.md#d268--a-watch-together-player-corrects-drift-as-its-device-measures-it-places-its-group-by-recent-pongs-starts-at-a-moment-the-server-names-and-its-hosts-moves-are-taken-at-the-controls) §4; closed-beta B1 | partly | M | media keys already reach the player as commands; whether they move the group is to be checked |
| B-9 | **Voice on a second device as the same person** | The film on the TV, the same person's voice on their phone | [D120](../../decisions/working-notes.md#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people); [watch-together.md](watch-together.md) "Open"; closed-beta B5 | decided | M | — |
| B-10 | **Google Cast receiver** | A receiver page registered with Google, so a Chromecast plays our films (today only a lab spike) | [D88](../../decisions/working-notes.md#d88--casting-is-remote-control-of-another-client-which-is-watch-together-with-a-group-of-one), [D135](../../decisions/working-notes.md#d135--casting-on-your-own-is-a-device-delegation-not-an-invitation-the-credential-is-the-session-and-the-queue-is-next-up); closed-beta B5; [tv-device-tests.md](tv-device-tests.md) | decided | L | VQ58 first |
| B-11 | **AirPlay from Safari** | Check that the video element's AirPlay works with our player | closed-beta B5 | decided | S | — |
| B-12 | **Takeover for this session only** | Cast to a device not signed in as the person (a friend's TV) | [D337](../../decisions/working-notes.md#d337--casting-starts-with-a-persons-own-signed-in-devices-one-chosen-from-those-connected-now-plays-the-film-the-one-that-cast-becomes-its-remote-and-either-may-take-control-back) ("later, not now") | decided (later) | M–L | B-1 |
| B-13 | **Speaker-side offset alignment** | The speakers' player runs ahead by the voice delay so leaks arrive in step; is it still wanted? | [D65](../../decisions/working-notes.md#d65--playback-offset-is-per-participant-and-it-is-an-echo-control); watch-together.md "Open" | question | M | — |
| B-14 | **Group rate and scrubbing as a suspension** | Protocol pieces the web client does not use: is a group playback rate wanted? | watch-together.md "Open" | question | S | — |
| B-15 | **Groups above two** | Whether anything changes at four or five people beyond [D250](../../decisions/working-notes.md#d250--a-watch-together-group-stops-only-at-its-own-sync-points-a-members-trouble-is-theirs-to-catch-up-on-from-what-is-already-being-made) | watch-together.md "Open"; the owner's *Later*: bigger groups | question | M | — |
| B-16 | Device checks: casting | Cast to a Fire TV and the TCL (VQ64), take control back (VQ65), the TV's own keys (VQ66), a laptop with no Orpheum tab in front (VQ71) | [D337](../../decisions/working-notes.md#d337--casting-starts-with-a-persons-own-signed-in-devices-one-chosen-from-those-connected-now-plays-the-film-the-one-that-cast-becomes-its-remote-and-either-may-take-control-back), [D338](../../decisions/working-notes.md#d338--a-cast-goes-to-the-devices-tab-in-front-its-receiver-keeps-a-televisions-controls-and-leaves-by-back-or-a-double-ok-and-stop-casting-returns-it-to-the-page-it-was-on) | device | — | — |
| B-17 | Device checks: Up next and groups | Up next on the TV (VQ67), on a cast (VQ68), Still watching (VQ69), the group follows the host into the next episode (VQ70) | [D339](../../decisions/working-notes.md#d339--up-next-keeps-its-30-s-lead-until-credits-are-detected-a-run-of-three-autoplays-with-no-press-asks-still-watching-and-a-hosts-autoplay-moves-their-group-on) | device | — | — |
| B-18 | Device checks: Watch Together | Voice across houses (VQ21, VQ23, VQ26), an iPhone in step (VQ22, VQ27), starts and jumps (VQ32, VQ34), the upload budget (VQ36, VQ37; VQ36 is [upload-budget.md](upload-budget.md)'s last step) | verification-queue.md | device | — | — |

## 4. Libraries, scanning and matching

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| C-1 | **A play falls through to the next place** (NB) | A film whose file is gone at one place (renamed, deleted) answers "not found" instead of trying the person's next place it is at | standing orders, from VQ81 | queued (bug) | S–M | — |
| C-2 | **A vanished folder is marked unavailable** (NB) | A library folder whose listing is refused ("no such file") is still marked "complete" | standing orders, from VQ81 | queued (bug) | S | — |
| C-3 | **Library choices in a Configure dialog** (NB) | "Show in Films/Series" and "show on Home" chosen per library from its browse page, the account page's two lists gone | standing orders (owner, 2026-10-27) | queued | S–M | the widget set's dialog (exists) |
| C-4 | **Fill in what is missing** | The regular sweeps fetch what a fetch left empty or a newer field needs; built for series' orderings only | [D335](../../decisions/working-notes.md#d335--what-should-have-been-fetched-and-was-not-is-filled-in-by-the-sweeps-a-refetch-that-replaces-is-the-persons-deliberate-act) §1; code-debt (frame stills wait on it) | partly | M | [D345](../../decisions/working-notes.md#d345--reserved-sweeps-as-job-kinds) (C-5) |
| C-5 | **Refetch and replace, deliberately** | A person's order on an item or library fetches its metadata anew and replaces it, optionally what they set by hand | [D335](../../decisions/working-notes.md#d335--what-should-have-been-fetched-and-was-not-is-filled-in-by-the-sweeps-a-refetch-that-replaces-is-the-persons-deliberate-act) §2 | decided | M | — |
| C-6 | **Sweeps as job kinds** | The heavy sweeps run as queued jobs under the queue's priorities instead of inline (audit C7; owner answered 2026-10-26) | [D345](../../decisions/working-notes.md#d345--reserved-sweeps-as-job-kinds) (reserved) | queued (decision to write) | M | after [D344](../../decisions/working-notes.md#d344--the-whole-film-prepare-is-retired-every-playback-the-device-cannot-play-as-it-is-plays-from-its-ladder-cut-on-time-where-its-keyframes-cannot-be-had) (done) |
| C-7 | **Genres** | Genres on a title's page and as a filter (cast, crew and ratings later) | [D95](../../decisions/working-notes.md#d95--people-are-entities-with-provider-identity-a-name-is-never-a-key); closed-beta B2 | decided | M | — |
| C-8 | **Watched marks** | A watched mark in the poster's tray, toggled by the person; a film marked watched leaves Continue Watching | closed-beta B2 (the owner's note) | decided | S | — |
| C-9 | **Browse filters** | Filters beyond the letter rail and the sorts [D366](../../decisions/working-notes.md#d366--every-sort-runs-either-way-films-and-series-also-sort-by-release-date-and-each-person-chooses-which-libraries-fill-their-home) added | closed-beta B2 | partly | S–M | C-7 |
| C-10 | **The watchlist** | "Watch later" (its tables exist) | [D140](../../decisions/working-notes.md#d140--five-gaps-the-sentiment-corpus-exposed-three-declines-and-one-that-undercuts-a-headline-claim) §3, [D150](../../decisions/working-notes.md#d150--read-the-withdrawals-not-just-the-requests-five-features-the-field-built-and-took-away) §3; closed-beta B2 | decided | S | — |
| C-11 | **Collections** | Box sets (a provider's collections, e.g. a film series) and a person's own collections; the tables exist, nothing fills or shows them | [D160](../../decisions/working-notes.md#d160--there-is-no-series-concept-a-sequence-is-what-a-collection-produces-and-next-up-is-a-query-over-any-of-them), [D159](../../decisions/working-notes.md#d159--d96-holds-under-the-new-lens-and-reaches-three-things-it-did-not-the-ordering-choice-is-a-preference-an-ordering-may-be-authored-and-a-multi-episode-file-is-a-boundary); the standing orders ("collections' UI, D160"); the owner's *Later* | decided (later) | M–L | — |
| C-12 | **Episode orderings, the rest** | A refresh refetches a series' groups and marks one no longer listed (step 5); an admin sets a series' files' order (step 6, [D210](../../decisions/working-notes.md#d210--a-series-has-an-on-disk-order-its-files-are-numbered-in-and-a-viewing-order-each-person-chooses-built-after-cut-a) §1); a person's own ordering (step 7) | [episode-orderings.md](episode-orderings.md) §6 | partly | M (5, 6), M (7) | three items "flagged for the owner" (§11) |
| C-13 | **Missing episodes** | List a series' episodes that are not in the library, as missing | closed-beta B2 | decided | S–M | — |
| C-14 | **Disc folders and multi-part films** | A disc's folder (VIDEO_TS, BDMV), an encrypted image, and `cd1`/`cd2` films (images are built) | [D306](../../decisions/working-notes.md#d306--the-scanner-is-tested-on-names-of-our-own-places-an-unnumbered-episode-by-a-guessed-title-match-and-keeps-watching-by-title-and-edition), [D350](../../decisions/working-notes.md#d350--a-disc-image-plays-its-main-title-read-by-the-node-from-the-images-own-structure-and-served-to-ffmpeg-through-fdproxy) §5 | decided (later) | M | the owner kept them out of the beta unless asked |
| C-15 | **Rescan to find missing media** | The core matches missing files by name across a node's roots, confirmed by hash | [D241](../../decisions/working-notes.md#d241--what-goes-missing-stays-listed-as-unavailable-until-an-admin-says-it-is-gone-and-a-play-of-it-fails-at-once) §8 ("later") | decided (later) | M | — |
| C-16 | **"Identified incorrectly" shows the file and folder names** | The correction view names the file and its folder, which may hold clues | the owner, 2026-10-20 (lead's notes) | queued | S | — |
| C-17 | Device checks: copies and the nas.example step | D363's copies on staging (VQ81, mostly passed; its two bugs are C-1, C-2); production's step 0060 with Continue Watching kept (VQ82) | [D363](../../decisions/working-notes.md#d363--libraries-may-overlap-a-file-is-found-once-at-every-place-it-is-and-each-list-shows-an-item-once-however-a-person-reaches-it) | device | — | — |
| C-18 | Device check: the Libraries page on a TV | The Libraries page by a Fire TV's remote (VQ72) | [D349](../../decisions/working-notes.md#d349--libraries-are-browsed-on-a-page-of-their-own-which-replaces-search-a-kind-other-holds-the-videos-that-are-never-identified-films-and-series-gather-every-library-of-theirs-but-those-a-person-leaves-out) | device | — | — |

## 5. Navigation and UI widgets

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| D-1 | **Up and Back everywhere** (NB) | Every page with a parent offers Up (episode → season, film → its page, video → its library folder, season → series, title → library, folder → folder above, library → Libraries); Back stays the browser's | [D367](../../decisions/working-notes.md#d367--up-goes-a-level-up-from-any-page-to-its-parent-back-goes-back-the-way-the-person-came) | decided | M | D-2's revision first |
| D-2 | **The Up decision revised** | The owner's answers: Up never adds history (back to the nearest parent behind, else replace); Up named by the parent ("‹ Show"); a trail on computers and phones ("Series › Show › S1E1 Pilot"); pages reached from anywhere show no Up | [navigation report](../research/navigation-back-up-2026-10-27.md) §6.4; [answers](navigation-and-widgets-answers-2026-10-27.md) | queued (decision to write) | S | blocks D-1 |
| D-3 | **One series page with a season picker** | Seasons folded into the series page; choosing a season replaces the history entry; coming back preselects the season of the episode left | answers, Navigation 1–2 | queued | M | with D-1 |
| D-4 | **Leaving Watch Together's page by Back keeps the session** | The tray leads back, as for casting | answers, Navigation 3 | queued | S | B-4 |
| D-5 | **The warning sign's page** | The sign leads to a "What's wrong" page per subject, with Up to the subject and a link to where it is fixed; Needs attention becomes its list; also signs owed on an episode, a person and a device, and a confirmation before a role is deleted ([D278](../../decisions/working-notes.md#d278--a-problem-is-shown-beside-what-it-affects-by-a-yellow-warning-sign-that-opens-what-is-wrong-yellow-means-something-needs-fixing-bright-red-that-an-action-cannot-be-undone)'s gaps) | answers, Widgets 2–3; screens report §1.2, Q5 | partly (asked; Q5 open) | S–M | — |
| D-6 | **Every top-bar pop-up is modal** | Clicking anywhere closes it without pressing what is under it | answers, Widgets 2 | queued | S | B-4 removes the Watch Together pop-up |
| D-7 | **One icon-button size** | The tray's buttons at 2.75 rem like the others, to be looked at on devices | answers, Widgets 1 | queued | S | — |
| D-8 | **Play as a button of its own colour; Approve removals asks first** | Play matches the other buttons in a special colour; "Remove what the last scan did not find" becomes a two-step confirm | answers, Widgets 4–5 | queued | S | D-9 step 6 |
| D-9 | **The standard widget set** | One component per kind in place of bespoke copies: the player set first (over-video colours, icon buttons, one timeline, one control bar), then a focusable chooser for the eleven native selects (track menus by remote), button variants, problem and loading lines, panels and layout | [ui-widgets-audit](../research/ui-widgets-audit-2026-10-27.md) §4.4 (8 steps) | question (scheduling) | L (steps 1–3: M) | steps 1–3 before A-2 and B-1 |
| D-10 | **The UI revamp** | One skinned interface over the finished features, a UI principles document, screens changeable alone; m5's questions 4 and 6–10 still open | [D313](../../decisions/working-notes.md#d313--the-closed-betas-milestones-joining-between-episodes-the-library-and-its-people-every-file-plays-well-the-ui-revamp-then-the-living-room); [m5.md](m5.md); closed-beta B4 | decided (waits) | L | waits until the features are in (the owner) |
| D-11 | **TV mode** | A layout for across the room in a TV's own browser (remote-control navigation on today's pages is built) | m5 Q5; closed-beta B5 | partly | M–L | D-10 |
| D-12 | **An info icon in the tray** | For news such as "an update is available"; a security update shows through the warning sign instead | the owner, 2026-10-19 (lead's notes) | queued | S | the update check (§9 *Later*) |
| D-13 | Device checks: TV and controller input | Silk spatial walk (VQ61), the 8BitDo controller (VQ62), the lab's input runs (VQ59, VQ60) | [tv-device-tests.md](tv-device-tests.md) | device | — | the navigation report §6.6 adds four more when D-1 is built |

## 6. Accounts, devices and remote access

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| E-1 | **The paid service made whole** | The probe, names and certificates, caps, the central client's integrity and kill switch, operations (connect.db's off-site backup, uptime checks) and lapse | [b0.md](b0.md) Phase F; orpheum-connect service.md M-S1–M-S4, M-S6; audit-2026-10-25 R5 | decided | L | before outside households |
| E-2 | **Terms, privacy policy, Impressum** | From the drafts | b0.md Phase F; service.md P13; [D187](../../decisions/working-notes.md#d187--users-are-responsible-for-what-they-play-we-never-supply-locate-unlock-or-promote-it--and-the-store-and-legal-constraints-that-follow) (legal review owed) | decided | ? (legal) | — |
| E-3 | **B0's acceptance run** | A fresh machine and an invitation, nothing else, to a film playing | b0.md Phase F | decided | M | E-1, E-2 |
| E-4 | **Bug reports from inside the app** | A report from the app reaches the operator with the logs that explain it, pseudonymised | [bug-reports.md](bug-reports.md); [D74](../../decisions/working-notes.md#d74--bug-reports-pseudonymise-rather-than-redact-and-fan-in-across-components); closed-beta B0 | decided | M | — |
| E-5 | **Where an activating device is** | "A TV in Germany asked 12 s ago" from Cloudflare's country header; the network's name waits for a database | [D322](../../decisions/working-notes.md#d322--activating-a-device-names-the-country-it-asks-from-as-cloudflare-names-it-the-networks-name-waits) | decided | S | — |
| E-6 | **Profiles** | Who's watching, with a PIN to switch | [D150](../../decisions/working-notes.md#d150--read-the-withdrawals-not-just-the-requests-five-features-the-field-built-and-took-away); closed-beta B2 (picker unplanned) | decided | M | — |
| E-7 | **A second factor on the core's own accounts** | TOTP or a passkey for a core account | closed-beta B2 (F47) | decided | M | — |
| E-8 | **Connections that heal on network changes** | ICE restart and network-change events | discovery-core K3b, K4; b0.md Phase E 2 | decided (deferred by the owner, 2026-10-20) | M | discovery-core open question 7 |
| E-9 | **The identity key's recovery code** | A sealed recovery for a server whose key is lost | [D292](../../decisions/working-notes.md#d292--the-third-audit-round-taken-no-admin-through-a-discovery-a-thin-path-first-two-machines-staging-inside-the-owners-own-names-the-household-zone-at-orpheum-corecom) §4 (K8a); b0.md Phase E 1 | decided (deferred until restores prove common) | M | — |
| E-10 | **Self-hosted discovery** | A household runs its own discovery on its own VPS (the command, its web serving, the conformance suite) | [discovery-mode.md](discovery-mode.md) W4b.3, W4b.5, W4b.8; [D334](../../decisions/working-notes.md#d334--a-tester-sets-up-a-core-from-one-command-at-a-terminal-which-asks-for-its-admin-and-if-they-have-one-its-key) | decided | L | before the open beta |
| E-11 | **A relay for connections that fail** | TURN through the service, only if the beta's count of failures says so | [D274](../../decisions/working-notes.md#d274--a-core-is-reached-first-by-what-needs-nothing-from-its-owner-webrtc-through-the-discovery-direct-paths-are-upgrades-and-a-relay-waits-for-demand), [D292](../../decisions/working-notes.md#d292--the-third-audit-round-taken-no-admin-through-a-discovery-a-thin-path-first-two-machines-staging-inside-the-owners-own-names-the-household-zone-at-orpheum-corecom) | decided (waits for the count) | L | the count (built) |
| E-12 | **Several cores per account; a switcher** | An account with more than one core, and a person on two cores | [D310](../../decisions/working-notes.md#d310--the-closed-beta-runs-on-the-paid-service-free-an-account-gives-one-key-for-one-core-the-cores-admin-enters-it-at-home-and-the-household-then-uses-the-services-app), [D291](../../decisions/working-notes.md#d291--what-the-discovery-audits-raised-and-d290-left-open-taken-as-the-audits-recommend-the-probe-on-a-machine-of-its-own-dns-on-bunny-ipv6-before-ipv4) (U8); the owner's *Later* | decided (later) | M | — |
| E-13 | **Passkeys for admin away from home** | An option the owner turns on, never required | [D294](../../decisions/working-notes.md#d294--admin-from-anywhere-in-the-beta-and-an-operators-reset-of-who-administers-from-the-servers-machine), [D297](../../decisions/working-notes.md#d297--a-household-is-found-by-its-realm-realm-name-and-password-together-proven-both-ways-by-a-key-the-password-makes-admin-only-from-the-cores-own-network) | decided (later) | M | — |
| E-14 | **WebRTC's remaining steps** | W2e, the rest of W4c, W5's measurement and W6 | [webrtc-transport.md](webrtc-transport.md) Status | partly | M | — |
| E-15 | **Gzip over WebRTC** | API answers over a data channel compressed too, once the SDK asks for it | [D342](../../decisions/working-notes.md#d342--api-answers-that-carry-no-credential-are-gzipped-an-answer-that-carries-one-never-is) ("a later change") | decided | S | — |
| E-16 | Device checks: remote reach | The VPS tunnel's TURN (VQ19) and rules (VQ47), a TV's QR activation (VQ48), Firefox's local-network prompts (VQ49), one connection per page (VQ63) | verification-queue.md | device | — | VQ19 and VQ47 test the optional tunnel (§13) |
| E-17 | **Device lifecycle questions** | Old devices pruned from "Your devices" | [D103](../../decisions/working-notes.md#d103--a-device-scoped-preference-is-stored-a-device-capability-never-is) (open since 2026-09) | question | S | — |

## 7. Admin and diagnostics

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| F-1 | **Diagnostics** | A page anyone can open (a guest too) that tests a device: a built-in sync clip the node makes, a sound-offset calibration, guided tests, reports kept for the household's admin only; it replaces the /lab pages | the owner, 2026-10-26; [playback-diagnostics research](../research/playback-diagnostics-2026-10-26.md) | queued (design sketch next; no code until agreed) | L | the owner's "before the beta, low priority" |
| F-2 | **Per-component log levels** | Log depth per subsystem, and the component on every service's lines (the core's single level is built) | [D347](../../decisions/working-notes.md#d347--the-cores-log-level-is-a-setting-an-admin-changes-while-it-runs-and-every-api-request-carries-an-id-into-its-log-lines-and-its-answer) ("later work") | decided | M | — |
| F-3 | **Waits shown to people** | The user-facing pieces of the scheduling scenarios: issues and badges with a daily email, bare entries and unreadable files, the HDR warning, the wait-or-play choice | [work-scheduling.md](work-scheduling.md) §16 (after phase 4) | decided | L | G-1 phases 2–4 |
| F-4 | **Playback failure over the video** | "Cannot be played now: the folder … is missing" shown over the video, not under it with the spinner turning | the owner, 2026-10-21 (lead's notes; not re-checked since) | queued | S | D-9 |
| F-5 | **An uncertain match's page shows the candidates** | The warning page of an uncertain match lists the candidates and why one was taken ([D365](../../decisions/working-notes.md#d365--among-a-films-candidates-the-one-whose-runtime-fits-the-file-is-preferred-a-year-either-side-of-the-names-then-the-most-voted) logs it) | screens report Q5 | question | S | D-5 |

The hardware playback matrix the owner asked for on 2026-10-23 is done: every machine's row passed
on 2026-10-24, nas.example's Quick Sync and the Mac's arm64 included (`deploy/hwtest/README.md`), and
VQ53 is ticked.

## 8. Infrastructure, process and gates

| # | Item | What it is | Source | State | Size | Depends on / blocks |
| --- | --- | --- | --- | --- | --- | --- |
| G-1 | **Work scheduling, phases 1–9** | Every media read through the node's disk scheduler (its decider is written and simulated, not wired); wants instead of numeric priorities; long work cut into pieces; playback admission and the uplink; quiet hours; the core's writer and provider tiers; observability; the client's side | [D329](../../decisions/working-notes.md#d329--work-is-scheduled-from-wants-each-resource-is-decided-next-to-it-playback-is-admitted-and-long-work-is-cut-into-pieces); [work-scheduling.md](work-scheduling.md) §16.2 | partly (phase 0, phase 1's decider) | L (weeks) | F-3; trick-play (A-8) |
| G-2 | **M4's close** | The full 30-minute acceptance under four network profiles, and its confirmation on the remote-household VM (VQ24) | [m4.md](m4.md) Status | device | M | nuc13's VM |
| G-3 | **The audit of 2026-10-25, what waits for the owner** | Design: the watched rung at enqueue time (C4/S6), a hub timer for waits (R2b), refused results as permanent (N7), a home for the scheduling loops (C14/G10), one waits endpoint or push (W4), the heartbeat (R13), the auth package split (R9); settings: node slot counts and pprof (N13), the probe rate (R8); gates: G3, G6, G7, G13, S7, S8, W12, W14; the pion upgrade (G12); three ad-hoc settings caches to delete | [audit-2026-10-25.md](../research/audit-2026-10-25.md) "Needs the owner" | question | S–M each | some gate items may have been settled in the 2026-10-26 process review |
| G-4 | **Release supply-chain audit** | Pin every dependency, transitive too, to an audited commit before the public release | the owner (lead's notes) | queued | L | before the public beta |
| G-5 | **CI's warmed development image** | A cache-warmed image on GHCR so CI starts fast | the owner, shelved until public-beta preparation | queued (shelved) | M | — |
| G-6 | **A private mount namespace on a bare install** | Closes a GPU encode's Unix socket to others on a bare (non-container) install | [D331](../../decisions/working-notes.md#d331--the-node-proves-itself-on-both-sockets-so-a-gpu-encode-may-open-a-unix-socket) ("not built") | decided | S–M | — |
| G-7 | **Native Windows build** | Six to ten weeks by the research, if testers ask | [D314](../../decisions/working-notes.md#d314--a-core-is-installed-from-a-public-image-on-any-machine-that-runs-docker-with-the-hardware-it-needs-and-with-no-tunnel-a-connection-that-needs-a-relay-fails-and-is-counted) | decided (later) | L | — |
| G-8 | **A `.deb`, and Mac or Windows development** | Deferred, not refused | [D235](../../decisions/working-notes.md#d235--development-and-deployment-run-in-containers-one-image-family-one-command-each-built-by-goreleaser) | decided (later) | M | — |
| G-9 | **Per-test coverage for wiring changes** | A recorded coverage run so revert-red can narrow a change to `NewServer` or `assembly.Build` (nightly or on demand: the owner's call) | code-debt (H-5) | debt | M | the owner's choice |
| G-10 | **Litestream for connect.db** | Continuous off-site backup of the paid service's database | audit-2026-10-25 R5; service.md P11 | decided | S–M | a dependency and credentials (the owner) |
| G-11 | Device checks: confinement and hardware on devnas | VQ9 (Landlock escape tests), VQ10 (Quick Sync), VQ16 and VQ17 (the dev image and disk facts) all name devnas, now retired | verification-queue.md | device | — | retarget to nas.example or the amd box, or drop (§13) |
| G-12 | Device check: OpenSubtitles on request | VQ11, search and fetch a subtitle on the dev loop | verification-queue.md | device | — | — |
| G-13 | Device checks: TV lab runs | Fire TV sound and WebRTC (VQ55), the phone remote's endurance (VQ56), extending to a TV from Windows (VQ57), our Cast receiver (VQ58) | tv-device-tests.md | device | — | B-10 waits on VQ58 |
| G-14 | **An optimisation sweep** | Once most features are in; first loads after a deploy pay one-off costs | the owner's *Later* (2026-10-18) | decided (later) | M–L | — |

## 9. Code debt

Every open entry of [code-debt.md](../../code-debt.md), in its order. Each is "debt"; *Size* is the
fix's.

| # | Debt | What it costs | Size | Owner's call? |
| --- | --- | --- | --- | --- |
| H-1 | The store's own tests insert raw SQL (8 files) | a schema change edits them by hand | M | — |
| H-2 | ~47 log assertions match English sentences | rewording a message breaks tests | M | — |
| H-3 | The subtitle fetch-on-request flow has no package tests | its changes are probed through the 311-test API package | M | — |
| H-4 | `NewExchanges` built by hand in 8 test files | a constructor change edits all of them | S | — |
| H-5 | Revert-red cannot narrow a wiring change | each such probe costs the whole API package | M | yes: nightly or on demand |
| H-6 | The locale probe trusts only literal keys | its probes run ~34 s | S | — |
| H-7 | `check_revert_red.py` at 796 of 800 lines | the next change must split it first (a gate file: the owner's) | S | yes (gate) |
| H-8 | `internal/core/api/errors.go` is 427 lines | harder to read | S | — |
| H-9 | The pre-migration snapshot trusts a leftover copy | an interrupted start may leave a damaged way back | S | — |
| H-10 | `listeners_test.go` dials a port another test may hold | a flaky `make check` | S | — |
| H-11 | `session_uses_test.go` sleeps 5 ms on the real clock | real time in a test | S | — |
| H-12 | A restarted playback starts its ladder at the stored position, not where the page goes on | slower restart after a track or boost change, and wasted encoding | S | yes: `startsAtMs` becomes more than a log's |
| H-13 | A given-up chunk is timed from its ask, a loaded one from its first byte | the link reads slower than it is | S | — |
| H-14 | A later move to WebRTC does not report its path pairs | no line says whether the house's LAN pair worked | S | — |
| H-15 | The hairpin warning and path counts live in memory | a restart clears a standing warning | S | yes: keep them in the store? |
| H-16 | "At home" is judged twice (`api` and `rtcapi`) | a change to the house's networks is made in both | S–M | — |
| H-17 | A retired folder cannot be added back to its library | an internal error; a new library needed | S | — |
| H-18 | The new-library form lists chosen folders flat | a long list on a node with many roots | S | — |
| H-19 | A learnt house network is written on every connection from home | a small file write per connection | S | — |
| H-20 | `lab-pad-input.test.tsx` failed once under load | a re-run-clears red | S | — |
| H-21 | A rescan's merge leaves a person's hidden progress on the old title | hidden progress shows again after a merge | S | — |
| H-22 | A rematched series keeps an ordering the new work lacks | episodes placed by a missing group | S | yes: [D357](../../decisions/working-notes.md#d357--a-rescan-re-runs-the-matcher-on-everything-a-person-did-not-set-per-item-per-library-and-for-all-of-them) §3 leaves what a person set |
| H-23 | `Matcher.Handle` reads `rematch` before every fetch | a read per fetch job | S | — |
| H-24 | Manage › Libraries is not marked on a library's own pages | the admin sees no place marked in the menu | S | — |
| H-25 | No `GET /libraries/{id}` | many libraries read for one page, every poll | S | — |
| H-26 | A frame still's read is not counted by the disk scheduler | a still's read beside a disk's other readers | S | if a disk is seen to suffer |
| H-27 | An ambiguous film gets no frame still until a person chooses | a blank tile meanwhile | S | yes: count ambiguity as tried? |

(The 28th entry, D335's passive sweeps, is row C-4.)

## 10. Device checks

The 50 unticked items of [verification-queue.md](../../verification-queue.md) sit in their areas'
rows above, so each is seen beside the feature it checks: A-17 to A-20, B-16 to B-18, C-17, C-18,
D-13, E-16, G-2 and G-11 to G-13. A-16 (the TCL's sound) has no VQ item; it waits on the owner's
next visit. Ten items test things since retired or made optional (§13, item 14).

## 11. Owner questions open

From today's reports and the plans; each is in its row above.

1. **Screens report Q1–Q6** (B-4, B-5, D-5, F-5): leaving the film no longer stops it for the
   group, and what does; no mini player for now; Cast and the tray's cast icon to the Casting page;
   a signed-in member's tray icon; the warning page per subject and whether it shows a match's
   candidates; the guest's words.
2. **The widget set's scheduling** (D-9): the owner answered its five questions; which steps run,
   and when, is the owner's.
3. **Episode orderings' flagged points** (C-12): showing how many episodes an ordering holds; the
   sweep's 20 series a run; one request per series where a provider's record cannot carry the list.
4. **Code debt's owner calls** (H-5, H-7, H-12, H-15, H-22, H-27).
5. **Watch Together's open points** (B-13, B-14, B-15).
6. **The 2026-10-25 audit's leftovers** (G-3).
7. **m5's questions 4 and 6–10** (D-10): a behaviour library; a specimen screen; a resume banner;
   poster sizes; artwork tinting; how screen independence is tested; where [D268](../../decisions/working-notes.md#d268--a-watch-together-player-corrects-drift-as-its-device-measures-it-places-its-group-by-recent-pongs-starts-at-a-moment-the-server-names-and-its-hosts-moves-are-taken-at-the-controls) §4 lands.
8. **Device lifecycle** (E-17).

## 12. After the closed beta: the owner's marks

From [closed-beta.md](closed-beta.md) "After the closed beta" (the owner, 2026-10-18), kept as the
owner marked them; none is scheduled. Sizes are all **M** to **L**.

- **Before the open beta:** the self-hosted discovery (E-10).
- **Revisited when the closed beta ends:** music.
- **Later:** cast, crew and ratings; collections (C-11); external lists; NFO write; the metadata
  editor; play statistics; parental controls and the rules engine; limits per role; the sharing
  page; scoped API keys; bigger Watch Together groups and reactions (B-15); bitmap subtitles (sooner
  if asked); fetched subtitles checked automatically; wider prepared outputs; an external player;
  audio description; playback speed; store apps for TVs; a Jellyfin-compatible API; DLNA; native
  phone apps; offline downloads (whose design [D344](../../decisions/working-notes.md#d344--the-whole-film-prepare-is-retired-every-playback-the-device-cannot-play-as-it-is-plays-from-its-ladder-cut-on-time-where-its-keyframes-cannot-be-had) changed: it was a prepared copy plus a lease);
  small player extras; a metrics endpoint; the update check (D-12); a second media node; several
  cores per account (E-12); billing; plugins; other languages; accessibility as a release gate; the
  dependency audit (G-4); an optimisation sweep (G-14).
- **Maybe:** OIDC or LDAP; recommendations; trailers; Trakt; Watch Together's text chat; each
  viewer's own subtitles and audio; our own relay (E-11).
- **Not wanted:** the API's compatibility gate for now; live TV; photos.
- **From decisions, also later:** role templates ([D246](../../decisions/working-notes.md#d246--what-a-person-may-do-is-their-roles-an-invite-names-who-it-is-for-and-a-watch-together-viewer-is-an-ephemeral-account-that-sees-only-the-hosts-playback)); sign-in with Google, Apple and others
  ([D316](../../decisions/working-notes.md#d316--the-services-accounts-hold-a-verified-e-mail-address-sign-in-by-it-or-the-username-and-a-forgotten-password-or-username-is-sent-to-it-a-reset-restores-the-account-at-once)); a light palette (dropped, [D271](../../decisions/working-notes.md#d271--m5s-first-questions-answered-four-browsers-no-user-facing-theming-or-light-palette-and-guests-who-may-pause-and-seek-unless-the-host-withholds-it)).

## 13. Duplicates, superseded items and out-of-date status lines

Listed once above; here is what folds into what, and which records disagree with the code.

**Out-of-date status lines** (a status to correct, once read back against the code):

1. [D346](../../decisions/working-notes.md#d346--a-watch-together-invitation-is-short-term-a-link-admits-for-an-hour-ending-the-session-ends-every-link-and-membership-and-a-session-idle-for-six-hours-is-ended) says "not built"; it was built in an earlier commit (short-lived links, idle sessions ended), with
   VQ69 and VQ70.
2. [D350](../../decisions/working-notes.md#d350--a-disc-image-plays-its-main-title-read-by-the-node-from-the-images-own-structure-and-served-to-ffmpeg-through-fdproxy) says "not built"; disc images were built in 1296a602 and wait on VQ73–VQ75. Disc folders
   and encrypted images remain (C-14).
3. [D210](../../decisions/working-notes.md#d210--a-series-has-an-on-disk-order-its-files-are-numbered-in-and-a-viewing-order-each-person-chooses-built-after-cut-a) says "not built"; its viewing order was built as episode orderings' steps 1–4. Its §1, a
   series' files' order, is C-12's step 6.
4. [D335](../../decisions/working-notes.md#d335--what-should-have-been-fetched-and-was-not-is-filled-in-by-the-sweeps-a-refetch-that-replaces-is-the-persons-deliberate-act) says "not built"; its §1 is built for series' orderings (`internal/core/identify/missing.go`).
5. [D330](../../decisions/working-notes.md#d330--ffmpeg-reads-its-input-through-the-node-over-a-protocol-of-our-own-patched-into-jellyfin-ffmpeg) says it waits on published patched builds; FFmpeg is now pinned from ffmpeg-fdproxy
   (`make ffmpeg`), and every machine of the hardware matrix logged reading films through the node
   on 2026-10-24. Its status predates the rename.
6. [D333](../../decisions/working-notes.md#d333--the-container-is-given-an-nvidia-cards-gpu-when-docker-can-hand-it-over) says the driver's route waits on VQ53, which was ticked on 2026-10-23.
7. [D340](../../decisions/working-notes.md#d340--a-videos-text-subtitle-tracks-are-extracted-by-one-job-from-one-read-of-it-and-a-failed-track-alone-is-tried-again-when-next-played), [D353](../../decisions/working-notes.md#d353--a-player-that-jumps-ahead-of-a-run-stops-it-when-waiting-for-the-run-would-cost-more-than-starting-a-new-one), [D354](../../decisions/working-notes.md#d354--a-webrtc-pair-that-goes-out-to-the-nat-both-ends-share-and-back-is-named-a-hairpin-warned-of-to-the-admin-and-never-counts-as-home), [D355](../../decisions/working-notes.md#d355--a-playbacks-picture-and-sound-are-each-wanted-from-a-position-of-their-own-moved-by-their-own-asks-and-together-by-a-seek-a-pause-a-resume-and-the-start), [D356](../../decisions/working-notes.md#d356--an-ipv4-only-cores-devices-at-home-are-recognised-by-the-houses-ipv6-networks-which-the-core-learns-from-devices-that-reached-it-directly-in-the-house-and-tells-its-discovery) and [D357](../../decisions/working-notes.md#d357--a-rescan-re-runs-the-matcher-on-everything-a-person-did-not-set-per-item-per-library-and-for-all-of-them) say only "Decided"; each has its feature
   commit and tests, and owes the read-back that turns its status to built.
8. [b0.md](b0.md) Phase B steps 3–5 carry no *Built* mark; passkeys, the operator's panel and
   invitations are built in orpheum-connect.
9. [closed-beta.md](closed-beta.md)'s tables predate what was built since: the countdown and the
   group following into the next episode (B1, [D339](../../decisions/working-notes.md#d339--up-next-keeps-its-30-s-lead-until-credits-are-detected-a-run-of-three-autoplays-with-no-press-asks-still-watching-and-a-hosts-autoplay-moves-their-group-on)), disc images (B2, [D350](../../decisions/working-notes.md#d350--a-disc-image-plays-its-main-title-read-by-the-node-from-the-images-own-structure-and-served-to-ffmpeg-through-fdproxy)), AMD encoding (B3,
   [D332](../../decisions/working-notes.md#d332--the-node-encodes-on-an-amd-card-through-vaapi-tried-after-quick-sync-and-nvenc)), HEVC played as it is (B3, `playable-formats.ts`), TV sign-in by code (B5, [D288](../../decisions/working-notes.md#d288--a-device-is-activated-by-the-code-it-shows-entered-on-a-device-already-signed-in-and-a-check-code-both-show),
   [D321](../../decisions/working-notes.md#d321--a-tv-shows-a-qr-code-carrying-its-keys-hash-which-activates-it-with-one-tap-the-typed-codes-check-code-is-committed)).
10. Plans still `PLANNED` though largely built in Cut A: [schema.md](schema.md),
   [migrations.md](migrations.md), [auth.md](auth.md), [web-client.md](web-client.md),
   [stream-service.md](stream-service.md), [play-state.md](play-state.md),
   [identification.md](identification.md), [scanner.md](scanner.md). Their unbuilt remainder is in
   closed-beta's *Later* list.

**Superseded or folded:**

11. **"Get this ready" and the session-start prepare** (closed-beta B5, cut-a M6): both made a
    whole-film prepared copy, which [D344](../../decisions/working-notes.md#d344--the-whole-film-prepare-is-retired-every-playback-the-device-cannot-play-as-it-is-plays-from-its-ladder-cut-on-time-where-its-keyframes-cannot-be-had) retired. Not listed as rows; if still wanted they need a
    new shape (chunks made ahead on the ladder).
12. **The database access-pattern audit** queued on 2026-10-05: done by the 2026-10-25 audit's store
    and core findings (S1–S5, C3–C6); its one leftover, the three ad-hoc settings caches, is in G-3.
13. **The subfolder picker** handed on from M4 to m5: built as [D359](../../decisions/working-notes.md#d359--a-librarys-folder-is-any-folder-beneath-one-of-its-nodes-configured-roots-chosen-by-browsing-the-nodes-folders).
14. **Device checks for retired or moved things:** VQ13 (its prepared half), VQ14, VQ15 and VQ18
    test the whole-film prepare [D344](../../decisions/working-notes.md#d344--the-whole-film-prepare-is-retired-every-playback-the-device-cannot-play-as-it-is-plays-from-its-ladder-cut-on-time-where-its-keyframes-cannot-be-had) retired; VQ9, VQ10, VQ16 and VQ17 name devnas, retired; VQ19
    and VQ47 test the VPS tunnel, optional since [D314](../../decisions/working-notes.md#d314--a-core-is-installed-from-a-public-image-on-any-machine-that-runs-docker-with-the-hardware-it-needs-and-with-no-tunnel-a-connection-that-needs-a-relay-fails-and-is-counted). Each wants retargeting or dropping.
15. **One remote, one control bar:** the widget audit's player set (D-9 steps 1–3), our own controls
    (A-2), D364's remote (B-1) and Watch Together's remote (B-4) are one piece of work in four
    places; built once, in that order.
16. **[D254](../../decisions/working-notes.md#d254--in-the-browsers-own-fullscreen-the-chosen-subtitle-track-is-shown-as-the-browsers-captions)'s "fullscreen of our own"** is part of A-2.
17. **Cast inside Watch Together** (B-3) is reshaped by [D364 §11](../../decisions/working-notes.md#d364--casting-is-a-session-with-a-device-while-it-lasts-whatever-the-person-plays-plays-there-and-the-sending-device-shows-it-in-the-tray); the plan's step 8 is built to
    D364, not to the plan.
18. **The earlier inventories**, [feature-inventory-2026-10-09.md](../research/feature-inventory-2026-10-09.md)
    and [milestone-reassessment-2026-10-18.md](../research/milestone-reassessment-2026-10-18.md),
    are history; this one is current.
