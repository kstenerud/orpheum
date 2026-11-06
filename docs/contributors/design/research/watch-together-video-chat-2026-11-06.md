> **ABOUTME:** Research, for the owner to decide, on adding video chat to Watch Together: the film
> synced as now, a small tile for each person, any of which a viewer can enlarge locally. Covers
> topologies and their bandwidth against a home uplink, self-hosted SFUs, whether it works without
> traffic through Orpheum's hosted service, products that do it, the UX and the platforms. Read
> before designing video in Watch Together or moving its voice off the mesh. Decides nothing.

# Video Chat in Watch Together

- **Date:** 2026-11-06.
- **Requested by** the owner, 2026-11-06: *"It would be great if watch together could also support
  video chat between participants. There are many video conferencing software companies and likely
  also open source products out there. I want to know what the possibilities are. I imagine having
  a main movie video that's synced between everyone, and then small picture-in-picture tiles for
  each participant, any of which a user can locally enlarge. This is not a feature we'd implement
  before beta, but solid research would be good."*
- **Constraint** from the owner, 2026-11-06: *"I'm not interested in putting this behind paid
  services. It would be provided in the standard orpheum package, and the paid service wouldn't
  bear the traffic. I may in future relax this if it's just not tenable without us hosting traffic,
  but I'd prefer to keep all but essential traffic off the paid service."* So video chat ships in
  the standard self-hosted package. Its media goes through no commercial video API and not
  through Orpheum's hosted service (the discovery, Connect). Signalling over the discovery is
  essential traffic and is allowed.
- **Marks:** **[V]** is verified at the cited source (§12) on the date given. **[R]** is from
  this repository (a decision, plan or earlier research). **[I]** is inference. Bandwidth figures
  are computed in §3 from the bitrates stated there.

## 0. Summary

1. **An SFU inside the household's core is the best fit for the constraint.** Build it on pion,
   which `deps/go.allow` already allows. Every participant becomes a client of a server the
   household already makes reachable for the film. Video then needs no relay that the film does
   not already need, and none of its traffic touches the paid service. [I, R]
2. **The film dominates the home uplink, not the tiles.** With the film at about 8 Mbit/s per
   remote member (D175) and 180p tiles, video adds 3–17% to what the core already sends at
   N = 2..8. Film alone passes a 40 Mbit/s uplink at six people. That is the existing "upload
   overload" question, and video makes it slightly worse. [I, from §3]
3. **Mesh is fine for two and tolerable for three or four.** That covers D69's first two
   priorities and D175's case. Each member uploads N−1 copies, and a pair that cannot connect
   directly needs a relay. Today that relay is the core's own TURN, so mesh with a relay costs
   the home line more than an SFU would. [I, V: mesh limit 4–6]
4. **Is it tenable without hosted traffic? Yes, under the SFU design.** The households that
   need a relay for video are exactly those that need one for the film: the core is behind CGNAT
   with no port-forward, and the member has no IPv6 and cannot hole-punch. That was 2–10% of
   remote viewing after one-time fixes (direct-reachability.md). Video adds no new failure class.
   If the owner ever relaxed the rule, relaying the film (about 7 GB per member per 2-hour film)
   would be the burden; the tiles and voice are about a tenth of that. [R, I]
5. **The UX is mostly CSS over the existing player.** Browsers give speaker detection,
   simulcast and encoded-frame E2EE today. The hard limits are elsewhere:
   - an iPhone cannot fullscreen an element, only the `<video>`, so tiles cannot overlay a
     fullscreen film there;
   - Safari has no Document Picture-in-Picture;
   - TVs mostly have no camera; Google withdrew Meet calling from Android TV in early 2025.
   The phone-as-camera for a TV, which Apple ships for FaceTime on Apple TV, is the right model
   and extends the split-devices design. [V, R]

## 1. What exists today [R]

- **Voice is a full WebRTC mesh between voice-role devices** (D281; plans/watch-together.md
  §Voice). Each page's voice device keeps one call with every other person's voice device.
  Per-person volume runs 0–4×. Echo cancellation and browser AGC are settings, off by default.
  Signalling rides the session WebSocket.
- **The relay is a pion/turn TURN server embedded in the binary** (`internal/core/turn`, D243
  §8, §10). It runs on 3478 UDP/TCP with relay ports 49160–49199. It relays only to the public
  internet and never into the house. Credentials come per member from `GET /together/{id}/ice`;
  the discovery's last 15 minutes. "Voice is WebRTC between the people, its fallback relay the core's
  own TURN or the owner's VPS, never the paid service's" (discovery-protocol.md §3.5).
