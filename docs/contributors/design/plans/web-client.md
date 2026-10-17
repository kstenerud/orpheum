> **ABOUTME:** The web client — also the admin UI — and the shared TypeScript SDK that every
> other client is built on.

# Web client

- **Status:** PLANNED — designed 2026-09-17; licence, adaptation, presentation and failure handling added 2026-09-19 ([D80](../../decisions/working-notes.md), [D86](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D92](../../decisions/working-notes.md)). **In Cut A** ([D178](../../decisions/working-notes.md), good on a phone); **2026-09-23: plain React + TypeScript + Vite over a React-free SDK, and the cast receiver a second Vite app served from the owner's own HTTPS name** ([D193](../../decisions/working-notes.md) §6, [D192](../../decisions/working-notes.md)).
- **Depends on:** stream-service.md

## Scope

The shared SDK first (generated from the server's OpenAPI document, plus the hand-written
probe-and-pick logic that implements D2 once for everyone), then the web UI on top of it:
library browsing, search, playback with `hls.js` + `jassub` (stock JASSUB in Cut A —
[D193](../../decisions/working-notes.md) §4), and the admin surface.

**The stack** ([D193](../../decisions/working-notes.md) §6, refining
[D10](../../decisions/working-notes.md)): **plain React + TypeScript + Vite**, in an npm
workspace ([D196](../../decisions/working-notes.md): npm ships with Node, so the build needs one tool fewer) whose
packages are the SDK (React-free), the web client and the cast receiver. The
API types come from the server's OpenAPI document through `openapi-typescript` and
`openapi-fetch`. React Native is not in the web client; the television's framework is decided
separately, when TV work starts.

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
the SDK without React Native anywhere near it, continuously. With the web client itself on plain
React ([D193](../../decisions/working-notes.md) §6), the SDK is its own workspace package and
imports no React either.

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

**It is a second Vite app in the same workspace**, over the same SDK
([D193](../../decisions/working-notes.md) §6). During the internal stage it stays
**unpublished**, registered to her device by serial number, and **served from the owner's own
server under its one public HTTPS name** ([D192](../../decisions/working-notes.md),
[remote-access.md](remote-access.md)) — a private repository cannot use free GitHub Pages
hosting, and that name has to exist anyway for the microphone.

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
reach a platform that supports neither — an element over a `<video>`, not a project. **Not in
Cut A:** D78's overlay is narrowed to a later question, PGS is not in the first build, and a
prepared rendition carries its text subtitles as ASS for JASSUB and as WebVTT for the receiver
([D193](../../decisions/working-notes.md) §2).

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
a selected source per title with permanent manual fields ([D170](../../decisions/working-notes.md)), manual identification overrides that survive rescans
([D7](../../decisions/working-notes.md)), user-chosen artwork, sidecar write-back
([D98](../../decisions/working-notes.md)) — and **no document said where a person actually does
any of it.** A UI gap behind a well-specified model, which is the kind that survives review.

It belongs here because it is admin-shaped and this client is the admin surface. The rules it
must honour already exist: an edit is **user-authored data about the work**, so it rides the
change journal, outranks any provider, and **is never overwritten by a refresh** — the failure
Jellyfin's *Some custom primary images/art seem to have automatically reverted* records.
Plex's *Metadata Editing on Mobile* sits at 66 likes, so the phone wants it too, later.

## Gatekeepers a web client has too

[D187](../../decisions/working-notes.md), [research/app-stores.md](../research/app-stores.md). **Served by the server, same
origin** — no LAN-access prompt, no mixed content. And because Google Safe Browsing has flagged
self-hosted Jellyfin and Emby instances as *"Deceptive site"* — many identical login pages on
unrelated domains look like phishing — **no project-branded share or meta tags on a user's
instance, an admin-customisable login page, `noindex` by default**, and our demo on a domain
separate from the project site. Demo content is public-domain or Creative Commons.

## Creating a library asks three things

[D184](../../decisions/working-notes.md), [principles §15](../../principles/15-safe-visible-defaults.md). Each question states its effect, and
the answer is shown on the library's page afterwards:

1. **Where metadata comes from** — NFO files where present and TMDB for the rest · TMDB only · NFO
   files only, never fetch ([D183](../../decisions/working-notes.md)). Artwork follows the same answer.
2. **How changes are noticed** — watch · check every N hours · only when I ask, with what that
   means for sleeping disks ([D94](../../decisions/working-notes.md)).
3. **Whether we may write into the folder** — read-only unless granted; if read-only, the page
   says which features that disables.

## Now playing

[D176](../../decisions/working-notes.md), 2026-09-23. Every live session. **The admin sees all; everyone else sees their
own.** Per stream: who, what, which device, where (LAN, remote, relay), direct play or
transcode **with the stated reason**, bitrate against the ceiling, and **client-reported buffer
health** — the field that answers "why is it buffering". Actions: **stop with a message** to the
viewer, and **lower that stream's ceiling** on the spot (ceilings are not in Cut A —
[D178](../../decisions/working-notes.md) §4). Live only — history is
[play-statistics.md](play-statistics.md)'s.

