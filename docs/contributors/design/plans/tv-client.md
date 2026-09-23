> **ABOUTME:** The Android TV client — the client the project will actually be judged on — and
> the native player module pattern the other platforms will reuse.

# TV client

- **Status:** PLANNED — designed 2026-09-17; sign-in, accessibility and the Apple licence consequence added 2026-09-19 ([D93](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D80](../../decisions/working-notes.md)).
- **Depends on:** web-client.md


## Stores, and what gets a media client pulled

[D187](../../decisions/working-notes.md), [research/app-stores.md](../research/app-stores.md). **No store bans a client for
your own server; they remove content-source features and reject on operations.** So: no add-on
catalogue, IPTV import or torrenting in any build; a **permanent reviewer demo server** of
public-domain or Creative Commons content, reachable worldwide over HTTPS with a stable login;
listings whose first line is *requires your own server* and whose screenshots come only from that
demo library; an app that **opens with no server reachable, says why, and resumes after
reconnecting** — the failure that held Jellyfin's Samsung app for two years; an **organisation**
Play account and package names and signing keys registered under Google's developer verification
(enforced from 30 September 2026 in four countries, globally in 2027 — for every developer; nothing
is grandfathered); the same APK for Fire OS and a WebView build for Vega OS, which cannot sideload;
and the signed APK, F-Droid and `adb` paths [D108](../../decisions/working-notes.md) promised,
because any store may pull us for a while.
## Why Android TV first

Largest installed base among self-hosters, sideloadable with no store gatekeeping, and cheap
Android TV sticks are the community's standard answer to bad smart-TV platforms.

**Sideloading is a rollback advantage before it is a distribution one**
([D108](../../decisions/working-notes.md)). The second-biggest cluster on Plex's forum this
year is Fire TV users asking how to get last month's app back after an update broke playback,
with a pinned workaround thread and a campaign to petition Vizio — because on a television the
vendor is the only party who can un-ship an update. Publishing every build as a versioned APK
and keeping old ones up costs nothing and removes that entire failure mode. It also shares
almost everything with the Android phone client, which then comes nearly free.

## Shape

React Native (`react-native-tvos`) shell over the shared SDK, with a **Kotlin native player
module** exposing two engines — Media3 by default, libmpv as the "play anything" fallback,
user-switchable (D10). Streamyfin's `modules/exoplayer-player` and `modules/mpv-player` are the
reference implementation.

## The thing to prototype before committing

**Remote-control focus navigation.** It is what a TV app lives or dies on, it is not something
research can settle, and it is where React Native on TV is most likely to disappoint. Build a
throwaway focus-navigation prototype before writing any real screens.

**And now there is a number for it to produce** ([D138](../../decisions/working-notes.md)).
Measured from Plex's own forum, where **51% of the year's top-50 likes are interface
complaints**, the two things users actually judge a TV client on are **click depth** and
**focus latency** — *"one click instead of 30"* to get between library and collections, and
*"scrolling between titles and the 'selection' lags by an entire second"* on a 4K Fire Stick.
**Nobody in that corpus says it is ugly.** So the prototype's deliverable is the **delay
between a D-pad press and the selection moving, on the worst device in the house**, not a
feeling about whether it seems smooth.

Two consequences for the screens that follow. **The home screen's row order belongs to the
user** — removing a re-orderable sidebar is one of the specifically named angers
(*"I could arrange the order. Your categories are not arrangeable"*). And **a dense, titled
list view is a first-class layout, not a later option**: `List view` (+30) and
`show titles in poster view` (+28) are both open requests on `jellyfin-androidtv`, and
retrofitting a second layout costs more than allowing for two.

**And the row *set* belongs to the owner, where the row *order* belongs to the user**
([D149](../../decisions/working-notes.md), 2026-09-21). Plex models it as `ManagedHub` with a
`homeVisibility` of none/all/admin/shared; the Jellyfin plugin that adds server-defined home
sections is 513★. The design lives in [web-client.md](web-client.md) and the table in
`library-model.md`, but it lands here first, because the television is where a curated row for
a role actually matters — *"this collection is a row on the kids' home screen"* is a
living-room request, not a browser one.

## Fire TV is available for testing, and is not quite Android TV

Fire OS is an Android fork, so the client and `react-native-tvos` work and sideloading over ADB
is straightforward — which makes the available Fire TV Sticks a real test bed spanning
H.264-1080p to AV1-4K ([test-hardware.md](../research/test-hardware.md)). What Fire OS does not
provide is Google Play Services (so no Google Cast), and the Android TV / Google TV home-screen
channel and recommendation APIs, which Fire TV replaces with its own. Plan the platform-
integration work against a Google TV device; plan the playback and focus work against these.

## Locale on a TV

Two TV-specific consequences of [D51](../../decisions/working-notes.md).

**Direction follows the OS, and there is no in-app switch.** `I18nManager.forceRTL()` sets a
native flag read at startup, so changing it does not re-lay-out the running app — still
reproducing on RN 0.77. A forced restart is tolerable on a phone and bad on a TV, and nobody
changes their TV's language weekly. One RTL locale belongs in the [device-lab](device-lab.md)
pass regardless: Jellyfin has the Arabic layout bugs that come from not having one.

**The device may have no font for the script.** The Android CDD mandates Latin, Greek, Cyrillic
and emoji — and searching it for "CJK", "Chinese", "Arabic", "Hebrew" and "Thai" returns zero
occurrences each. A compliant TV box can render tofu, and the user will read that as broken
subtitles. Bundling is the wrong fix at ~16 MB per weight per CJK variant, so the client reports
the scripts it can render and the server supplies the rest
([D53](../../decisions/working-notes.md)). The floor under that is **Unicode Last Resort HE**,
bundled — 588 KB for every code point in Unicode, rendering a framed script indicator rather
than a box, so the worst case tells the viewer *"you need a Japanese font"* instead of *"this
app is broken"* ([D54](../../decisions/working-notes.md)). Enumerating the fonts on every device in the rig is a
few seconds each and belongs in the first [device-lab](device-lab.md) pass.