- **The core already terminates WebRTC** (`internal/core/rtcapi`, pion/webrtc/v4, D273 §7) for
  the film's `webrtc` path. That path reaches a core behind carrier NAT when the client has no
  IPv6, using data channels, a pacer (D397) and an unused video m-line to enlarge Chrome's receive
  buffer.
- **Echo is handled in tiers** (D63, D65): headphones first, then a playback offset; D193 §4
  deleted content-referenced AEC. Voice on a bad link (D262): the film is capped at half the
  link while voice is live, downloads are paced and marked DSCP LE, voice gets RED and Opus FEC.
- **Split devices** (watch-together-split-devices-2026-10-24.md): the film plays on the TV, and
  voice and controls run on the phone. Open question 7 asks what to shed under upload overload.
- **Group sizes** (D69): two first, then 4–5, then a party on one network. **Sessions** (D346):
  a link admits for one hour, ending the session ends everything, six idle hours end it, and
  voice signalling is not activity.
- **Licences:** the server is AGPL-3.0 with a contributor agreement (D275); `web/` is MPL-2.0.

## 2. Topologies

| Topology | What each member sends | What each member receives | Server work | Fits |
| --- | --- | --- | --- | --- |
| **Mesh** (P2P) | its camera N−1 times, one encode per peer [I] | N−1 streams | none; a TURN relay for pairs that cannot connect | 2–4 people [V: 2–4 antmedia, "4–6 at most" bloggeek] |
| **SFU** (selective forwarding) | its camera once, as 2–3 simulcast layers or one SVC stream | N−1 streams, each at the layer its tile needs | forwards RTP packets, no decode; picks layers per receiver | 3+; the norm for group calls [V: Galène, LiveKit, mediasoup are all SFUs] |
| **MCU** (mixing) | once | one composited stream | decodes N, composes, encodes per layout | rejected: needs transcoding on the core, which D193 §1 forbids, and per-viewer layouts defeat "enlarge locally" [I, R] |

**Simulcast and SVC.** The sender encodes 2–3 resolutions (simulcast, `rid`s), or one stream with
spatial and temporal layers (SVC, `scalabilityMode`). The SFU forwards each receiver the layer
that fits its tile and its bandwidth. Browser support [V, MDN BCD 2026-11-06]:

| Feature | Chrome | Firefox | Safari |
| --- | --- | --- | --- |
| Simulcast (`rid` in encodings) | yes | 46 | yes |
| `scalabilityMode` (SVC, L1T3 etc.) | 111 | no | 17 |
| Encoded transform (E2EE), `RTCRtpScriptTransform` | 141 | 117 | 15.4 |
| `getSynchronizationSources()` (audio level for speaker detection) | 73 | 59 | 12.1 |
| Document Picture-in-Picture | 116, desktop only | 151, desktop only | no |
| `Element.requestFullscreen` | 71 | 64 | 16.4; iOS iPad only, "not on iPhone" |

Simulcast works in every browser; SVC does not work in Firefox. **Simulcast is the baseline and
SVC an optimisation** [I]. Galène, for example, turns simulcast/SVC on only at three or more
users and switches layers at most every 500 ms [V, Galène 0.4 notes].

## 3. Bandwidth against a 20–40 Mbit/s home uplink

**Inputs:**
- Tile bitrates are LiveKit's 16:9 presets [V]: 180p (320×180, 20 fps) 0.16 Mbit/s; 360p
  (640×360, 20 fps) 0.45; 720p (1280×720, 30 fps) 1.7.
- Simulcast publishing 180p + 360p is 0.61 Mbit/s; adding 720p makes it 2.31.
- Voice is Opus with RED at about 0.05 Mbit/s each way per person [I].
- The film is a prepared version at about 8 Mbit/s per remote member (D175 §3) [R].
- Each case assumes the household's own devices are on the LAN with the core, so the R = N−1
  remote members are what loads the uplink.

### 3.1 Mesh: each member's own uplink

Per member, upload = download = (N−1) × (tile + voice):

| N | 180p tiles | 360p tiles | encodes per sender [I] |
| --- | --- | --- | --- |
| 2 | 0.2 Mbit/s | 0.5 | 1 |
| 3 | 0.4 | 1.0 | 2 |
| 4 | 0.6 | 1.5 | 3 |
| 5 | 0.8 | 2.0 | 4 |
| 6 | 1.1 | 2.5 | 5 |
| 8 | 1.5 | 3.5 | 7 |

In mesh, "enlarge locally" means the receiver asks that sender, over signalling, for 720p on that
one connection: +1.25 Mbit/s on the sender's uplink, per viewer who enlarges it. **Mesh is light
on the home line**: the core sends only the film, and the household's own camera goes out R
times. The cost falls on each remote member's uplink and CPU. A phone on mobile data encoding
seven streams is the failure case [I]. The other cost is reachability: each pair must connect
(§5).

