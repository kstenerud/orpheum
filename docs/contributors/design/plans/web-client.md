> **ABOUTME:** The web client — also the admin UI — and the shared TypeScript SDK that every
> other client is built on.

# Web client

- **Status:** PLANNED — designed 2026-09-17; licence, adaptation, presentation and failure handling added 2026-09-19 ([D80](../../decisions/working-notes.md), [D86](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D92](../../decisions/working-notes.md)).
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

## The SDK is MPL-2.0, and the server is not

[D80](../../decisions/working-notes.md). The clients and this SDK are **MPL-2.0**; the server,
node and relay stay GPLv3. Two reasons, and the first is fatal rather than stylistic: a GPLv3
application cannot be distributed through the Apple App Store — VLC was pulled in January 2011
and returned in July 2013 only after relicensing, and in this exact field **Swiftfin and
Streamyfin are both MPL-2.0 while the Jellyfin server is GPL**. The second is that
[D20](../../decisions/working-notes.md) says this SDK is what third parties build clients on,
and a GPL SDK is a poor foundation for that. **Set it before the first commit** — relicensing
later needs every contributor's agreement.

## Four more things the SDK owns

- **Re-picking on the buffer** ([D86](../../decisions/working-notes.md)). The rendition choice
  is already here; what is new is that it re-runs during playback, driven by buffer occupancy
  rather than a bandwidth estimate — which is what Netflix's own SIGCOMM'14 study found beat
  their production algorithm by 10–20% on rebuffer rate. One implementation, shared, or every
  client adapts badly in its own way.
- **The failure ladder and the local memory** ([D92](../../decisions/working-notes.md)). When a
  decoder fails despite a positive probe, fall to the next entry in the client's own prioritised
  list, say so, and remember the (probe answer → failure) pairing **on the device**. It never
  leaves except as an observation, and clearing it must be harmless.
- **Presentation state** ([D90](../../decisions/working-notes.md)): caption font, size, colour,
  background, window and edge; the signed audio and subtitle sync offsets; dialogue boost; the
  per-device dynamic-range declaration. All of it persists server-side at
  [D24](../../decisions/working-notes.md)'s scopes, so it follows the user rather than the
  browser.
- **Accessibility is a conformance list, not a preference screen**
  ([accessibility.md](accessibility.md)). The control set is the one 47 CFR §79.103 enumerates
  and the EU Accessibility Act requires; it is also, item for item, what `jellyfin-web`'s open
  subtitle issues ask for.

## Non-negotiables

Subtitle offset control **that persists**; manual override of the playback decision; a visible
"why is this transcoding?" — and "why is this subtitle an image, and what did that cost?";
trickplay scrub previews; resume correct to the second.
