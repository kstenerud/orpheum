> **ABOUTME:** AirPlay from Safari and the iPhone: what it takes (a playlist the TV fetches itself, a
> grant lasting the film, a route the TV can reach, formats Apple's player takes), the spike that
> settles it, and the devices that test it. Read before scheduling the spike or touching the
> player's remote playback.

# AirPlay

- **Status:** SKETCH — 2026-11-04. The owner asked what AirPlay needs and will schedule the spike
  later. The requirements below are known from the research; what is open is named under "Open".
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
   - valid for the film's length and a margin, and revoked by the server when the sender's playback
     ends or the person presses Stop;
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

## Testing devices

- **For the spike and the build:** the owner's MacBook (2018 or later, macOS 12 or newer) as the
  AirPlay receiver, and the iPhone or Safari on another Mac as the sender. That proves the hand-over,
  the grant, the playlist and the controls.
- **Before telling testers it works:** an Apple TV 4K (a used one does) or a TV with AirPlay 2
  built in, since households use those and their players differ from the Mac's on HEVC, sound
  passthrough and HDR.
- The sandbox cannot test AirPlay; every run is on the owner's devices.

## Open

- Whether hls.js leaves room for the alternative source under ManagedMediaSource, or the player
  must set it up itself (the spike).
- Whether the receiver's own controls (an Apple TV remote) reach the sender's page, so the
  position is recorded (the spike, then an Apple TV).
- How a core reached only through Connect is told apart, so the page does not offer AirPlay where
  the TV cannot reach it.