**The server owns sort order.** Hermes does not bundle ICU — it projects the platform's, whose
*"results may vary between devices running different Android versions"* — so `localeCompare`
can order the same list differently on an Android 10 phone and an Android 14 TV. The client
renders the list it is given ([D47](../../decisions/working-notes.md)). For the same reason,
plural handling must not depend on `Intl.PluralRules`, which Hermes does not implement.

## The long tail is a cast receiver, not more TV clients

[D124](../../decisions/working-notes.md). This plan covers the television **our user** owns.
For a Watch Together guest's television we have no such luxury, and the answer is not to write
more native clients: it is a **cast receiver**, which is the web player in a different shell
([web-client.md](web-client.md)) and reaches any Cast-capable set. Note the hole in that —
**Fire OS has no Google Cast**, as below — so "cast" is a family of receivers behind one
role (Google Cast, AirPlay, ours) rather than one protocol.

## Handing off to an external player

[D140](../../decisions/working-notes.md). *"Add option to play via external player directly
from movie/episode screen"* is +28 on `jellyfin-androidtv`, and it is **the honest answer to
[D17](../../decisions/working-notes.md)'s tail**: when we cannot play a file and no transcoder
is registered, hand it to something that can rather than showing a failure. Cheap — an intent
with a URL — and it is a pressure valve: the alternative is the force-transcode arms race the
same tracker is full of.

It composes with the transcode-reason field rather than replacing it. The client still says
*why* it could not play the file; the handoff is what it offers next.

## Signing in, which is the first thing that happens

Nobody had designed it, and it is the first interaction every user has, on the device with the
worst text entry ever shipped. [D93](../../decisions/working-notes.md): **RFC 8628**, the OAuth
2.0 Device Authorization Grant — the TV shows a short code and a URL, the user opens it on a
phone, the TV polls. It is what the large streaming services use and the shape Jellyfin arrived
at independently as Quick Connect. See [auth.md](auth.md); the TV's part is a screen, a poller
and no keyboard.

## The remote's contract

[D188](../../decisions/working-notes.md). On a television the remote *is* the interface, and the field's
player complaints are about it treating the remote as a pointer:

- **Left and right always seek from the current position**, from any on-screen state,
  whatever has focus. Samsung users report focus jumping to Pause after one press, and seeks
  that snap to fixed marks.
- **10-second steps** (Apple moved from 15 to 10, *"easier to track how far you've jumped"*),
  **scrub thumbnails for anything over 15 minutes** (Roku's certification rule), and the
  controls hide quickly.
- **Track menus are visible buttons on the control bar**, one level deep, each track named.
  Apple removed its swipe-down panel in tvOS 15 because buttons *"improved the
  discoverability"*.
- **Long-press on a Continue Watching item offers "hide"**, with undo
  ([play-state.md](play-state.md)).

## Accessibility on a television

[accessibility.md](accessibility.md), [D90](../../decisions/working-notes.md). The caption
control set — font, size 50–200%, text and background colour, background and window opacity,
five edge attributes — has to be reachable **during playback in at most two remote presses**,
because "readily accessible" is the actual requirement and a TV settings tree is where controls
go to die.

And the screen-reader question is the same question as the focus prototype above: TalkBack
walks the focus tree. **Every focusable card's accessible name is its item's title**, never
its badges: Plex's Fire TV reader announces unwatched counts, *"1… 241… 36"*, instead of
the shows. Doing them together costs almost nothing; doing them separately means
building the focus model twice.

## Risk, and the bail-out

`react-native-tvos` is a fork of React Native, not first-party — though it tracks upstream
within weeks and Streamyfin runs it in production
([client-framework.md](../research/client-framework.md)). If the fork stalls, the fallback is a
native Kotlin Compose-for-TV client reusing only the API contract.

The decision was **re-examined on 2026-09-18** against Shopify's reversal to Swift and Kotlin
and upheld, with a falsifiable bail-out so the focus prototype below is a decision point rather
than a formality. **Bailing out does not mean going fully native** —
[D28](../../decisions/working-notes.md) names the fallbacks: a **hybrid** (keep RN for web and
phone, native TV only) if the failure is TV-specific, which is the likely case; **KMP plus
native UI** if React Native fails generally.

**One constraint on the Apple half that is not technical.** Under
[D80](../../decisions/working-notes.md) the clients are **MPL-2.0**, not GPLv3, because a GPLv3
application cannot be distributed through the App Store. That makes
[D67](../../decisions/working-notes.md)'s "tvOS never gates a feature" achievable; under a
whole-project GPLv3 there would have been no tvOS app to gate anything.

[D30](../../decisions/working-notes.md) prices the Apple half of that fallback much lower than
assumed: Swiftfin shares ~80% of its code between iOS and tvOS with only ~3.2% tvOS-specific, so
going native on Apple buys **both** for roughly the cost of one. And a rule that matters more
for TV than anywhere else: **tvOS has no WebKit**, so the ship-the-web-UI contingency available
on Tizen, webOS and Android TV does not exist there. Reverse if: the prototype feels wrong on the oldest Fire Stick
(dropped frames on directional navigation, focus traps, latency a native app would not have);
`react-native-tvos` falls two minor React Native versions behind for a quarter; our own native
fraction passes roughly half (Streamyfin's is 31%); or Shopify's migration is reported a success
with numbers in a year.
