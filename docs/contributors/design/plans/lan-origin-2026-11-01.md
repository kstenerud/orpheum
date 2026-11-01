# The core's LAN origin at home: the proposal for D396 (2026-11-01)

For the owner's decision. It is item 2 of the 11-02 batch, and item 1 of "Reaching the core at home,
and WebRTC's cost" in
[navigation-and-widgets-answers-2026-10-27.md](navigation-and-widgets-answers-2026-10-27.md). The
owner was "not against" it (2026-10-28). The evidence is
[network-stack-audit-2026-10-28.md](../research/network-stack-audit-2026-10-28.md) §4–5 and
[webrtc-throughput-review-2026-10-28.md](../research/webrtc-throughput-review-2026-10-28.md) §6 item 1.
Once decided, this becomes D396, replacing its reserved entry.

## Why

At home, Firefox and the Fire TV's Silk reach the core only over WebRTC data channels:

- Firefox fails a public page's requests to a LAN name under a service worker (`usesLanName`).
- Silk's local-network permission sits at `prompt`, and Silk shows no prompt.

The old Fire TV loads 15–30 Mb/s over data channels. The same TV loads 190–224 Mb/s when it opens
the core's own page at the LAN name, because a page at the LAN name is same-origin to a private
address and so outside every Local Network Access check. A 4K rung does not fit through the data
channel.

Item 3 of this batch (WebRTC tuning) helps desktops 2–3×. It cannot reach HTTPS on a slow device,
whose data channel costs about 15× the processor time per byte (the review's §4.4). The at-home
fallback also rests on WebRTC staying outside Local Network Access, which the standard's own
explainer says will change.

## The proposal

1. **When the tab moves.** It moves when the page is at home and its requests would otherwise
   cross WebRTC:
   - "At home" means the WebRTC connection is proven a direct host pair inside the house (D300,
     D356 §1), or the discovery judged the device at home (`isLanOf` in `discovery-start.ts`).
   - "Would otherwise cross WebRTC" means the browser can't use the LAN name cross-origin: Firefox,
     or a local-network permission that isn't `granted`, as on Silk.

   Chrome and Safari with the permission granted already load at HTTPS speed cross-origin, so they
   stay where they are. The move is not raced and never happens by itself. Protocol §3.1's "not a
   path the client races" still holds; what changes is that the client offers the move.
2. **It asks first, once per network.** The first time, the page shows a note: "Use the faster home
   connection? This page will reopen at your server's home address." There are two buttons, *Use it*
   and *Not now*, and both can be reached by remote. The answer is remembered per network
   (`clientPrefix`, protocol §3.3 item 7). *Not now* is asked again after 30 days.
3. **The sign-in is carried over** by D308 §4's hand-off, in a variant that takes the signed-in
   session instead of a password:
   - `POST /session/handoff/signed-in` takes the session's bearer token over the path the page has
     (WebRTC). It answers a one-use code, valid for a minute and bound to the same device record, so
     *Your devices* does not show the device twice.
   - The page navigates to `https://<the core's LAN name>/#handoff=<code>&back=<client origin>`, and
     the core's own client redeems the code with `/session/redeem`.
   - The new session keeps its realm and its admin standing as the request's, as every session
     does (D297 §6). At home, admin works in every browser.
4. **The place in the app moves with the tab:** the hand-off carries the path, so a film's page
   reopens as that film's page. A film playing is paused and resumed at its position after the move.
5. **Leaving home moves the tab back.** The LAN origin's page keeps the `back` address. When none of
   its requests reach the core for 10 s while the browser can reach the network, it navigates to the
   client origin, which takes over as today. Its service worker can show that step offline. Watch
   Together and cast links keep naming the client origin, which hands on again.
6. **A move that fails is noticed.** A router with DNS rebinding protection (dnsmasq's
   `stop-dns-rebind`, the FRITZ!Box, Pi-hole, NextDNS) refuses public names that resolve to private
   addresses. The browser then shows its own error page, which the client page can't catch, and the
   person presses Back.
   - Before navigating, the client page records "moved at T on network N" in its storage. If it
     loads again on N within 2 minutes without the LAN page's word that it got in, it takes the move
     as failed.
   - After a failed move, the page doesn't move again on that network. It says once: "Your router
     blocked the home address, so this page stays on the slower connection. Your router's
     'DNS rebinding protection' setting can allow it." That setting's exemption is the same one
     Plex asks for `plex.direct`.
   - The LAN page tells the client origin it got in through the `back` navigation's fragment, and
     through nothing else, since the two origins share no storage.
7. **Each origin keeps its own storage.** Settings kept per device, such as the remote's choices and
   levelling, are stored per origin, so the LAN origin starts with the server's defaults for the
   device. This is accepted for a first version. The settings that matter are already the core's.
8. **WebRTC stays** as the path below every direct name: away from home behind CGNAT, and at home
   when the move is refused or fails. D274 §2's baseline promise is unchanged.

## What it amends

- D308: "the client a browser runs is its core's, at its client origin" becomes "…at its client
  origin, or at its LAN origin at home".
- D308 §4: a second hand-off, which takes a session.
- Discovery protocol §3.1: the home address becomes a path the client offers, still never races.
- D300 and D356: their at-home judgement is now used for the move as well.

## What it costs

- **Core:** one endpoint, and `Auth.HandOff`'s variant that takes a session and binds the code to the
  device.
- **Web:**
  - the choice in `path-choice.ts` and `discovery-start.ts`;
  - the note and its remembered answer;
  - the navigation, and the redeem with the path at the LAN origin;
  - the way back after 10 s unreached;
  - the failed-move record.
- **Tests:** a unit test for each rule above, plus an e2e test of the hand-off between two origins.
- **Device checks (VQ):** Firefox on the Mac, Silk on the Fire TV, and a router with rebinding
  protection on.
- **Size:** small to medium, about one agent's batch item.

## For the owner

1. **The move, as above:** at home, offered once per network and asked first, with a session per
   origin. Yes or no?
2. **The wording of the two notes** (items 2 and 6). Change them as you like.
3. **Not proposed now:** the client at the core's own names away from home too (the audit's §5.2
   step 2). It touches D308 §3's reasons for client origins and the shared `orpheum-core.com`
   cookies. My recommendation is to decide it after the at-home move has run for a while.
