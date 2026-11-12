> **ABOUTME:** AirPlay from Safari and the iPhone: what it takes (a playlist the TV fetches itself, a
> grant lasting the film, a route the TV can reach, formats Apple's player takes), the spike that
> settles it, and the devices that test it. Read before scheduling the spike or touching the
> player's remote playback.

# AirPlay

- **Status:** SKETCH — 2026-11-04, findings added 2026-11-12; **the spike's lab built 2026-11-12**
  (X3, below), for the owner to try on staging. The requirements below are known from the research;
  what is open is named under "Open".
- **Depends on:** [cast-to-device.md](cast-to-device.md) (casting to one's own devices, the other
  route to a TV), [lan-origin-2026-11-01.md](lan-origin-2026-11-01.md) (the LAN name the TV reaches
  at home).
- **Governing decisions:**
  [D154](../../decisions/working-notes.md#d154--one-grant-table-opaque-and-revocable-the-session-credential-never-appears-in-a-url)
  §6 (no credential in a URL), which this plan proposes to amend narrowly.
- **Research:** [casting-review-2026-10-25.md](../research/casting-review-2026-10-25.md) §3.
- **Size:** M (closed-beta.md's B5 and inventory B-11 said S, before the research found the
  grant and the playlist the TV must fetch itself).

## Why it is not free

With AirPlay the sender hands the TV a URL, and the TV fetches the playlists and chunks itself for
the whole film. It runs none of our code: no service worker, no WebRTC, no Connect relay, no token
renewal, no subtitle renderer.

Our player feeds the video element through hls.js over Media Source Extensions (ManagedMediaSource
on an iPhone, `web/packages/player/src/hls.ts`). A stream fed that way cannot be sent to an AirPlay
TV, only mirrored. Safari's way out is a second, plain HLS source on the element, whose URL it gives
the TV when the person chooses AirPlay; without one, remote playback stays off.

## What it needs

1. **A playlist URL the TV fetches itself.** The core serves the master playlist, the renditions'
   playlists and the chunks over plain HTTPS, with relative URLs in every playlist so the grant
   in the path follows. The player adds it as the element's alternative source.
2. **A route the TV reaches.** At home, the core's LAN name and its certificate (D396). Away from
   home, only the core's public address; a core reached only through Connect's relay cannot AirPlay.
3. **A grant that lasts the film** (casting-review §3):
   - one item, all its renditions, since the TV's player picks the rendition itself;
   - valid for much longer than the film, since the person may pause it (the owner, 2026-11-12:
     *"We'd need the lease to last much longer than the movie, because they could pause it. I don't
     believe the threat model is very big here. At best someone could sniff the URL and play the
     movie on another device."*), and revoked by the server when the person presses Stop; its length
     is the owner's to set;
   - in the path (`/g/<grant>/master.m3u8`), not the query string;
   - bound to the sender's member and session, listed in their devices while live, with a Revoke.

   This amends D154 §6 to: "No endpoint reads a token from a query string or path segment, except
   the media endpoints under `/g/`, which accept only a `link` grant", its test changed to assert
   that one exception. The amendment is the owner's decision, as a new D-number.
4. **Formats Apple's player takes:** H.264 or HEVC video, AAC or AC-3/E-AC-3 sound, fMP4 or TS
   chunks. Subtitles go in the playlist as WebVTT renditions, since our own rendering stays on the
   sender.
5. **The sender as remote:** play, pause and seek from the page drive the TV through the element;
   the page shows that it is playing on the TV, and Stop ends it and revokes the grant.

**Scope (casting-review's recommendation):** watching alone, at home. Not Watch Together: a TV
several seconds behind its buffer cannot be kept in step with the group well enough to offer.

## The spike (the owner schedules it)

About a day. From the iPhone to the owner's MacBook, one film, at home:

- does Safari offer AirPlay with the alternative source beside hls.js's ManagedMediaSource, and
  does it hand over the position;
- does the TV fetch through the LAN name with a path grant, and does the grant's revocation stop
  it;
- which of our renditions and sound tracks the receiver plays.

Its findings settle items 1 and 3 and go into the D154 amendment the owner decides.

**The lab, built 2026-11-12 (X3):** the lab's AirPlay page (`/lab/airplay`, Show debug tools) plays
a film here by hls.js, and AirPlays it two ways past hls.js's `disableRemotePlayback`: way A gives a
second element, AirPlay allowed, the grant's address at the position; way B lets hls.js go of the
page's element and gives it the grant fetched as the AirPlay menu opened, so the picker keeps the
press's gesture. Every step is logged. Stop ends both and revokes the grant, showing the answer.

- **The grant** is in the media's path, `https://<core>/api/v1/bytes/media/g/<grant>/master.m3u8`
  (POST `/lab/airplay/grants` with the playback's media token), its stream playlists and chunks
  under it by the playlists' relative URIs. It stands for that playback's film alone, for 24 hours,
  kept in memory as its hash (a restart revokes it), and DELETE `/lab/airplay/grants` (Stop) revokes
  the session's. A master or chunk not made yet is held up to 30 s under it, as AVPlayer does not
  honour the 425.
- **The gate** is the admin setting `shouldOfferAirPlayLab` (Diagnostics, Offer the AirPlay lab), off
  by default: off, every path of the lab answers not found, so D154 §6 is not widened beyond the lab.
  Staging's admin turns it on; production keeps it off.
- **Not settled by it:** the playback token's own idle expiry still ends a paused playback before
  the grant's day; the master is not filtered to the sound codecs Apple's player takes; the grant is
  not among the person's devices. The device check is VQ310.

## Testing devices

- **For the spike and the build:** the owner's MacBook (2018 or later, macOS 12 or newer) as the
  AirPlay receiver, and the iPhone or Safari on another Mac as the sender. That proves the hand-over,
  the grant, the playlist and the controls.
- **Before telling testers it works:** an Apple TV 4K (a used one does) or a TV with AirPlay 2
  built in, since households use those and their players differ from the Mac's on HEVC, sound
  passthrough and HDR.
- The sandbox cannot test AirPlay; every run is on the owner's devices.

## What the Chromecast work settled (2026-11-12)

The owner asked again what AirPlay would take, "similarly" to the Chromecast cast of D465 and D467.
What that work and a read of the code found:

- **No channel to hand a grant over.** D465's receiver is our code, so the grant goes to it over
  Cast's namespace and is traded once for the cast's credential. An AirPlay receiver is Apple's
  player and takes only a URL, so the grant can only be in the path (item 3), and it cannot be
  traded: the TV presents it on every playlist and chunk for the whole film. The D154 §6 amendment
  stays the owner's decision.
- **The route is proven at home.** The Chromecast reached the core's LAN name and its certificate
  directly (D467, on production). Apple's player is not a page, so D467's cross-origin rule does not
  even apply to it; it needs only the name. There is no WebRTC fallback without our code on the TV:
  a core reached only through Connect cannot AirPlay, as item 2 says.
- **Today's media URLs already carry the playback in the path** (`/bytes/media/chunks/{token}/…`,
  `internal/core/api/chunks.go`), with relative URIs in every playlist, but they also need a
  credential beside the token (an Authorization header or the media cookie, `mediaHolder` in
  `internal/core/api/media_cookie.go`), which the TV sends neither of. A path grant standing in for
  that credential, for one playback's chunks, is the core's change.
- **Our 425 does not suit Apple's player.** The core answers a chunk not made yet with 425 and a
  Retry-After, which our hls.js loader honours and AVPlayer does not. For a grant's requests the
  core holds the request until the chunk is made (the hold the chunk route already has, off for
  hls.js by D250 §5), and the master playlist waits for the film's grid the same way.
- **Copied sound may not play.** A copied track (DTS, TrueHD, Opus) is offered to hls.js as it is;
  the AirPlay master lists only AAC, AC-3 and E-AC-3 renditions.
- **hls.js turns AirPlay off itself** (1.7.3): attaching under ManagedMediaSource it sets
  `disableRemotePlayback` and removes every `<source>` child, so an alternative source added before
  it is gone. Either the player puts the alternative back after hls.js attaches, or, when the person
  chooses AirPlay in our Cast menu (D465 §3's magnifying glass and "AirPlay"), it leaves hls.js and
  gives the element the grant's URL as native HLS at the position, then opens Safari's picker
  (`webkitShowPlaybackTargetPicker`). The second needs the grant fetched before the press, as the
  menu opens, since the picker needs the press's gesture. The spike tries both.
- **Only Safari and iOS senders** (every iPhone browser is WebKit) can offer it; Chrome and Firefox
  on a computer cannot.

## Open

- Which of the two ways past hls.js's `disableRemotePlayback` works (the spike; see above): it does
  not leave room for the alternative source by itself.
- Whether the receiver's own controls (an Apple TV remote) reach the sender's page, so the
  position is recorded (the spike, then an Apple TV).
- How a core reached only through Connect is told apart, so the page does not offer AirPlay where
  the TV cannot reach it.
