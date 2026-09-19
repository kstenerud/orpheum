> **ABOUTME:** The Android TV client — the client the project will actually be judged on — and
> the native player module pattern the other platforms will reuse.

# TV client

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** web-client.md

## Why Android TV first

Largest installed base among self-hosters, sideloadable with no store gatekeeping, and cheap
Android TV sticks are the community's standard answer to bad smart-TV platforms. It also shares
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
([D53](../../decisions/working-notes.md)). Enumerating the fonts on every device in the rig is a
few seconds each and belongs in the first [device-lab](device-lab.md) pass.

**The server owns sort order.** Hermes does not bundle ICU — it projects the platform's, whose
*"results may vary between devices running different Android versions"* — so `localeCompare`
can order the same list differently on an Android 10 phone and an Android 14 TV. The client
renders the list it is given ([D47](../../decisions/working-notes.md)). For the same reason,
plural handling must not depend on `Intl.PluralRules`, which Hermes does not implement.

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

[D30](../../decisions/working-notes.md) prices the Apple half of that fallback much lower than
assumed: Swiftfin shares ~80% of its code between iOS and tvOS with only ~3.2% tvOS-specific, so
going native on Apple buys **both** for roughly the cost of one. And a rule that matters more
for TV than anywhere else: **tvOS has no WebKit**, so the ship-the-web-UI contingency available
on Tizen, webOS and Android TV does not exist there. Reverse if: the prototype feels wrong on the oldest Fire Stick
(dropped frames on directional navigation, focus traps, latency a native app would not have);
`react-native-tvos` falls two minor React Native versions behind for a quarter; our own native
fraction passes roughly half (Streamyfin's is 31%); or Shopify's migration is reported a success
with numbers in a year.
