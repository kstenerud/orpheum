> **ABOUTME:** Casting to any Google Cast device on the network (D465): the browser's own picker beside
> the person's registered devices, a grant for one cast in place of a pairing, and the receiver
> published to every Cast device. Read before building any of it.

# Cast to any Google Cast device

- **Status:** DECIDED, NOT STARTED — 2026-11-12. The owner answered the lead's four questions
  ([D465](../../decisions/working-notes.md#d465--any-google-cast-device-on-the-network-receives-a-cast-under-a-grant-for-that-cast-alone-beside-the-persons-own-devices)).
  The steps below are the lead's proposal; the owner schedules them.
- **Builds on:** [cast-to-device.md](cast-to-device.md) (D337's cast to one's own devices, which
  this keeps) and the receiver of `web/apps/cast-receiver`, which since VQ286 and VQ287 reaches its
  sender's core at home and through the discovery, joins the cast's session, leads it, follows the
  sender's controls, and takes casts in turn.
- **Later, the same way:** AirPlay ([airplay.md](airplay.md)), if it can join the menu like this.

## 1. What the person sees

The Cast menu lists the person's own devices connected now (D337), each with an icon in front for
the kind of cast it takes: the laurel for Orpheum's own clients, the Cast icon for a paired
Chromecast. Then a separator, and a magnifying glass with **Chromecast**, in Chrome on a computer or
on Android (D465 §3). Choosing it opens Chrome's own picker, which finds every Cast device on the
network; choosing a TV there starts the cast, and from then on it is a cast as any other: the sender
is its remote, Stop casting ends it, and the TV goes back to its own screen. Nothing on the TV asks
for a code. AirPlay comes later under it, a magnifying glass with **AirPlay**. A browser that cannot
cast (Safari, Firefox, any browser on an iPhone or iPad) shows no Chromecast entry; a paired TV is
reached there through the person's devices (D465 §4).

Google's sender script loads only in Chrome, at the menu's first opening (the owner, 2026-11-12), so
no page loads it until someone means to cast; the Chromecast entry shows in Chrome from the start.
The Cast icon is Material Symbols' `cast` (Apache-2.0), inlined as the client's other icons are, so
no package is added.

## 2. The steps

1. **The core: a cast for a guest receiver, and its grant** (D465 §2).
   - The sender asks for a grant for a Google Cast device (`POST /me/casts/guest-grants`, naming
     the device as Chrome's picker does); the core mints it: random, opaque, kept in memory as its
     SHA-256 alone, one use, valid for a minute until traded, at most four waiting per sending
     device. (Built 2026-11-12: D154's grant table already exists, in the baseline, but a grant
     that lives a minute and dies with the core needs no row; no migration.)
   - The trade (`POST /guest-receivers`, the receiver's first request) signs the device in as a
     guest receiver: a device row of the person's, with a session of the scope `guestReceiver`,
     never listed in Your devices nor among the live connection's devices, cast to by the grant's
     sender alone and once, and removed when that cast ends by any of D384, D385 or D389's ends, or
     when none starts within live.UnansweredAfter of the trade, and at the core's start. The
     session carries no capability: it is refused at every route but the cast's (its accept, the
     playback of the films the cast plays, their bytes, reports, the live and session sockets).
     The sender then casts to the device it names as to any of the person's (`POST /me/casts`).
   - Tests: the second trade is refused; the credential is refused outside its cast and after its
     end; the receiver's row is gone after each kind of end; the grant never appears in a URL or a log.
2. **The receiver: the grant in place of a pairing.**
   - On our namespace, the sender's launch message carries the grant and how to reach the core (its
     discovery, as the lab's does now). The receiver trades it, then takes the cast as `receive()`
     does now. A pairing kept from Make it a cast target still works as today.
   - The receiver is served from the loader's address, so it reaches the core over WebRTC alone,
     and the core takes an offer only when it is proven under a key it knows; before its trade a
     guest has none. The grant proves its first offer, as a hand-off code does (D308 §4): a key
     derived from the grant under a purpose of its own, which the core takes under its sender's
     locator only while the grant waits. After the trade the receiver proves later offers by its
     own session (the owner, 2026-11-12).
   - Tests in `src/guest.test.ts`: a launch with a grant accepts its cast without pairing and keeps
     nothing in storage after it ends.
   - (Built 2026-11-12. The receiver listens on the lab's namespace and on
     `urn:x-cast:org.orpheum.cast`; a launch there is `{type: "guest", grant, reach}`, the reach the
     lab's without its token, and it answers `{type: "ready", deviceId}` or `{type: "failed",
     reason}`. `proof.GuestGrantOfferKey` is the grant's key, and the SDK's `targetOfGuestGrant`
     proves by it until the trade. On a device: VQ302.)
3. **The sender: the Cast menu's icons, and Chromecast in it.**
   - Google's Cast sender script, loaded only in Chrome and only when the menu is first opened (§1).
     The web client's content security policy allowed it on the lab's pages alone; allowing it on
     the player is a security change, reviewed with the policy. Built: every screen of the client
     allows the sender's origin, since the client moves between screens in place and a page keeps
     the policy it was loaded with; files and the lab's other probes do not
     (`TestHandler_OnlyTheScreensAndTheReceiverMayLoadGooglesCastSDK`).
   - The receiver's application ID comes from the loader the page came from (D465 §5), never from
     a field typed in, as the lab's is.
   - On a session started: ask the core for the cast and grant (step 1), send them over the
     namespace, and become the cast's remote, as for a paired TV. A session the person stops from
     Chrome's own Cast control ends the cast too.
   - Each registered device's icon from its kind (the laurel, or the Cast icon for a paired
     Chromecast, which the receiver's client name says).
   - Tests: the menu offers Chromecast only in Chrome, each device has its kind's icon, and the
     sender script is not loaded before the menu opens; the grant
     goes over the namespace and nowhere else.
4. **Google's console (the owner's account).** Register production's receiver application, its
   address production's loader's `/cast/`, and publish it; staging's stays unpublished on the
   registered TVs. Publishing is self-service, with no review, and reaches every Cast device.
5. **Device checks (VQ items).** The owner's Chromecast with its registration removed, under
   production's application; a TV with Cast built in (an Android TV or Google TV) and a Chromecast
   with Google TV; from Chrome on Android; at a house that is not the core's, through its discovery;
   and a second sender on the same network that cannot trade a grant already traded.

## 3. What could go wrong

- **Older receivers.** TVs with Cast built in run their own Chromium builds; the receiver already
  lowers its syntax to Chromium 90 and fills what it lacks (`src/old-engine.ts`), and reports what
  each device plays. A Nest Hub shows 720p at most.
- **A listener on the network** sees the receiver's messages (D135): the grant is worth nothing once
  traded, and the credential it buys reaches one cast.
- **Senders other than Chrome** cannot use this at all; native apps would be the only way, and are
  not planned.
- **Rough size, unverified:** step 1 a few days, steps 2 and 3 about a week together, step 5 a week
  of device runs and fixes.
