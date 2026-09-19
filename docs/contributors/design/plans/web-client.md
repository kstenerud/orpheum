> **ABOUTME:** The web client — also the admin UI — and the shared TypeScript SDK that every
> other client is built on.

# Web client

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** stream-service.md

## Scope

The shared SDK first (generated from the server's OpenAPI document, plus the hand-written
probe-and-pick logic that implements D2 once for everyone), then the web UI on top of it:
library browsing, search, playback with `hls.js` + `jassub`, and the admin surface.

## Why the SDK is the deliverable

D2 puts the rendition choice on the client. That is only safe if the choice is implemented
**once**, correctly, and shared — otherwise every client picks badly in its own way. The SDK is
where `navigator.mediaCapabilities.decodingInfo()` and its per-platform equivalents get turned
into a rendition preference.

## Keep the SDK framework-agnostic

Per [D28](../../decisions/working-notes.md): **no React or React Native imports** in the API
client, the negotiation logic or the offline state machine. The SDK must be usable from a plain
web page, which keeps a future framework change a UI rewrite rather than a system rewrite. The
device-lab web harness ([device-lab.md](device-lab.md)) is the forcing function — it consumes
the SDK without React Native anywhere near it, continuously.

## Quarantine the offline module

Per [D29](../../decisions/working-notes.md), the offline download state machine is **the one
genuinely portable asset** in the client, and it is only portable if it is separable. Keep it in
its own module with no screen dependencies. Everything else in the SDK is either generated from
OpenAPI, resolved server-side, or inherently platform-specific — which is what makes a future
native client a small reimplementation rather than a port.

## Translation, from the first screen

`react-i18next`, per [D51](../../decisions/working-notes.md) — what both React Native media
clients in the survey converged on — with `returnEmptyString: false`, because translation
platforms export untranslated keys as `""` and the default renders them, so a
partially-translated language shows blank UI instead of English. Catalogues are **served by our
own server** rather than bundled, so a translation fix reaches every client on next connect
instead of waiting for a store review; English is bundled as the offline floor.

Extraction is miserable to retrofit, so this starts with the first screen, not after them. Two
rules that make the rest cheap: no string is concatenated from fragments, and nothing
user-visible is ever stored server-side in a language ([D46](../../decisions/working-notes.md)).

## Non-negotiables

Subtitle offset control; manual override of the playback decision; a visible "why is this
transcoding?"; trickplay scrub previews; resume correct to the second.
