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

## Risk

`react-native-tvos` is a community fork of React Native, not first-party. Streamyfin running it
in production is the mitigating evidence. If the fork stalls, the fallback is a native Kotlin
Compose-for-TV client that reuses only the API contract.
