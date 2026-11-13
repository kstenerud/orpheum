> **ABOUTME:** The client-server audit of 2026-11-13 against principles §30: for each page and
> common press, the requests a person waits on, chained or parallel, against the ideal of one
> essential fetch per page; menus that fetch to learn what they list; what is asked for again
> though the client holds it; and where cosmetic loads compete with what a person waits on. Read
> before changing how a page reads the server. It proposes; the design is the owner's.

# The client-server audit, 2026-11-13

- **Asked by** the owner, 2026-11-13, after the alphabet rail's slow jump on Films (the plan file's
  "The alphabet rail's slow jump" and "The client-server audit",
  [navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md)).
- **The yardstick:** [principles §30](../../principles/30-one-request-before-the-person-acts.md)
  as the owner sharpened it: "Ideally one page does one 'essential data' fetch, and then other
  fetches for the rest as matches the situation", and "Even a menu's contents rarely require a
  fetch just to know what its contents should be. That should be the exception, not the norm."
- **Method:** read the web client (`web/apps/web`, `web/packages/sdk`, `web/packages/player`,
  `web/apps/cast-receiver`) and the core's API (`internal/core/api`) on the commit "docs(principles):
  §30, what a person waits on comes in one request and cosmetic data follows". Nothing was run on a
  device, staging or production; every count below is from the code, and the device measurements
  that would confirm the top findings are listed at the end. A "round trip" (RT) is one request a
  person waits on; away from home each is a WebRTC request on a data channel of its own (D273 §8,
  `web/packages/sdk/src/rtc-request.ts`).
- **Not repeated here:** the startup items of the
  [web architecture audit of 2026-11-10](web-architecture-audit-2026-11-10.md) still open (Startup
  7, the loader's manifest, refused versions and page read in a row, `loader-run-client.ts:174-181`;
  8, the worker's re-fetch of hashed files; 9, `modulepreload` of `client-main`'s chunks). They are
  §30 findings too and keep their numbers there.

## Where the architecture stands

The pieces §30 asks for mostly exist; they are not yet put together that way.

- **The server already answers much of what the client asks for next.** A list entry carries the
  rail's letter (`Letter`, `internal/core/api/item_list.go:50`) and the list's `Total`
  (`item_list.go:166`); the playback decision carries the tracks, subtitles, preferences, resume
  position and sound offset (`playback.go:112-119`); a series carries its seasons and next up; sign-in
  answers the user (`session.go:152`); a new cast answers its credential (`casts.go:62`).
- **The client often does not use what it was given.** Sign-in's user is dropped and `/me` is asked
  again; the rail asks the server for a letter's place though each held entry names its letter; the
  cast remote asks `/me/casts` for the credential `POST /me/casts` just answered.
- **There is no shared memory of answers.** `useResource` (`web/apps/web/src/use-resource.ts`)
  keeps an answer per component, not per key, so two components asking the same read ask twice, and
  a page left and come back to reads again. Only a film's and a series' page are remembered in the
  tab (`rememberedInTheTab`, `title.tsx:511`, `series.tsx`); Films, Series and a library's page are
  not, so Back from a poster reads the list again from its first page.
- **Pages are built of sections that each read their own.** Home is three reads, the System page
  about nine, the shell five or six after `/me`; each is also its own background poll (D237).
- **Lists are read as chains.** Every paged list reads its next page only once the one before has
  come (`use-pages.ts:236-322`, `sdk/src/pages.ts:24`), and the server caps a page at 500 (D416 §6,
  `item_list.go:156`, `pageLimitMax`), so "the whole list's text at once" is today 1 + ⌈(n − 60) / 120⌉
  round trips in a row on Films and Series.
- **Cosmetic loads are held back on the browser's HTTP path, but not on WebRTC.** Posters are
  `loading="lazy"` and the ones filled ahead are `fetchPriority = "low"`, two at a time
  (`poster.tsx:58,162-163`, `posters-filled-ahead.ts:18`): well done. Away from home every image goes
  through the service worker to the page's WebRTC transport, whose rooms (4 connections × 12,
  `rtc-transport-link.ts:17,47`) are handed out first come, first served (`rtc-transport.ts:527-560`):
  there the priority is lost, and a poster and the next page of the list wait in the same queue.
- **Menus mostly know their contents** (the player's subtitle, audio and quality menus read the
  playback decision), with ten exceptions listed below (finding 9), five of which should not be.