## And it is the phone client, for now

[D176](../../decisions/working-notes.md): web first until the UI flows are settled, then native clients built from them.
So this client is **good on a phone from its first release** — touch targets, small screens,
iPhone Safari's media constraints — rather than a desktop page that happens to load there.

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
  background, window and edge; the signed audio and subtitle sync offsets; dialogue boost (on
  or off in Cut A — [audio-presentation.md](audio-presentation.md)); the
  per-device dynamic-range declaration. All of it persists server-side at
  [D24](../../decisions/working-notes.md)'s scopes, so it follows the user rather than the
  browser.
- **Accessibility is a conformance list, not a preference screen**
  ([accessibility.md](accessibility.md)). The control set is the one 47 CFR §79.103 enumerates
  and the EU Accessibility Act requires; it is also, item for item, what `jellyfin-web`'s open
  subtitle issues ask for.

## Curated home rows, and the behaviours that are simply missing

[D149](../../decisions/working-notes.md), from the 2026-09-21 source sweep.

**The home screen is a curated object, not just an ordered one.** Plex models this as
`ManagedHub`: an admin promotes a collection to a home row with
`homeVisibility ∈ {none, all, admin, shared}`, plus `promotedToOwnHome` and
`promotedToSharedHome`. The Jellyfin plugin that adds server-defined home sections is 513★.
We have per-user row **order** ([D138](../../decisions/working-notes.md)) and no way for an
owner to say *"this collection is a row on the kids' home screen."* Every piece already
exists — [D130](../../decisions/working-notes.md)'s roles say who the audience is,
[D75](../../decisions/working-notes.md)'s predicates say what is in the row, and
[D134](../../decisions/working-notes.md)'s maintained-value rule is what makes an extra row
affordable instead of another 43.6 ms query on the launch screen.

**And six behaviours are absent from this tree entirely or named once.** Listed because
together they are the difference between "complete" and "obviously unfinished":

| Missing | Evidence | Note |
| --- | --- | --- |
| **Shuffle / random play** | **zero mentions anywhere in the tree** | trivial; every competitor has it |
| **Folder browsing** | +17 *Folder View mode for browsing*; Jellyfin's `EnableFolderView` | the escape hatch when identification is wrong |
| **Playback speed** | streamyfin +10 | **not free**: shared state or explicitly local in Watch Together ([D66](../../decisions/working-notes.md)), no third option |
| **Sleep timer** | +23 | produces `abandoned`, not `completed`, in the play log ([D81](../../decisions/working-notes.md)) |
| **Picture-in-picture / background audio** | 20 reactions | platform surface, per client |
| **Bulk edit / multi-select** | 3 reactions | the admin's time, not the viewer's |

Manual re-identify is the seventh and it already has a home in
[identification.md](identification.md) — recorded here because it is the escape hatch for
every identification bug we will ever ship, and the sweep found it named five times and
designed nowhere.

## "What the scanner saw" — the admin view over every observed file

[D163](../../decisions/working-notes.md), 2026-09-22, asked for by the project owner:

> *"Make an admin UI where I can browse everything the scanner passed on, and tell the system
> what it actually is."*

**It is not a second screen.** [scanner.md](scanner.md) gives every observed file a `filing`
with a reason; the library listing is `filing = listed`, this view is `filing != listed`, and
"show me everything" is no predicate at all — which answers *"this should also extend to the
top level scanned items"* by removing a clause. Same grid, same search, same detail pane, one
extra column. [D132](../../decisions/working-notes.md) already made this argument on the other
axis — *one predicate compiler, one query, a different principal* — because a view that does
not share the real path diverges from it and is worse than none.

**The two axes stay apart.** This filter answers *what did the scanner do with this file*;
"browse as this role" answers *may this person see it*. A library fact and a policy fact.

### It groups by reason, because the complaint is never one file

An extras directory is twenty files, and a tool that only offers twenty rows is abandoned at
file three. The **group is the unit of action**:

> `**/sample.???` skipped **47 files** in *Films* — *unskip all · stop applying this rule here · confirm*