### 3.2 SFU in the core: the home uplink

Core uplink = R × (film + (N−1) × (tile + voice)). The share added by video is in brackets:

| N (R remote) | film alone | film + 180p tiles | film + 360p tiles | + each remote enlarging one tile to 720p |
| --- | --- | --- | --- | --- |
| 2 (1) | 8 | 8.2 (+3%) | 8.5 (+6%) | +1.5 |
| 3 (2) | 16 | 16.8 (+5%) | 18 (+13%) | +3.1 |
| 4 (3) | 24 | 25.9 (+8%) | 28.5 (+19%) | +4.6 |
| 5 (4) | 32 | 35.4 (+11%) | 40 (+25%) | +6.2 |
| 6 (5) | 40 | 45.3 (+13%) | 52.5 (+31%) | +7.7 |
| 8 (7) | 56 | 66.3 (+18%) | 80.5 (+44%) | +10.8 |

**Reading it:**
- A 20 Mbit/s uplink carries two remote members with film and tiles. A 40 Mbit/s uplink
  carries four.
- The film already fails before the tiles do. Five remote members at 8 Mbit/s fill 40 Mbit/s
  with no video at all.
- At 180p the tiles are a small increment. The SFU should default every tile to its lowest
  layer and raise only an enlarged one [I].
- The core's downlink takes each member's simulcast once, R × 0.61 Mbit/s (4.3 Mbit/s at
  N = 8). Downlinks are usually larger [I].

**SFU against mesh on the home line:** the SFU sends (N−1) tiles to each remote member, while mesh
sends only the household's own. At N = 8 with 180p tiles that is 10.3 against 1.5 Mbit/s. The SFU
trades home uplink for the members' uplinks, CPU and reachability [I].

### 3.3 SFU on a member's or the owner's machine

The same SFU code could run on a machine with a better uplink:
- the owner's VPS, which already hosts the relay D71 describes ("one relay artefact, three
  jobs");
- or a member's desktop.

Then the home line carries only the household's camera once, plus the film [I]. This is still
self-hosted. It suits the owner's deployment, but not the standard package's default, which is
one core.

### 3.4 CPU

- **The SFU forwards packets and never decodes** [I]. At N = 8 it handles 8 incoming simulcasts
  and 56 outgoing tiles. A 180p tile at 0.16 Mbit/s is about 15–20 packets/s, so the whole call
  is a few thousand packets/s.
- Galène, a pion-based SFU, states about 20 participants in one meeting per CPU core [V].
- The cost to watch is the core's existing data-channel overhead. Chrome's data channel costs
  about 15× the CPU per byte of HTTPS (webrtc-throughput-review-2026-10-28.md) [R]. Tiles are
  RTP, not data channels, so that finding does not carry over.
- **Clients:**
  - Each sender runs 2–3 encoders under simulcast, or N−1 encoders in mesh.
  - Each receiver decodes the film plus N−1 tiles.
  - Android's per-device limit on concurrent hardware decoders is undocumented. libwebrtc falls
    back to software when a hardware decoder cannot be created [V, W3C webrtc-stats discussion].
    Software decoding of a 180p tile is cheap.
  - A TV or Fire TV Stick decoding the film plus seven tiles is unmeasured and needs a device
    test [I].

## 4. Self-hosted options, ranked

| Rank | Option | Language, licence | Maintained (2026-11-06) [V] | Embeds in the Go core | Client SDK weight | Simulcast / SVC | E2EE | TURN |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | **Our own SFU on pion in the core** | Go, MIT | pion/webrtc v4.2.26, 2026-10-28; interceptor v0.1.51 | yes; pion is already allowed | none new: the browser's `RTCPeerConnection` and our SDK | ours to write; pion has simulcast and broadcast examples and GCC/TWCC, NACK and PLI interceptors | via encoded transform in the client; the SFU only forwards | not needed for members reaching the core; ICE-TCP and single-port UDP have pion examples |
| 2 | **Galène**, as a reference to read or a sidecar | Go, MIT | active, pushed 2026-11-06 | built on the same pion packages (go.mod: webrtc/v4 v4.2.26, interceptor, ice/v4, turn/v5) but written as a server, not a library [V go.mod; I] | protocol.js 64.5 KB unminified, documented and stable | simulcast; SVC for VP8/VP9 | "Galene does not perform end-to-end encryption" | built-in |
| 3 | **LiveKit server**, a separate process | Go, Apache-2.0 | v1.13.11, 2026-10-25; 22.1k stars | no; server-sdk-go is a client of the server [I] | livekit-client 2.23.0: 610 KB minified, 154 KB gzipped; pulls jose, sdp-transform, webrtc-adapter, protocol | simulcast, dynacast, adaptive stream, SVC | yes, room-level; the app distributes keys | built-in |
| 4 | **mediasoup** | C++ worker, Node or Rust host; ISC | active, 2026-11-04 | no; needs Node or Rust beside the core | mediasoup-client 3.26.0: 652 KB unpacked | yes | via the app | external |
| 5 | **Jitsi Videobridge** | Kotlin/JVM, Apache-2.0 | stable 2026-09-14 | no; needs a JVM plus Jicofo and Prosody for a full deployment [I] | lib-jitsi-meet (not measured) | yes | yes in Jitsi Meet | external |
| 6 | **Janus** | C, GPL-3.0 | active, 2026-11-05 | no; a separate C service | janus.js | yes | via the app | external |
| — | ion-sfu | Go, MIT | dormant: last push 2023-07-21, last release 2021-11-30 | — | — | — | — | — |