## One essential fetch per page: where each page stands

"Essential" is what a person waits on before they can act; "chain" is requests each waiting on the
one before. `/me` gates every signed-in screen (`app.tsx:167-173`), so every page below starts one
RT later than its own reads; it is counted once, in the first row.

| Page | Essential fetches now | What the one essential fetch would hold | Situational fetches |
| --- | --- | --- | --- |
| First load after sign-in (shell) | Chain of 2: `POST /session` → `GET /me`; then 4 parallel (`/client-settings`, `/libraries`, `/me/together`; an admin also `/needs-attention`, `/system/update`) beside the screen's own | `/me` (or the sign-in answer) carrying client settings, the libraries reached, the person's preferences (left-out libraries), their Watch Together sessions, and for an admin the needs-attention counts and update state | none needed; the polls of D237 |
| Home | 3 parallel (`/home`, `/items` newest films, `/items` newest series), after `/me` | `GET /home` with Continue Watching and both newest rows | posters as they near the view; tile interest |
| Films / Series | Chain of 1 + ⌈(n − 60)/120⌉ `/items` pages (9 for 1000 titles), plus `/items/letters` in parallel | the whole list's text in one answer, each entry with its letter (already sent), so the rail's letters come from it too | posters near the view; the genres only if the list is narrowed by one |
| A library's page | `/libraries` (shared, usually held) → `/items` page 1 (60), `/items/letters` in parallel; later pages as scrolled | as Films: the library's whole list's text in one answer | as Films; Folders view's `/libraries/{id}/folders` |
| A series' page | Chain of 2: `/items/{id}` → `/items/{id}/groups/{season}` page 1 | `/items/{id}` with the shown season's first page of episodes (the season returned to, or next up's) | other seasons as chosen; not-in-order episodes; posters, backdrop |
| An episode's page | Chain of 2: `/items/{id}` → `/files/{id}/up-next`, and `POST /playback` (warm-up, D258 §1) beside it | `/items/{id}` with up next and previous, and the resume position | the warm-up stays: it readies the start |
| A film's page | `/items/{id}`, then `POST /playback` (warm-up) for the Resume label | `/items/{id}` with each version's resume position | the warm-up, as now; art |
| Play (from a film page) | 0 more API reads: the warm-up's decision is used (well done); then the media chain (master playlist → media playlist → init → first chunk) | the decision holds the starting variant's playlist, or the first chunks are pushed with it | up next, asked once the film has played (`up-next.tsx:100`) |
| Play (from Continue Watching, a remote press) | `POST /playback`, then the media chain | as above | as above |
| A person's page | Chain of 2+: `/people/{id}` → `/items?person=` pages in a row (`credits.tsx:180-195`) | `/people/{id}` with their credited titles | people's photos, posters |
| Search (in Films/Series) | as Films, again per query after a 200 ms rest, plus `/items/letters` | as Films | — |
| Libraries (admin) | `/libraries`, though the shell holds it and polls it (`libraries.tsx:189-204`) | none: read the shell's | — |
| Settings (admin) | 3 parallel (`/config`, `/secrets`, `/system/update` if not shared) | `GET /config` with the secrets' names and set state | — |
| System (admin) | about 9 parallel: `/system`, `/system/update`, `/system/network`, `/now-playing`, `/system/resource-use`, `/backups`, `/config/unused`, `/filing-reports`, `/system/discovery` (+ `/system/videos` with debug tools) (`system.tsx:61-99`) | `GET /system` with each section's summary | a filing report's text as opened; the network check as asked |
| Users (admin) | 3 parallel (`/users` pages, `/invitations` pages, `/roles`) | `GET /users` with invitations and roles | — |
| A user's page (admin) | 4 parallel (`/me` again, `/users/{id}`, `/users/{id}/devices`, `/roles`) (`user.tsx:49-54,331`) | `/users/{id}` with their devices and the roles to choose from | — |
| Account | 3 parallel (`/me/devices`, `/me` again, `/me/own-link`) (`account.tsx:41-42,173`) | `/me` with the person's devices and own link | — |
| Needs a match (admin) | 2 parallel, each its pages in a row (`identify.tsx:212-230`) | one `/items?identification=` answer for both states | candidates as an item is opened (justified) |
| Cast menu | `/me/devices` on opening, again whenever the connected set changes (`cast-to.tsx:71-72`) | nothing: the live connection, which already says which devices are connected, says their names too | Google Cast and AirPlay discovery (the browser's) |
| Casting (the remote) | `POST /me/casts` → `GET /me/casts` (`cast-remote.tsx:245`), and `/files/{id}/up-next` twice (`cast-remote.tsx:420`, `cast-remote-choices.tsx:338`) | nothing more: the cast's answer and one up-next read | — |
| Watch Together, starting | Chain of 2: `POST /together` → `GET /together/{id}` (`watch-together.tsx:121,153-156`) | `POST /together` answering the session's details | voice's ICE as voice is turned on |

## Findings, ranked by cost to a person

Cost is how often the flow is used × how many round trips a person waits on in a row. Sizes are
rough: small (a day or less, one side), medium (both sides, or a new shape), large (a new mechanism).

### 1. Films and Series read their list's text as a chain of pages, and read it all again on Back

**Flow:** opening Films or Series; every Back from a film's or series' page to it; a letter on the
rail; a search.

**Requests, in order:**

1. `GET /me` (`app.tsx:167-168`) gates the screen.
2. `GET /items`, limit 60 (`browse.tsx:81,529-535`), and `GET /items/letters` (`alphabet-rail.tsx:151-168`) in parallel.
3. Then `GET /items`, limit 120, one after another until the list ends (`browse.tsx:87,529`;
   `useReadWhole`, `use-pages.ts:397-408`; each started by `useLoadMore` only once the page before is
   shown in pieces, `use-pages.ts:236-275`). 1000 titles: 8 more in a row; 2000: 17.
4. **Back:** the list is not remembered (no `rememberedInTheTab` in `browse.tsx` or
   `use-pages.ts`), so steps 2 and 3 run again, and `useListLoadsToFocusNoted`
   (`input-layer.tsx:398-411`) waits on that chain (`loadUntil`, `use-pages.ts:284-320`) to give the
   poster pressed its focus back.
5. **A letter:** `GET /items?from=…&limit=1` (`alphabet-rail.tsx:363-371`), then `loadUntil` waits on
   the read in flight and pages on to it (`alphabet-rail.tsx:374`) — the case that raised §30.

All critical (names, order, places); the posters are the cosmetic part and already wait for view.

**Proposed** (the plan file's recommendation, sharpened by what the server already sends):

- **Server:** a list by name answers the whole list's text in one request: either the page cap
  lifted for a list by name (D416 §6 amended), or a lean shape (`id`, `kind`, `name`, `year`,
  `letter`, the art hashes, the watched marks) with no cap; split in two or three large parts only
  when measured too big (§30's last point).
- **Client:** a jump finds the letter among the held entries by their `letter` (already in every
  entry by name, `item_list.go:50`), with no request; the rail's letters are the held entries'
  letters, so `/items/letters` and its poll go for a list read whole.
- **Client:** Films' and Series' read is kept in the tab, as a film's page is, and shown at once on
  Back while the poll reads it again (D237), so Back restores focus from memory.

**Size:** medium (server shape + client); the jump from held letters alone is small and could go
first, as the list is already read whole.

### 2. A library's page pages as it scrolls, so its jump and Back are a longer chain still

**Flow:** a library's own page, by name (`library-browse.tsx`).

**Requests:** `GET /items` limit 60 (`library-browse.tsx:71,491,535`) and `/items/letters` in
parallel; later pages only as the person nears the end (`usePages` without `isReadWhole`,
`library-browse.tsx:540`). A letter: `GET /items?from=…&limit=1`, then 60-title pages in a row up to
it (the plan file's diagnosis). Back: the same chain from page 1 to the poster pressed.

**Proposed:** as finding 1, the list's whole text in one answer on every list with a rail (the plan
file already recommends it); the Folders view (`library-browse.tsx:668-690`, its pages in a row by
`allPages`) the same way. **Size:** small once finding 1's shape exists.

### 3. Every screen waits on `/me`, and sign-in's own answer is thrown away

**Flow:** every first load; every sign-in; every return to a tab after a path move
(`me:${pathMoveCount}`, `app.tsx:167`).

**Requests, in order:**

1. Under a loader: `GET /client-manifest` → the refused versions → `GET /` (Startup 7 of 11-10).
2. Sign-in: `POST /session` answers `user` (`session.go:152`); `signInWith` keeps only its token and
   device (`accounts.ts:170-175`), and `SignedIn` asks `GET /me` again (`app.tsx:167-168`).
3. `GET /me` gates everything (`app.tsx:170-172`: "Loading…").
4. Then in parallel: `/client-settings` (`app.tsx:281-282`), `/libraries` (`app-shell.tsx:316`),
   `/me/together` (`app.tsx:216`, `together-mine.ts:128`), and for an admin `/needs-attention`
   (`status-tray.tsx:233-234`) and `/system/update` (`update.tsx:65-73`), beside the screen's own.
5. `checkSessionWhileShown` asks `/me` again each minute while no live connection is open, and as the
   page is shown (`client-main.tsx:101-104`, `signed-out-watch.ts:56`).

Critical: who the person is, what they may do, the libraries (the banner's menu). The rest is
needed soon, not first.

**Proposed:**

- **Client (small):** sign-in hands its `user` to `SignedIn` as the first `me`, so the first screen
  after a sign-in makes no `/me`.
- **Client (small):** the screen's own read starts with `/me`, not after it: the token is known, and
  a refusal signs out as now (`signed-out-watch.ts`).
- **Server + client (medium):** `/me` (and the sign-in answer) carries the client settings, the
  libraries reached, the person's preferences, their Watch Together sessions, and for an admin the
  needs-attention counts and update state: one essential fetch for the shell, each kept fresh by the
  live connection or its poll as now.

### 4. Home is three reads and three polls where one would do

**Flow:** Home, the first screen for most starts, and the most visited.

**Requests:** after `/me`, in parallel: `GET /home` (Continue Watching only, `continue-watching.tsx:28`,
`home.go:32`), `GET /items?sort=added&limit=18&home=true` for films and again for series
(`home.tsx:77-82`). Each is polled on its own (`continue-watching.tsx:250-254`, `home.tsx:83`), so
each poll turn is three requests. The rows appear as each answers, so the page moves under a remote's
focus as they come.

**Proposed:** `GET /home` answers all three rows (and any later Home row), one read and one poll.
Server and client, **small to medium**.

### 5. A series', an episode's and a film's page each wait on a second read

**Flows and requests:**

- **Series:** `GET /items/{id}` (`series.tsx:701-712`) → the shown season's
  `GET /items/{id}/groups/{season}` page 1 (`series.tsx:526`), mounted only once the series has come.
  The episodes are what a person picks from.
- **Episode:** `GET /items/{id}` (`title.tsx:543-555`) → `GET /files/{id}/up-next`
  (`title.tsx:564`, `around-file.ts:32-45`), and the warm-up `POST /playback` (`title.tsx:518-534`)
  beside it.
- **Film:** `GET /items/{id}` → the warm-up, whose answer gives the Resume label
  (`watching.ts:355-361`); the item body has no resume position.
- **Person:** `GET /people/{id}` → `GET /items?person=` pages, in a row inside one load
  (`credits.tsx:180-195`).

**Proposed:** `/items/{id}` for a series carries the shown season's first page of episodes (the
season returned to is the client's, so it names it: `?season=`); for an episode, up next and previous;
for a film or episode, each version's resume position. `/people/{id}` carries the credited titles, or
the two reads start together. The warm-up stays (D258 §1): it readies the start, it is not what the
page waits on. **Size:** medium (server shapes), small for the person page's parallel start.

### 6. Away from home, posters and the next page of the list wait in one queue

**Flow:** any list or row over WebRTC, above all Films reading its whole list while its posters load.

**Requests:** each visible poster's `<img>` and the two filled ahead go through the service worker
to the page's transport (`sw.ts:137-151`, `mediaPathPrefix`, `rtc-media.ts:130`); the transport
hands a room to whichever request claims it first when one frees (`rtc-transport.ts:520-560`), API
reads and images alike. The browser's `fetchPriority = "low"` (`poster.tsx:163`) does not reach
this queue. Tile interest (`PUT /interest/tiles`, `tile-interest.ts`) and page interest
(`PUT /interest/page`) ride the same queue.

Critical: the list's pages, a press's request. Cosmetic: the images.

**Proposed (client, medium):** the transport takes a priority with each request, the API's above
the images', and gives a freed room to the highest waiting; images (and the interest notes) keep a
few rooms at most while an API read waits. A per-request priority to the core's side of the channel
(its credit, D273 §8) is a later step if measurement shows the core's sending, not the client's
queue, is where they meet.

### 7. Starting playback is a chain of media requests after the decision

**Flow:** every start, and every quality switch's first load.

**Requests:** the decision (`POST /playback`, from the warm-up or `play-opened.ts:127`), then hls.js
reads the master playlist, the variant's playlist, its init segment and the first chunk, each
waiting on the one before (`web/packages/player/src/chunked.ts:708`). Away from home that is four
WebRTC requests in a row before the first frame, beside the core's own work to make the first chunk.

**Proposed (medium to large, to measure first):** the decision carries the master and the starting
variant's playlists (the player hands them to hls.js as loaded), so the start is decision → init and
first chunk together; or the core begins the init segment's answer with the decision. Measure how
long each step takes away from home first: if the core's first chunk dominates, this saves little.

### 8. Asking for what the client already holds

| Read | Where | Already held by | Proposed |
| --- | --- | --- | --- |
| `GET /me` after sign-in | `app.tsx:167` | `POST /session`'s `user` | finding 3 |
| `GET /me` | `browse.tsx:617` (an empty list), `account.tsx:42`, `user.tsx:49`, `together-join.tsx:311` | the shell's `me` | a `MeContext` from `SignedIn` |
| `GET /libraries`, polled | `libraries.tsx:189-204` | the shell's `LibrariesReachedContext`, polled too | read the shell's (one poll), as Home already does (11-10 Startup 10) |
| `GET /me/casts` | `cast-remote.tsx:245` | `POST /me/casts`'s answer, with its credential | hand the cast's answer to the remote; ask only after a reload |
| `GET /files/{id}/up-next`, twice | `cast-remote.tsx:420`, `cast-remote-choices.tsx:338` | each other | one read for both (`useAroundFile` shared, or a key-shared `useResource`) |
| `GET /roles` | `role-dialog.tsx:45` | the Users page's `roles` (`users.tsx:91`) | pass them in |
| `GET /needs-attention` | `needs-attention.tsx:149` | the tray's (`status-tray.tsx:233`), an admin's | share the shell's |
| `GET /together/{id}` after `POST /together` | `watch-together.tsx:153` | — (the create answers only the id and credential) | `POST /together` answers the details |

The general fix is one: **`useResource` answers by key from a tab-wide memory** (the latest answer
shown at once, read again behind it), so the same read in two places is one request and a page
come back to draws at once. **Size:** medium (one mechanism; every caller benefits).

### 9. Menus and dialogs that fetch to learn what they list

The owner's rule: a menu's contents come with the page's essential data; a fetch on opening is the
exception, for contents that change by the second or that come from outside.

| Menu or dialog | Fetch on opening | Verdict | Proposed |
| --- | --- | --- | --- |
| Films' and Series' genre chips (`listing-tools.tsx:118`, `list-chips.tsx:25-45`) | `GET /genres`, its pages in a row | **should move** | the list's essential answer names the genres it has (it changes only with a scan) |
| A library's choices (`library-configure.tsx:89-90`) | `GET /me/preferences` | **should move** | the preferences come with `/me` (finding 3) |
| The Cast menu (`cast-to.tsx:71-72`) | `GET /me/devices`, again as the connected set changes | **should move**, mostly: which are connected changes by the second, but the live connection already says so (`useLive().connectedDeviceIds`) | the live connection names the connected devices too; the menu then fetches nothing |
| A role's dialog (`role-dialog.tsx:45`) | `GET /roles` | **should move** (held by the page, finding 8) | pass them in |
| The sound offset control of a Watch Together guest (`sound-delay.tsx:160-168`) | `GET /me/devices` for one number | **should move** | `/me`'s `device` carries its sound offset (a host's playback decision already does, `playback.go:119`) |
| A filing report (`filing-report.tsx:144`) | `GET /filing-reports/{name}`, its text | justified: a large text, opened rarely | — |
| Subtitle search (`subtitle-search.tsx:65`), a match's candidates (`candidate-chooser.tsx:170`) | the provider's search | justified: an outside service, asked by the press | — |
| The folder picker (`folder-picker.tsx:55`) | `GET /nodes/{name}/folders` | justified: the disk as it is now | — |
| The levelling menu's waits (`file-waits.ts:31`) | `GET /files/{id}/waits`, repeated | justified: progress, by the second | — |

The player's subtitle, audio and quality menus fetch nothing (the decision holds them): kept.

### 10. Admin pages built of sections that each read their own

System (`system.tsx:61-99`): about nine parallel reads, each a section's own (table above), each
polled where it polls; Settings three; a user's page four, one of them `/me` again; Users three, two
of them their pages in a row. Used rarely and at home, so ranked low; but over WebRTC nine data
channels open at once for one page. **Proposed:** `GET /system` with each section's summary (the
network check, which takes time, stays its own); `GET /users/{id}` with the devices and the roles to
choose; `GET /config` with the secrets' state. **Size:** medium, server mostly.

### 11. Code before data on a first visit to a film's or series' page

`Title` and `Series` are lazy screens (`later-screens.tsx:41-45,151-152`): on a page's first visit
since a release the chunk loads, then the screen mounts, then `/items/{id}` is asked. The worker keeps
the client's files by hash (`sw.ts`), so after the first visit the chunk costs nothing; Play's code is
loaded ahead on idle (`usePreloadsPlay`, `later-screens.tsx:199-215`) and these are not.
**Proposed (small):** preload `Title`'s and `Series`' code on idle after Home, as Play's is; or start
the item read at the press, beside the chunk. Words are compiled into the chunks
(`web/apps/web/src/locales/*.ts`, `addWords`), so no locale file is fetched before the first screen;
the 11-13 move of the settings words (`home-imports.test.ts`) keeps them out of the start, and the
plan file's next step (`en-play.ts`, `en-cast.ts`, …) continues it.

### 12. Polls that read whole answers (note only; D237 governs them)

- A film's and a series' page poll the whole `/items/{id}` (`title.tsx:555`, `series.tsx:712`), and
  each season shown polls its pages (`series.tsx:532`).
- Films polls the two pages in view and `/items/letters` (`use-pages.ts:156-167`); with finding 1,
  the letters' poll goes and the list's poll could ask "changed since" rather than re-read pages.
- Home's three polls (finding 4); the shell's `/libraries` and the Libraries page's (finding 8).
- `/me` each minute without a live connection (`signed-out-watch.ts:56`).

A poll is not waited on, but away from home each turn is several WebRTC requests sharing the queue
of finding 6. A "changed since" token per answer (the server says nothing has changed in a few
bytes) would make every poll small; that is D237's to decide.

## Done well, to keep

- **Play from a film's page asks the server nothing more**: the warm-up's decision is used
  (`watching.ts:226-361`), and the decision holds every track, subtitle, preference, the resume
  position and the sound offset, so the player's menus fetch nothing.
- **Posters never block**: lazy images, a blank with its place drawn for each entry to come (from
  `Total`), two filled ahead at a low priority, a window of 100 each way (`posters-filled-ahead.ts`).
- **A page's rows draw in pieces** so a held arrow is never stuck behind 60 posters
  (`use-pages.ts:252-264`).
- **Reads that can go together do**: create library's `/nodes` and `/config` (`create-library.tsx:533`),
  Needs a match's two lists (`identify.tsx:215`).
- **A film's and a series' page draw at once when come back to** (`useRemembered`,
  `rememberedInTheTab`); the hosted session's details are read ahead (`hosted-details.ts:44-55`).
- **Genres are not read on every visit** — only when the chips open (`list-chips.tsx:20-24`); finding
  9 moves them into the list's answer rather than back to an eager read.
- **One poller for the page's life**, its waits from the server (`app.tsx:290-300`, D237), and the
  libraries read once for the shell and Home.

## Measurements worth taking on a device

Each away from home (WebRTC, through the loader) and at home, on the Fire TV and a phone, with the
owner's largest library:

1. **Films' whole read:** the number of `/items` requests and each one's time from the first to the
   last page; the time from a far letter's press (S, T) to the poster focused; the time from Back to
   the pressed poster focused. Confirms findings 1 and 2, and sizes one answer's bytes (compressed)
   for the whole list's text.
2. **The first screen after a start:** the time of each step, manifest → page → scripts → `/me` →
   the shell's reads → Home's three, to see how much of the start is the `/me` gate (finding 3).
3. **A series' page:** the time from the press to the episodes shown, split at the series' answer
   (finding 5).
4. **The WebRTC queue under posters:** while Films reads its pages and its posters load, each API
   request's time waiting for a room against its time on the wire (`rtc-transport.ts`'s claim),
   to show whether finding 6 is the queue or the core's sending.
5. **A start's media chain:** the decision, master playlist, variant playlist, init and first
   chunk, each one's time, from Continue Watching away from home (finding 7).