Five actions, closed: **make it a work** (into identification.md's ordinary identify flow) ·
**file it as an extra of X**, with a type · **keep ignoring it** · **stop applying this rule
here** · **retry**, which exists only for `errored`, because a failure is not a decision and
the share that was down may be up.

### You can look before deciding, and play before naming

- **Browse into a subtree that was never walked** — the node's read-only peek, bounded to
  paths under a library root.
- **Play an unfiled file.** You cannot say what something is without watching ten seconds of
  it, so the **playback path must be addressable by a File**, not only by a library item.
  That is an **invariant to keep true now**: free today, a rewrite once every endpoint takes
  an item id. Opening one probes it on demand.

### And the unreadable ones come to you

An ignored file is a decision you can go looking for. **An unreadable file is a failure, and
the user does not know there is anything to look for** — the owner's *"copied a file in, it
never showed up, turned out to be owned by root, no way to see that"*. So it is not only
findable here: the scan summary says **"complete — 3 files could not be read"** rather than
"complete", and the reason names the fix — *"owned by `root:root`, mode `0600`, and this node
runs as uid 998"* — rather than `permission_denied`.

### Where a correction goes

[D98](../../decisions/working-notes.md)'s test, three answers: *"this is the 1982 theatrical
cut"* is about the work → **sidecar**; *"this is not media, keep ignoring it"* is about the
file → **mirrored to disk**, gitignore-style per directory, so it survives a rebuild and
somebody else's scanner honours it too; *"this rule should not apply here"* is
**configuration** → database, and backup-critical.

The capability is on the role ([D130](../../decisions/working-notes.md)).

## The player, from what other web players got wrong

[D188](../../decisions/working-notes.md), from [ui-competitors.md](../research/ui-competitors.md) §4.

- **Keyboard: YouTube's map**, the de facto standard.
  - **Keys:** Space and `k` play/pause; `j` and `l` jump 10 s; the arrows step 5 s and change
    volume; `f` fullscreen; `m` mute; `c` captions; `0`–`9` jump to a tenth of the film; `,`
    and `.` step a frame; `?` shows the list.
  - **Bind on `KeyboardEvent.code`**, not `.key`, so the map works on non-Latin layouts
    (jellyfin-web #7397).
  - **Ignore any key held with Ctrl, Alt or Cmd.**
  - **Space never re-activates the last focused button** (jellyfin-web #4857, open since
    2023).
- **Subtitles have their own layer, which moves up when the controls appear.** This is the
  largest complaint class in jellyfin-web: subtitles under the controls, gone when they fade
  in fullscreen, missing in picture-in-picture. Native WebVTT where the renderer allows it,
  so picture-in-picture keeps them.
- **One panel for audio and subtitles**, the likely languages first and the rest under
  "Other", **each track named**: language, SDH, forced, commentary. Plex's *show track names*
  request has 417 votes, because those tracks otherwise look identical.
- **A network drop is a countdown, not an error page**: keep playing what is buffered, retry
  on a visible timer, resume at the same position.
- **Media Session** for lock screen, media keys and headset buttons, including
  `togglemicrophone` and `hangup` for Watch Together's voice.
- **iPhone: inline landscape plus picture-in-picture, not fullscreen.** iPhone Safari has no
  element fullscreen; its fullscreen is the system player, which hides our controls, our
  Watch Together notices, the voice controls and any subtitles we draw.
- **Autoplay: a play the user did not tap may be refused**, and Jellyfin hangs silently when
  it is (#4901). So a remote "play" that is refused shows **"Tap to resume"**, never a stalled
  splash screen.

## Browsing, settings and errors

- **No pagination and no cap on search.** Lists load a page at a time as they are scrolled, with
  no page to click through, and a poll reads only the pages in view (D306 §7); they are not
  windowed yet, which is to be our own code (D306 §10). Jellyfin's *remove
  pagination* request has 1,176 votes; its search returns at most 24 results and Emby's 30.
- **Focus and hover follow [D189](../../decisions/working-notes.md)**: a ring and a lift, never a darker tile,
  and never grown over a neighbour. Keyboard focus in a grid is a television's focus in a
  browser.
- **Titles on posters by default, and a list view.** A poster wall without titles is the
  default everywhere, and people ask for the opposite (+28, +19, +30).
- **Filters and sorts persist per view and always show a reset.**
- **Every effective setting shows where its value came from**: *you*, *this library*, *this
  series* or *this device only*. The scope chain exists ([D24](../../decisions/working-notes.md),
  [schema.md](schema.md)'s `preference`); making it visible is what the Emby and Jellyfin
  *"saved for the browser, not for the user"* complaints ask for.
- **Every error names the item, the cause and one next action**, in the words the
  [D152](../../decisions/working-notes.md) hop model already gives it. *"Send logs"* is the
  early reply in 39% of Jellyfin's top troubleshooting threads. Each one is an error that
  told nobody anything.
- **Accessible from the structure up** ([accessibility.md](accessibility.md)): semantic
  elements, every card named by its item's title, everything reachable by keyboard, visible
  focus.

## Non-negotiables

Subtitle offset control **that persists**; manual override of the playback decision; a visible
"why is this transcoding?" — in the player's info panel and in the admin's sessions view,
not only in a log (Plex's largest admin topic, 3,069 likes, is transcoding nobody could see) — and "why is this subtitle an image, and what did that cost?";
trickplay scrub previews; resume correct to the second.
