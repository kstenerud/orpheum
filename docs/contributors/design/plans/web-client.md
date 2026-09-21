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

## The cast receiver is this client in a different shell

[D124](../../decisions/working-notes.md). A Google Cast receiver is a web app, so it is mostly
this player with a different input surface — and it is **the best path for a Watch Together
guest**, because we choose what our own users install and not what their friends own. That
makes it a closer neighbour of this plan than of [tv-client.md](tv-client.md), and the build
order is **web client → cast receiver**, well before our own television client is finished.

**It inherits the SDK's probe-and-pick** — a receiver has a narrower decoder set than the phone
driving it and the *receiver* must be the one choosing
([D2](../../decisions/working-notes.md)) — and on Cast that is a documented API,
`CastReceiverContext.canDisplayType()`, rather than something to build.

**It does not inherit jassub** ([D137](../../decisions/working-notes.md), correcting an earlier
reading). Cast's own `TextTrackStyle` carries `fontFamily`, `fontScale`, `foregroundColor`,
`backgroundColor`, `edgeType`, `windowType` and the rest, and Google's documentation states
that *"the window concept is defined in CEA-608 and CEA-708"* — **it is every row of the
statutory caption set in [accessibility.md](accessibility.md)**, implemented by the platform.
Serving WebVTT and setting `TextTrackStyle` is better than shipping libass there, and cheaper.
What the receiver *does* need is an **image layer** for
[D78](../../decisions/working-notes.md)'s server-rendered overlay, which is how ASS and PGS
reach a platform that supports neither — an element over a `<video>`, not a project.

So the custom receiver is forced by **Watch Together's six methods and by bitmap overlays**,
not by subtitles in general: an MP4 of H.264 + AAC with WebVTT casts on the **Default Media
Receiver** with nothing of ours involved, which is a real degraded mode.

**And it has two consumers, not one.** Besides the Watch Together guest, it serves **ordinary
casting to your own television** ([D135](../../decisions/working-notes.md)) — which is the
commoner case and the one that justifies the work on its own. Useful facts for that build,
verified against Google's documentation: `QUEUE_NEXT` and `QUEUE_PREV` are standard media
commands and **even the Default Media Receiver queues audio and video** (only image queues are
unsupported); `QueueBase.fetchItems` loads a **window** of a queue on demand, which matters
because the media namespace caps a message at **64 KB** and a full season does not fit.

## The metadata editing surface, which nothing owned

[D140](../../decisions/working-notes.md). The metadata *model* is unusually thorough —
field-level provenance, manual identification overrides that survive rescans
([D7](../../decisions/working-notes.md)), user-chosen artwork, sidecar write-back
([D98](../../decisions/working-notes.md)) — and **no document said where a person actually does
any of it.** A UI gap behind a well-specified model, which is the kind that survives review.

It belongs here because it is admin-shaped and this client is the admin surface. The rules it
must honour already exist: an edit is **user-authored data about the work**, so it rides the
change journal, outranks any provider, and **is never overwritten by a refresh** — the failure
Jellyfin's *Some custom primary images/art seem to have automatically reverted* records.
Plex's *Metadata Editing on Mobile* sits at 66 likes, so the phone wants it too, later.

## And this is the desktop client

*Are the Desktop Apps Dead?* is 186 likes on Plex's forum
([D140](../../decisions/working-notes.md)) — a question worth answering before it is asked.
**There is no separate desktop application; this is it.** Saying so is better than leaving a
hole that reads as neglect.

## Theming is a seam we ship, or one they force

[D138](../../decisions/working-notes.md). Roughly **5,840 GitHub stars** sit on projects whose
only purpose is changing how Jellyfin looks — `ElegantFin` (2,163), `JellySkin` (1,026),
`Ultrachromic` (994), `finimalism` (460), `Jellyfin-Media-Bar` (207) and more. The strongest
signal is **`jellyfin-plugin-skin-manager` at 660★: a plugin whose only job is installing other
people's CSS.** When a community builds a package manager for workarounds, the missing thing is
a seam.

Two of those are worth reading closely. **`Jellyfin-Media-Bar` is a feature request written in
CSS** — a hero row the product does not have; themes are where unmet UI demand becomes visible.
And **`ElegantFin` is *"inspired from Jellyseerr"***, so the *request* tool is the design people
want their media server to look like.

So we ship one deliberately. The alternative is not "no theming" — it is unsupported CSS
injection against an unversioned DOM, which breaks every release and is
[D85](../../decisions/working-notes.md)'s compatibility problem pointed at markup. **A small
declared surface — tokens for colour, density and poster shape — is cheaper to honour than an
accidental one, and it can be versioned.**

**What it is not:** a UI plugin API, and not arbitrary script. Themes are **data**; third-party
code in the client's origin is the last thing to allow unsandboxed
([D12](../../decisions/working-notes.md)). What the surface actually contains is open, and
wants one real theme author before it is guessed at.

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
