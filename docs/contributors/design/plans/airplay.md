> **ABOUTME:** AirPlay from Safari and the iPhone: what it takes (a playlist the TV fetches itself, a
> grant lasting the film, a route the TV can reach, formats Apple's player takes), the spike that
> settles it, and the devices that test it. Read before scheduling the spike or touching the
> player's remote playback.

# AirPlay

- **Status:** PART BUILT — 2026-11-04, findings added 2026-11-12, the spike's lab built 2026-11-12
  (X3, below); **D473's core built 2026-11-13**: the grant is the product's (POST and DELETE
  `/airplay/grants`, no lab setting), for a day, bound to the receiver's address, revoked at Stop, at
  a device's revoke and as its sign-in ends, and a request whose receiver went away is logged as gone
  (VQ315). The player plays on as the phone's page hides while its film is on an AirPlay receiver.
  **Not built yet:** the Cast menu's AirPlay entry, and the player's own hand-over to AirPlay and
  back; until then the lab (debug tools) is the way to try it. What is open is named under "Open".
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
   that one exception. The owner made the amendment as D473 (2026-11-13): a day's grant, bound to
   the TV's address (IPv4, or IPv6 by its /64), and the lab's setting goes.
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
  (POST `/airplay/grants` with the playback's media token, `/lab/airplay/grants` until D473), its
  stream playlists and chunks under it by the playlists' relative URIs. It stands for that
  playback's film alone, for 24 hours, kept in memory as its hash (a restart revokes it), and DELETE
  `/airplay/grants` (Stop) revokes the session's. A master or chunk not made yet is held up to 30 s under it, as AVPlayer does not
  honour the 425.
- **The gate** was the admin setting `shouldOfferAirPlayLab` (Diagnostics, Offer the AirPlay lab), off
  by default, until D473 made the grant the product's and removed it (2026-11-13).
- **Not settled by it:** the playback token's own idle expiry still ends a paused playback before
  the grant's day; the master is not filtered to the sound codecs Apple's player takes; the grant is
  not among the person's devices. The device check is VQ310.

**The lab's first passing run (2026-11-13, VQ310):** Way B, the page's own element given the grant
after hls.js lets go of it, AirPlays to a Mac at home: the Mac fetched everything under the grant
from the core's LAN name, all answered 200. Way A, a second element, did not: the iPhone played the
grant itself and the Mac asked for nothing. The player's AirPlay is therefore Way B's: the element
that plays hands the TV the grant. Two runs before it failed: the lab was off (every grant
answered not found), then the grant was on the client's origin, which serves no media.

**The lab's second run (2026-11-13, 11:40 to 12:01 UTC, the owner, iPhone to MacBook, Way B), read
beside the core's log of every request under the grant:**

- **What plays.** Five films, each with picture and sound on the Mac, playing, pausing and seeking
  from the iPhone: H.264 with DTS (film 27; its sound converted to AAC, its picture copied), H.264
  with AC-3 (films 8 and 13), MPEG-4 Part 2 with MP3 (film 33, both converted), and HEVC with E-AC-3
  (film 2, which the iPhone's playback took at an encoded H.264 quality, so a copied HEVC picture on
  the Mac is still untried). The Mac fetches far ahead: up to 200 chunks in 15 s.
- **The page going away and coming back.** With the iPhone locked, or Safari left for another app,
  the Mac played on; it stopped as Safari came back, on unlocking the iPhone or on returning to
  Safari. The lab does not pause a hidden page (the player's `pauseWhenHidden`, D287, is not on the
  lab's element), so the stop at Safari's return is Safari's or the page's own, not yet told apart. The player's AirPlay must not pause its element when the page is
  hidden while it plays on a wireless target, as D287 does today for a handheld.
- **A long pause** was not measured: both the iPhone and the Mac locked themselves while the film
  waited. The core's log shows the Mac's last request at 12:04:59 UTC; at about 12:12:57 the
  iPhone's own element fetched the grant's chunks itself and was answered 425 on q2000k540p and
  q3000k720p, so as AirPlay ended the element went on loading the grant's playlist on the phone. The
  player's AirPlay is to hand the element back to hls.js as `webkitCurrentPlaybackTargetIsWireless`
  goes false, and revoke the grant, so the phone does not (not built yet).
- **For the build (D473):** a request the client gives up on is answered and logged as an internal
  error (500, "a request's client went away … context canceled") and should be logged as gone, not
  as a fault; a chunk or init not made yet still came back 425 under the grant, twice to the Mac
  and repeatedly to the iPhone after about 1 to 12 s held, not after the 30 s hold; and the lab's
  film list stops after about 40 films, so most of the library cannot be picked there. Read from
  the log as D473 was built: the hold never ended early. Each of the 57 425s under the grant was
  logged as `chunkWaited` `playerGone`: the receiver gave up during the hold, and the relay then
  answered 425 to no one. Each 500 was the same, a client gone during a node's read. Both are now
  logged as gone (`clientGone`, 499, which reaches no one), not as answered.

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
