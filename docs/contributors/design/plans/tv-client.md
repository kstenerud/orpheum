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
native UI** if React Native fails generally. Reverse if: the prototype feels wrong on the oldest Fire Stick
(dropped frames on directional navigation, focus traps, latency a native app would not have);
`react-native-tvos` falls two minor React Native versions behind for a quarter; our own native
fraction passes roughly half (Streamyfin's is 31%); or Shopify's migration is reported a success
with numbers in a year.