**Notes on the ranking** [I unless marked]:

- **Our own pion SFU.**
  - **Why first:**
    - It adds no new dependency. The core already holds pion connections (rtcapi), a TURN
      server and session signalling.
    - It runs in the same process that knows who is in a session (D346's lifetime, guests,
      rights), so a member's video ends when their admission does.
    - It ships in the standard package with no second service to deploy.
  - **What we write:**
    - track routing per session;
    - layer selection per receiver by tile size and estimated bandwidth;
    - keyframe requests on layer switch;
    - RTP header rewriting, so a switch keeps sequence numbers continuous;
    - speaker detection.
  - **Dependencies:** pion/interceptor, rtp and rtcp are allowed today as indirect
    dependencies. Using them directly changes their entry in `deps/go.allow`, which is the
    owner's decision under rule 3, though a formality.
  - **Size:** Galène is about the size of what we would write, and it is MIT, so we can read it
    or borrow from it with attribution.
- **Galène** is the strongest reference. It is the same stack, active, and small. As a sidecar
  it would be a second program to package, configure and authenticate against our sessions, with
  its own protocol in the client. Its Go dependencies outside pion are not on the allow list
  (golang-jwt, gorilla/websocket, ebml-go, jech/*).
- **LiveKit** is the most complete: dynacast pauses layers nobody watches, and adaptive stream
  matches tile size. The price is a second server process with its own ports and keys, plus a
  154 KB gzipped client. That client would be about three quarters of the whole first-page
  JavaScript the Preact spike measured (201–263 KB gzip, preact-spike-2026-11-03.md). Both are
  new dependencies.
- **mediasoup, Jitsi and Janus** each add a runtime the core image does not have (Node or Rust,
  a JVM, C).
  - Janus's GPL-3.0 is compatible with shipping beside AGPL-3.0, but as a separate program.
  - Bundling GPL code would constrain the dual-licensing the contributor agreement keeps open
    (D275) [I].

## 5. Is it tenable without hosted traffic?

**What fraction of networks need a relay** [R, V as cited there]:

| Measure | Figure | Source |
| --- | --- | --- |
| WebRTC calls relayed through TURN | 17.7% in one operator's data (mid-2010s); 22% of conferences in field data; vendors say 10–25% | fippo [V, dated]; discovery-protocol.md §3 [R] |
| Relay need by network type | Tailscale under 10%; WebRTC 20–25%; corporate 60–85% | D68 [R] |
| CGNAT on eyeball networks | 17–18% of non-cellular ASes (2016); over 92% of cellular; 11% of fixed-line CGNs symmetric | direct-reachability.md [R] |
| Client IPv6 | about 46–50%; Google traffic crossed 50% on 2026-03-28 | remote-access.md, direct-reachability.md [R] |
| A phone on mobile data reaching a WebRTC-only household | may fail over 30% of the time | discovery-protocol.md §3 [R, I] |
| Remote viewing needing a third party | 5–20%; 2–10% after one-time fixes (a port-forward, IPv6) | direct-reachability.md [R] |

No recent (2024–2026) measured figure for the share of WebRTC calls relayed was found. Published
numbers are old or vendor estimates [V, search 2026-11-06].

**Why the topology decides this:**

- **Mesh needs every pair to connect.** A pair fails when both ends are behind
  endpoint-dependent (symmetric) NAT, or one is on a UDP-blocked network. Two hard NATs take
  about 28 minutes of birthday-paradox probing (remote-access.md), so a relay is needed in
  practice. The pairwise fraction grows with N: with p the chance that a member is behind hard
  NAT, it is about 1 − (1 − p²)^pairs [I]. Today's relay is the core's own TURN, so a relayed
  pair costs the home line both directions of that pair's media. With mobile members, 40% of
  the cellular networks behind carrier NAT in one field study showed a symmetric NAT
  (discovery-protocol.md §3 [R]), so a group of four with two phones on mobile data will often
  relay [I].
- **An SFU in the core needs only each member to reach the core,** which the film already
  requires. The routes:
  - a port-forward or IPv6: a direct UDP connection to the core;
  - a UDP-blocked member: ICE-TCP to the core's listening port (pion supports it);
  - a core behind CGNAT whose member has no IPv6: the film's existing `webrtc` path,
    hole-punched with discovery signalling. The tiles ride the same peer connection.
  - when hole-punching fails: the owner's own relay (D71) or no remote session, **exactly as for
    the film**.

**What happens to those users:**
- Under the SFU design: whoever can watch the film can see the tiles.
- Whoever cannot reach the core cannot watch the film either. Video adds no new failure.
- Under mesh, a member who can watch but cannot reach another member would lose that member's
  tile and voice, unless the core relays it. The core relaying it is an SFU in all but name.

**Who hosts the relaying, quantified** [I, from §3's bitrates]:

| Relay | Carries | Per relayed member, 2-hour film, N = 4, 180p tiles |
| --- | --- | --- |
| Core's TURN for a mesh pair | one pair's tiles and voice, in and out of the home line | 0.21 Mbit/s each way per pair, about 190 MB each way |
| Core as SFU | already counted in §3.2 | — |
| Owner's VPS (D71) for a CGNAT core | film + tiles + voice for that member | 8.6 Mbit/s, about 7.8 GB out of the VPS |
| **If the owner relaxed the rule**, the paid service carrying video and voice only (the film stays direct) | 3 tiles + 3 voices down, own 180p + voice up | about 0.84 Mbit/s, about 0.75 GB egress |
| Paid service carrying voice only | 3 voices down, 1 up | about 0.2 Mbit/s, about 0.18 GB |

**Answer:**
- With the SFU in the core it is tenable. The fraction that needs a relay is the film's fraction,
  and those users already depend on the owner's relay or go without.
- The minimal relaxation would be a TURN on the paid service carrying **voice only**, and only
  for members who reach the core for the film but cannot connect for real-time media. With the
  SFU in the core that set is close to empty: the same connection carries both. Under mesh it
  is the hard-NAT pairs.
- What is expensive to host is the film, not the chat. Video chat does not change the tenability
  of remote Watch Together; reachability for the film decides it [I].

**Signalling stays essential and small:** SDP offers and ICE candidates of a few kilobytes per
connection over the session WebSocket or the discovery, plus STUN binding requests (D406) [R, I].

## 6. Commercial services, for comparison only

None is an option under the constraint. One line each, prices read 2026-11-06 [V]:

| Service | Model and price |
| --- | --- |
| LiveKit Cloud | hosted LiveKit; free tier 5,000 min; then per-minute and per-GB |
| Daily | $0.004 per participant-minute with video, $0.00099 audio only |
| Twilio Video | $0.004 per participant-minute; the 2026-12-05 end of life was reversed in Oct 2024 |
| Cloudflare Realtime (SFU + TURN) | $0.05/GB egress after 1,000 GB/month free |
| Agora | per 1,000 minutes by aggregate resolution: audio $0.99, HD $3.99, Full HD $8.99 |
| Zoom Video SDK | $0.0035 per participant-minute after 10,000 free |
| Vonage Video | about $0.0041 per participant-minute (search snippet, not read) |

## 7. Watch-party products with video

| Product | Video between viewers | Layout | Limits | Audio | Source |
| --- | --- | --- | --- | --- | --- |
| **Apple SharePlay on Apple TV 4K** (2nd gen+) | FaceTime on the TV, **the iPhone or iPad as its camera** (Continuity Camera) | "Picture in Picture, Split View, Full Screen or Move" | older Apple TVs take the call on the phone | smart volume ducks the film under speech | Apple TV guide [V]; voice-apps.md [R] |
| **Scener** | webcam video, audio, text | room or theater | room 10 on video; theater 10 co-hosts on camera, unlimited text guests; Premium | "Everyone should wear headphones while their microphones are turned on" | scener.com/faq [V]; ran on Agora |
| **Teleparty** | voice and video | — | Premium only | — | voice-apps.md [R] |
| **Deltatre DIVA "Watch Together"** (Sceenic) | camera and microphone | tiles in a panel beside the player; newest on top; yellow border on the speaker; initials when the camera is off | 4 including the host | participant volume separate from the film's | Deltatre docs [V] |
| **Messenger Watch Together** | video call | — | 8; Rooms 50 | — | voice-apps.md [R] |
| **Watch2Gether** | webcam and text | — | — | — | voice-apps.md [R] |
| **Kast** | webcam via screen share | — | — | — | voice-apps.md [R] |
| **Discord Watch Together** | the call's cameras beside a synced YouTube embed | not verified | — | Discord's attenuation | Discord support [V]; voice-apps.md [R] |
| Disney+ GroupWatch, Prime Watch Party, Plex Watch Together | (withdrawn 2023–2025) | — | — | — | watch-together.md [R] |

**What they share:**
- small tiles beside or over the film;
- a speaker highlight;
- per-person volume separate from the film;
- caps of 4–10 people on camera;
- headphones recommended.

Only Apple solves the TV with a camera, and it does so with the phone. No product found lets
each viewer enlarge a tile locally while the film plays; Apple's layouts come closest
[V, as far as searched].

## 8. UX

- **Tiles over the film.**
  - The tiles are absolutely positioned `<video>` elements in the player's container, muted,
    since voice plays through the voice path. Camera off shows initials, as Deltatre does.
  - Default: tiles in a strip along one edge, each at its 180p layer, the strip hideable in one
    press.
  - **Local enlarge:** a press raises a tile to a large overlay or a side-by-side split with the
    film, and asks the SFU for the 360p or 720p layer for this viewer only. The others are
    unaffected [I].
  - The layout is per device and never sent, matching "locally".
- **Fullscreen:**
  - Fullscreen the player's container, not the `<video>`, so the tiles stay visible. This works
    on desktop, Android and iPad.
  - **On an iPhone `Element.requestFullscreen` is unavailable** ("Only available on iPad, not on
    iPhone" [V BCD]). Only the `<video>` goes native fullscreen, and that hides the tiles.
  - An iPhone in landscape keeps the page's own full-window layout instead [I].
- **Picture-in-picture outside the page:**
  - Document Picture-in-Picture (Chrome 116 and Firefox 151 desktop only; no Safari [V]) could
    float the tiles, or the film and tiles together, over other apps.
  - Plain video PiP holds one `<video>`, so it can float the film or one tile, not both.
- **Speaker detection:**
  - `RTCRtpReceiver.getSynchronizationSources()[].audioLevel` works in every browser [V BCD].
  - An SFU can read the RTP audio-level header extension (RFC 6464) to pick and announce the
    active speaker [I; pion support not checked].
  - Use it to highlight the speaker's tile, and later to raise the speaker's layer.
- **Ducking and echo:**
  - Video adds no new echo problem; the camera tile's sound is the voice that exists already.
  - The tiers stand (D63, D65): headphones, the playback offset, Chrome desktop's wide echo
    cancellation, which includes the film's `<video>` audio in its reference (voice-apps.md [R]).
  - Firefox cancels only the output of its default media graph, one device at a time
    [V, Mozilla bugs 1849108 and 1879180].
  - Ducking the film under speech, as SharePlay does, works with speaker detection [I].
- **The TV with the phone as camera:** an extension of split devices.
  - The phone publishes camera and microphone. The TV subscribes to the others' tiles and shows
    them over the film. The person's own tile is the phone's.
  - The phone shows a self-view and the controls.
  - This is Apple's model [V]. It needs the TV page to receive video tracks on its voice-free
    role, which is a new role or a widening of the playback role [I].
- **Mobile:**
  - A phone shows the film with 2–3 tiles at most, or tiles only in landscape.
  - Opening the camera on iOS puts Safari's audio session in a communication mode, as the
    microphone already does (voice-apps.md [R]).
  - Android Chrome with echo cancellation enters MODE_IN_COMMUNICATION, which caps the media
    volume [R].
  - Backgrounding the page stops the camera on iOS and usually on Android [I].
- **Consent and privacy:**
  - The camera is off on joining, every time. Turning it on is an explicit press, shows a
    self-view, and lights a "camera on" mark everyone sees [I].
  - Each person can hide any tile locally, and the host can stop anyone's camera [I; a design
    question].
  - Nothing is recorded: the SFU forwards, stores nothing, and keeps no frames [I].

## 9. Platform constraints

| Platform | Camera publish | Tiles receive | Notes |
| --- | --- | --- | --- |
| Desktop Chrome, Edge | yes | yes | simulcast and SVC; wide echo cancellation includes the film [R] |
| Desktop Firefox | yes | yes | simulcast, no `scalabilityMode` [V]; narrower echo reference [V] |
| Desktop Safari | yes | yes | SVC since 17 [V] |
| iPhone Safari | yes | yes | no element fullscreen [V]; camera stops in background [I]; audio session changes [R] |
| iPad Safari | yes | yes | element fullscreen since 16.4 [V] |
| Android Chrome | yes | yes | concurrent hardware decoder limits undocumented [V: no source]; communication mode caps volume [R] |
| Fire TV Silk, Google TV browsers | no camera in practice | likely, unmeasured | Google ended Meet calling on Android TV in early 2025 [V]; USB webcams worked only for some models [V, forum] |
| Cast receiver (CAF) | no | unknown | no documentation of WebRTC in a Web Receiver found [V: search 2026-11-06]; tiles on a cast TV may need to be drawn by the phone instead |
| tvOS (a native app later) | the phone, as Apple does | react-native-webrtc has audio and video, no data channels [R] | |

**What only a device can confirm:**
- the TV browsers' decode capacity for the film plus N−1 tiles;
- whether Silk on a Fire TV Stick accepts remote video tracks at all;
- the cast receiver's WebRTC.

Each needs a `VQ<n>` item when built (rule 4).

## 10. Interaction with what exists

- **D281 (voice mesh).** Moving voice to the SFU would change D281, which is the owner's
  decision. Audio and video for one person should travel on the same path and peer connection,
  so the browser keeps lip sync. It syncs tracks of one `MediaStream` from the same sender, and
  mixing mesh audio with SFU video loses that [I]. The choice is therefore mesh for both or SFU
  for both.
- **D68 / D71 (relay).** §5: with an SFU in the core the TURN relay matters only for members
  reaching the core, which is the film's reachability. In mesh it remains the pairwise
  fallback.
- **D262 (the film gives way to voice).** The tiles need a place in the order. Proposed [I]:
  voice, then the film, then tiles. The SFU's bandwidth estimate drops tiles to 180p, then to
  the speaker's alone, then to none, before the film's cap moves. The owner may rank the face
  above the film.
- **Split devices Q7 (upload overload).** Video adds the 3–18% of §3.2. "Send away the newest
  player" now has a cheaper first step: shed tiles [I].
- **D346 (lifetimes).** A member's tracks end with their admission and with the session. Video
  signalling, like voice signalling, is not activity. A camera left on does not keep an idle
  session alive [I; consistent with D346].
- **Ephemeral guests** (D174, D346). A guest publishes and receives under the same admission. No
  account and nothing kept.
- **E2EE.**
  - In mesh, DTLS-SRTP is end to end already.
  - With the SFU in the household's own core, the core is the household's machine, which
    already serves the film. Guests already trust it. Encrypting frames against it protects
    guests from the host's own server, which matters little here.
  - Against a VPS-hosted SFU (§3.3), encoded transforms are available in all three engines
    (Chrome 141, Firefox 117, Safari 15.4 [V]) if the owner wants it [I].
- **Recording:** none. Nothing in the SFU writes media [I].
- **The core's own rule** (D193 §1, the core never runs FFmpeg): an SFU forwards RTP and never
  decodes, so it stays within the rule. A composited MCU would not [I].
- **Ports:** pion's single-port UDP mux and ICE-TCP mean one UDP and one TCP port on the core,
  not a range [V: pion examples; I for our use].

## 11. Recommendation (phased), costs and open questions

**Phases** [I]:

0. **Before beta: nothing.** Settle D281's voice and the split-devices questions first. Video
   depends on both.
1. **Video for two** (D175's case, D69's first priority).
   - Add a camera track to the existing voice connection between the two people's devices.
   - Tiles with local enlarge; the camera off by default.
   - Uses the existing relay. No server change beyond renegotiation.
   - Small: client work in `together-voice-call.ts` and the player's overlay.
2. **An SFU in the core on pion, for three or more**, with voice moved onto it.
   - Simulcast at two layers (180p and 360p, plus 720p on desktop).
   - Layer chosen by tile size; speaker detection.
   - Reachability becomes the film's.
   - The largest piece: an estimated few weeks, with Galène as the reference [I].
3. **The phone as camera for a TV**, on split devices.
4. **Later, if wanted:** SVC where supported; Document PiP on desktop; the SFU in the
   relay-mode binary on the owner's VPS (§3.3).

**Options ranked for the full feature:**
1. pion SFU in the core;
2. Galène as a sidecar;
3. LiveKit as a sidecar;
4. mesh only, capped at 4;
5. mediasoup, Jitsi or Janus.

None of the commercial services qualifies under the constraint.

**Costs:**
- Engineering: phase 1 small, phase 2 the bulk, phase 3 medium [I].
- Home uplink: §3.2, about 3–18% over the film at 180p.
- New dependencies: none for options 1 and 4. Several for 2, 3 and 5.

**Open questions for the owner:**

1. Is a group above four with video wanted? If not, mesh plus phase 1 may be enough.
2. SFU in the core for voice too, replacing D281's mesh? Recommended if video goes beyond two.
3. Where do tiles rank against the film when the uplink is short: below the film (proposed) or
   above it?
4. Default tile layout (an edge strip, a corner cluster, or beside the film as Deltatre does),
   and whether "enlarge" means an overlay or a split with the film.
5. May the host stop someone's camera, or only each viewer hide it locally?
6. A TV's tiles: drawn on the TV's page, or only on the phone? It decides whether the TV's
   playback role receives video.
7. On an iPhone in fullscreen the tiles cannot overlay. Accept that, or keep the iPhone out of
   native fullscreen while video is on?
8. Should the SFU also run in the owner's VPS relay (§3.3), for households with a poor uplink?
9. Is the constraint's "essential" signalling meant to include STUN from the discovery (D406)?
   §5 assumes yes.

## 12. Sources

Read 2026-11-06 unless stated.

**Products and services**
- Apple TV SharePlay and FaceTime: https://support.apple.com/en-au/guide/tv/atvb65186d08/tvos
- Scener FAQ: https://scener.com/faq
- Deltatre DIVA Watch Together: https://documentation.deltatre.com/docs/watch-together.md
- Discord Watch Together:
  https://support-apps.discord.com/hc/en-us/articles/26502500234519-Watch-Together
- Google Meet on Android TV: https://support.google.com/meet/answer/13420160
- LiveKit pricing: https://livekit.com/pricing.md
- LiveKit end-to-end encryption: https://docs.livekit.io/transport/encryption.md
- LiveKit video presets: livekit-client 2.23.0's UMD build, from jsDelivr.
- Daily: https://www.daily.co/pricing/video-sdk/
- Twilio Video pricing: https://www.twilio.com/en-us/video/pricing
- Twilio Video end-of-life reversal:
  https://www.twilio.com/en-us/changelog/-twilio-video-will-remain-a-standalone-product
- Cloudflare Realtime: https://developers.cloudflare.com/realtime/sfu/pricing/
- Agora: https://docs.agora.io/en/realtime-media/video/reference/pricing.md
- Zoom Video SDK: https://explore.zoom.us/docs/en-us/video-sdk-rates.html
- Vonage (snippet only): https://api.support.vonage.com/hc/en-us/articles/6646227378204

**Open-source projects**
- GitHub API for repository metadata: pion/webrtc, pion/interceptor, livekit/livekit,
  livekit/client-sdk-js, versatica/mediasoup, meetecho/janus-gateway, jitsi/jitsi-videobridge,
  pion/ion-sfu, jech/galene.
- Galène's dependencies: https://raw.githubusercontent.com/jech/galene/master/go.mod
- Galène: https://galene.org (protocol.js measured from the repository); release 0.4 notes.
- npm registry metadata for livekit-client and mediasoup-client.

**Browser support and implementation**
- MDN browser-compat-data, main branch: RTCRtpSender, RTCRtpScriptTransform, RTCRtpReceiver,
  DocumentPictureInPicture, Element.
- Firefox echo cancellation: https://bugzilla.mozilla.org/show_bug.cgi?id=1849108,
  https://bugzilla.mozilla.org/show_bug.cgi?id=1879180,
  https://bugzilla.mozilla.org/show_bug.cgi?id=1830247
- Decoder fallback when hardware instances are taken:
  https://lists.w3.org/Archives/Public/public-webrtc-logs/2023Mar/0165.html

**Relay and topology figures**
- Mesh limits: https://bloggeek.me/?p=73319 (snippet), https://antmedia.io/webrtc-network-topology/
- Relay share: https://medium.com/@fippo/what-kind-of-turn-server-is-being-used-d67dbfc2ff5d
  (undated, about mid-2010s);
  https://dialogue-io.github.io/2017/04/25/2nd-webrtc-metrics-report-callstatsio

**Repository**
- Decisions: D63, D65, D68, D69, D71, D174, D175, D193, D243, D262, D273, D275, D281, D346,
  D397, D406.
- Research: watch-together.md, voice-apps.md, voice-bad-links.md,
  watch-together-split-devices-2026-10-24.md, webrtc-throughput-review-2026-10-28.md,
  direct-reachability.md, remote-access.md, preact-spike-2026-11-03.md.
- Plans and protocols: plans/watch-together.md §Voice, discovery-protocol.md §3.2 and §3.5.
- Code: `internal/core/turn/doc.go`, `internal/core/rtcapi`, `deps/go.allow`.
