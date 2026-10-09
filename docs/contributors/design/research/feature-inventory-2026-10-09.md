> **ABOUTME:** (2026-10-09) Everything planned, designed, decided, deferred or merely wanted that
> is not in M5, by area, with the UI surfaces each will need, so the owner can check the feature
> list and M5's UI can leave room for what comes after it.

# Feature inventory beyond M5 (2026-10-09)

The owner asked for two things: one list of every feature the project knows about beyond M5, to
check he is happy with it, and, for each, what the user interface will need, so that the polished
UI M5 builds ([m5.md](../plans/m5.md), cut-a's M5 row, [D245](../../decisions/working-notes.md)) does not have to be torn up to take
them.

**How it was made.** Five read-only sweeps ran in parallel. Four read the decision log in four
ranges (D1–D87, D88–D162, D163–D207, D208–D270). The fifth read
[plan-deviations.md](../../plan-deviations.md), the overview and the UI and voice research. The
coordinator read [FEATURES.md](../../../FEATURES.md), [ROADMAP.md](../../../ROADMAP.md),
[cut-a.md](../plans/cut-a.md), [m5.md](../plans/m5.md) and the user-facing plans directly. It also
read the owner's wishes kept in the agent's memory outside the repository; items found only
there are marked *memory*.

**What counts as M5's.** Two lists: cut-a's M5 row, and m5.md's "Built toward it already" and
"Handed on from M4". Items m5.md hands on whose milestone is still an open owner question are
listed apart, in §1, so they are not lost either way. Everything Cut A built in M0–M4 is left out.

**The columns.**
- **Status:**
  - *planned*: named for a milestone;
  - *designed*: decided, with a plan or decision text, and no milestone;
  - *deferred*: wanted later, little or no design, no milestone;
  - *idea*: mentioned, not decided;
  - *research-only*: proposed in research and adopted by no decision or plan.
- **Milestone:** what the documents name (M6, "TV", a ROADMAP phase), or "—".
- **UI surfaces:** what the feature will need on screen, which is the point of this document.
- **F-numbers** are [FEATURES.md](../../../FEATURES.md)'s.

Within each group, rows run planned, designed, deferred, idea.

## 1. On M5's edge: handed on, milestone undecided

These are in m5.md's lists, but its owner questions 11 and 12 (or another document) leave open
whether M5 builds them.

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| E1 | **Get this ready** | Ask the server to prepare a film before watching it (`POST /files/{id}/prepare`) | [P36](../../plan-deviations.md); m5.md "Handed on"; cut-a M6 row | planned | **M6 in cut-a, M5 in m5.md** (they disagree) | an action on the title page; a "getting ready / ready" state on the title page and in the job view |
| E2 | Next Up | The next episode of each series in progress, as a row and on the series page | [P27](../../plan-deviations.md); [play-state.md](../plans/play-state.md); m5.md | designed | Cut A, after M3 (owner, 2026-09-26) | a Home row; a "next" card on the series page; it is also the cast queue ([D135](../../decisions/working-notes.md)) |
| E3 | Per-person volume in Watch Together | A volume for each other participant and one for the film, for everyone; the device's own buttons stay the master | [watch-together.md](../plans/watch-together.md) "Open"; m5.md Q12 | idea (owner's want; not a decision yet) | M5 or the next | a small mixer on the Watch Together play page, phone-sized |
| E4 | Viewer subtitles, audio track and levelling | A viewer chooses their own subtitles, sound track and levelling ([D188](../../decisions/working-notes.md) §4) | watch-together.md "Later"; m5.md Q12 | designed | M5 or the next | the track panel on the viewer's player |
| E5 | Subfolder picker for a new library | Choose a subfolder of a node's root, not only the whole root | m5.md "Handed on"; *memory* `library-folder-picker-later.md` | planned (needs a node-side listing, [D193](../../decisions/working-notes.md) §1) | M5? | a folder tree or browser in the new-library form |
| E6 | Recently added | A Home row of new arrivals | [P27](../../plan-deviations.md) | deferred | — | a Home row |
| E7 | Keyboard arrow navigation in grids and rows | Arrow keys move through tiles; the same mechanism later serves a TV remote | m5.md Q5; [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) R7 | open question | M5 or the TV milestone | one roving-focus module under Grid and Reel |
| E8 | Resume banner, poster size, title-page tint, theming seam | as named | m5.md Q1, Q7–Q9; [D138](../../decisions/working-notes.md) §4 | open questions | M5 if yes | Home banner; a density setting; the title page's background; tokens as a public surface |

## 2. Playback and transcoding

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| P1 | Session-start prepare of `h264` | A Watch Together film is made playable for the remote TV as the session starts | cut-a M6 row | planned | M6 | a "getting ready" state in the host's session screen |
| P2 | Loudness levelling on the cast receiver | The cast receiver levels sound as the web player does | [D226](../../decisions/working-notes.md) §4; cut-a M6 row | planned | M6 | the cast receiver's player (the levelling note it shows) |
| P3 | Skip intro, recap and credits; next-episode prompt placed on the real ending; multi-episode files | A skip button only when it applies; the next-episode card appears where the programme ends, never on a timer; "next episode" inside one file is a seek | [D82](../../decisions/working-notes.md), [D115](../../decisions/working-notes.md), [D139](../../decisions/working-notes.md), [D159](../../decisions/working-notes.md); [media-segments.md](../plans/media-segments.md) | designed (detector out of Cut A) | — | a skip button over the picture; a next-episode card; marks on the scrub bar; an admin segment editor (a user override beats any detector); detection progress in the job view |
| P4 | Scrub-bar previews (trickplay, F25) | Frame thumbnails while scrubbing, from keyframes alone | [D102](../../decisions/working-notes.md), [D114](../../decisions/working-notes.md), [D188](../../decisions/working-notes.md) §1; FEATURES F25 | deferred (owner's intended method recorded) | — | a thumbnail popover on the scrub bar (mouse, touch and remote) |
| P5 | Whole length known and a seek anywhere during a prepare (F96) | A prepared film shows its full length and can be sought anywhere at once | [P40](../../plan-deviations.md); FEATURES F96; [D193](../../decisions/working-notes.md) §2 | deferred (owner's want). The chunked player ([D247](../../decisions/working-notes.md), [D260](../../decisions/working-notes.md)) may already meet most of it; not checked | — | the scrub bar |
| P6 | Player controls as streaming services have them (F97) | Full length, previewing scrub bar, touch-sized controls | [P40](../../plan-deviations.md); FEATURES F97 | deferred; may fold into M5's player skin | M5? | the control bar on phone and desktop |
| P7 | Manual override of the playback decision | "Play the original" or "play a prepared copy", against the automatic choice | [web-client.md](../plans/web-client.md) "Non-negotiables"; [D2](../../decisions/working-notes.md) | designed | — | an entry in the player's info panel or quality menu |
| P8 | Fallback ladder and the device's failure memory | When a decoder fails, playback drops to the next rendition and says so; the device remembers, and the household sees the pattern | [D92](../../decisions/working-notes.md) | designed | — | a notice in the player; a "forget this device's playback memory" control; an admin view of failures by device |
| P9 | Chapters in the player (F61) | Named chapter marks to jump between | [media-segments.md](../plans/media-segments.md); FEATURES F61 | deferred (modelled) | — | chapter ticks on the scrub bar; a chapter list |
| P10 | Live bandwidth transcoding and a wider output menu | HEVC, AV1, HDR and more rungs made live for a weak link; distinct from Cut A's prepare and chunks | [D4](../../decisions/working-notes.md) (b), [D21](../../decisions/working-notes.md), [D86](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §2; [transcoding.md](../plans/transcoding.md) §(b) | deferred | ROADMAP Phase 4 | a quality menu with "Auto" and the reason for the current choice |
| P11 | Compatibility transcoding to a stored version | Old or odd files converted once in the background, then direct-play for ever | [D19](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §3; transcoding.md §(a) | deferred | ROADMAP Phase 4 | an admin "convert" action and its jobs; the extra version listed on the title page |
| P12 | Burned-in and server-rendered subtitles, bitmap subtitles (PGS) | Image subtitles shown, or fansub styling rendered by the server; the player says what that choice costs | [D23](../../decisions/working-notes.md), [D78](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §2, §4 | deferred (cut-a "Out") | — | a label in the subtitle menu ("image; size and colour controls unavailable"); a "prefer rendered" preference |
| P13 | Subtitle verification and automatic fetching | A missing subtitle is fetched and checked against the audio; "could not be verified" said in those words; the quota shown before it is spent; automatic fetch is a person's opt-in | [D147](../../decisions/working-notes.md), [D148](../../decisions/working-notes.md), [D217](../../decisions/working-notes.md) §9, [D223](../../decisions/working-notes.md) §4; [subtitle-acquisition.md](../plans/subtitle-acquisition.md) | deferred | — | verified and unverified badges in the track panel; the measured offset shown; a quota line; a personal "fetch automatically" setting; an admin report of subtitles that do not match their audio |
| P14 | Embedded subtitle fonts as an opt-in | A track's own fonts are used only if the person agrees, per series, and a device may refuse with a reason | [D56](../../decisions/working-notes.md), [D57](../../decisions/working-notes.md) | designed (whether Cut A built it was not checked) | — | a one-time prompt in the player; a three-state choice in the subtitle menu; a per-device override |
| P15 | Forced subtitles and "say why" | Signs-and-songs subtitles turn on by themselves; a pre-chosen track says why it was chosen | [D23](../../decisions/working-notes.md), [D24](../../decisions/working-notes.md) | designed | — | a reason line in the track panel |
| P16 | Language defaults per person and per library | An ordered list of audio and subtitle languages, and a library default ("anime: Japanese audio, English subtitles") | [D24](../../decisions/working-notes.md) | designed | — | the person's settings; the library's settings; where each value came from ([D188](../../decisions/working-notes.md) §1) |
| P17 | Tracks chosen before play | Audio and subtitles picked on the title page | [D138](../../decisions/working-notes.md) §7 | designed | — | track pickers on the title page |
| P18 | Audio description as a kind of track | "Prefer audio description" is a setting, not a track number | [D90](../../decisions/working-notes.md); [accessibility.md](../plans/accessibility.md) | designed | — | the audio menu; a preference |
| P19 | Dynamic range per device; finer dialogue boost | Night mode or full range for this device; boost in steps or a stronger mix, not only on and off | [D83](../../decisions/working-notes.md), [D103](../../decisions/working-notes.md), [D184](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §2, [D222](../../decisions/working-notes.md) §4; [audio-presentation.md](../plans/audio-presentation.md) "Later" | designed (DRC); deferred and idea (finer boost) | — | the player's sound menu (the "how does this device play sound?" question already exists); device settings |
| P20 | Playback speed (F46) | Faster or slower playback; local, unless a Watch Together group shares it | [D90](../../decisions/working-notes.md), [D66](../../decisions/working-notes.md); FEATURES F46 | designed | — | a speed menu in the player; in Watch Together, a group rate (watch-together.md "Open") |
| P21 | External player handoff (F59) | "Play in VLC" with a short-lived link | [D140](../../decisions/working-notes.md) §5, [D154](../../decisions/working-notes.md) §2; [tv-client.md](../plans/tv-client.md) | designed | TV | a title-page action; on TV, an intent |
| P22 | A second engine, libmpv (native clients) | "Play anything" fallback beside the platform decoder, switchable | [D10](../../decisions/working-notes.md) | designed | TV | a player setting on native clients |
| P23 | Styled subtitles in our own fullscreen | ASS styling kept in fullscreen (not on iPhone) | [D254](../../decisions/working-notes.md) §3 | deferred | — | the fullscreen control |
| P24 | Surround kept in prepared copies | A receiver keeps its channels | [D222](../../decisions/working-notes.md) §2 | deferred | — | none new |
| P25 | Shuffle, sleep timer, picture-in-picture and background audio (F55–F57) | as named | [D149](../../decisions/working-notes.md) §7; FEATURES F55–F57; [web-client.md](../plans/web-client.md) | deferred | — | a shuffle action on series and collections; a sleep-timer menu; a picture-in-picture button |
| P26 | Dual subtitles | Two subtitle tracks at once | [D24](../../decisions/working-notes.md), [D138](../../decisions/working-notes.md) §7 | idea (the schema allows it) | — | multi-select in the subtitle panel; two subtitle layers |
| P27 | Piecewise subtitle alignment | Subtitles for ad-break rips or other cuts | [D147](../../decisions/working-notes.md) | idea | — | none new |

## 3. Watch Together and voice

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| W1 | Her TV plays, her phone talks | The remote person's TV plays the film while her phone carries voice and remote control | [D188](../../decisions/working-notes.md) §3, [D245](../../decisions/working-notes.md), [D267](../../decisions/working-notes.md) §3; cut-a M6 row | planned | M6 | a TV playback mode (TV browser or cast receiver, 10-foot); a phone companion page with voice and transport and no video |
| W2 | Guests given controls; rights per invite link | Play, pause, seek, next and previous given per link, the last link's rights becoming the next one's defaults | [D246](../../decisions/working-notes.md) §8, [D268](../../decisions/working-notes.md) §5 | deferred (D268 §4 in M5 is the seam) | — | checkboxes in the invite dialog; a guest's control bar showing only what they may do |
| W3 | A signed-in guest's viewing kept as their own; avatars; inviting members in the app | History advances for a signed-in guest; pictures on the roster; a member invited without a link | [D246](../../decisions/working-notes.md) §4, [D261](../../decisions/working-notes.md) §3; [P46](../../plan-deviations.md) | deferred | — | avatars in the roster and menus; an invitation card on the invitee's Home (an in-app notice, see §12) |
| W4 | Undo for a seek, named events, a reconnecting state | "Karl skipped ahead 2 min, undo"; who paused | [D188](../../decisions/working-notes.md) §1; watch-together.md "Later" | designed | — | a notice strip on the play page with an undo button |
| W5 | Text chat | Typed messages in a session | [D188](../../decisions/working-notes.md), [D246](../../decisions/working-notes.md) §5; watch-together.md "Later" | deferred (and declined by [D63](../../decisions/working-notes.md), [D69](../../decisions/working-notes.md), see §12) | — | a chat panel, which must fit beside the film on a phone |
| W6 | Play sets and auto-next in a session | The host picks a season or trilogy and the group moves through it; the next episode follows | [D121](../../decisions/working-notes.md), [D135](../../decisions/working-notes.md), [D246](../../decisions/working-notes.md) §5 | deferred | — | a session queue; next and previous for whoever has the right |
| W7 | Groups of four or five, a LAN party of about ten | Larger sessions, with host-only control, a stall policy and remote parties gated on adaptive bitrate | [D69](../../decisions/working-notes.md), [D123](../../decisions/working-notes.md), [D250](../../decisions/working-notes.md) §2; ROADMAP Phase 6b | designed in part; above two "not designed" (watch-together.md "Open") | Phase 6b | a roster that scales; host control settings; health per member |
| W8 | Offset alignment against echo | One button that puts the speaker-side player ahead by the voice delay | [D65](../../decisions/working-notes.md); watch-together.md "Open" | open (whether still wanted) | — | a button and a nudge in the voice controls |
| W9 | Voice on a second device of the same person; film paced per person | Voice on the phone while the computer or TV plays, as one member | [D120](../../decisions/working-notes.md), [D267](../../decisions/working-notes.md) §3; watch-together.md "Prepared for" and "Open" | designed (seam built); not finished | M6 | joining a session from a second device; nothing may assume voice and film share a page |
| W10 | Native-app voice | A capped jitter buffer, Opus DRED and per-client bounds | watch-together.md "Prepared for"; [D262](../../decisions/working-notes.md) | idea | native apps | none new |
| W11 | Push-to-talk, ducking, headphone or speaker choice, a pre-prompt before the microphone | as named | [D63](../../decisions/working-notes.md) (push-to-talk an idea; ducking designed); [voice-apps.md](voice-apps.md) | idea / research-only | — | voice settings; a roster showing who is speaking or muted; a join step before the browser's prompt |
| W12 | Emoji reactions | as named | [D63](../../decisions/working-notes.md) (mentioned) | idea | — | an overlay on the picture |

## 4. Library and metadata

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| L1 | Collections as sequences (F17) | Hand-made, provider (TMDB franchise) and rule-made collections, ordered and nested; Next Up and progress over a franchise as over a season | [D75](../../decisions/working-notes.md), [D79](../../decisions/working-notes.md), [D159](../../decisions/working-notes.md), [D160](../../decisions/working-notes.md) | designed (cut-a "Out") | — | a collection page with progress and a "next" card; a collection editor with reordering; "add to collection" on titles |
| L2 | Episode orderings (F18) | Aired, DVD or absolute order per person; the order files are numbered in, per series | [D96](../../decisions/working-notes.md), [D159](../../decisions/working-notes.md), [D210](../../decisions/working-notes.md) | designed | after Cut A | an ordering switch on the series page; an admin series setting |
| L3 | People and credits | Actor and crew pages; same-name people kept apart; duplicates merged | [D95](../../decisions/working-notes.md), [D185](../../decisions/working-notes.md) §1 | designed | — | a person page; credits on the title page; an admin merge |
| L4 | Critic and audience scores (F40) | IMDb, Rotten Tomatoes, Metacritic and TMDB scores with vote counts | [D149](../../decisions/working-notes.md) §3, [D171](../../decisions/working-notes.md), [D172](../../decisions/working-notes.md) | designed | — | a score badge on posters; a ranked list on the title page; per-library score sources; sort by score |
| L5 | External lists (F41) | A collection fed by an IMDb, Trakt or Letterboxd list, refreshed on a schedule, showing how many members are missing | [D149](../../decisions/working-notes.md) §4, [D176](../../decisions/working-notes.md), [D187](../../decisions/working-notes.md) §3; [external-lists.md](../plans/external-lists.md) | designed in part (plan UNSPECIFIED) | — | a "from a list" source in the collection editor; a missing-members count (hidden from restricted roles); the schedule shown |
| L6 | NFO write and export (F29) | Corrections written beside the media, with lock markers, so leaving is supported | [D98](../../decisions/working-notes.md), [D141](../../decisions/working-notes.md), [D142](../../decisions/working-notes.md), [D143](../../decisions/working-notes.md), [D182](../../decisions/working-notes.md) §1 | designed (cut-a "Out") | — | the library's write permission; an export dialog choosing whose watch state; a per-root result report; an "NFO changed on disk" notice |
| L7 | The full metadata editor | Every field, artwork and identification editable, never overwritten by a refresh; phones later | [D140](../../decisions/working-notes.md) §4, [D170](../../decisions/working-notes.md); web-client.md | designed (Cut A has manual identify and overrides) | — | an edit page with each field's source and lock; an artwork chooser; a phone layout |
| L8 | Editions by hand; extras attached to an edition; merging titles | A user-named cut; deleted scenes filed under the cut they belong to; two titles proved one | [D158](../../decisions/working-notes.md), [D162](../../decisions/working-notes.md) §3, [D173](../../decisions/working-notes.md) §3–§4 | designed | — | actions in the metadata editor; a merge dialog |
| L9 | Retention surface (F43) | Candidates for removal gathered in a visible collection with a grace period, nothing deleted | [D75](../../decisions/working-notes.md), [D144](../../decisions/working-notes.md), [D149](../../decisions/working-notes.md) §6 | designed | — | a retention collection page with a countdown per item |
| L10 | Deletion (F50) and the trash | Two-phase delete naming every file, its size and how many devices hold a copy; a trash per root that never empties itself | [D89](../../decisions/working-notes.md), [D144](../../decisions/working-notes.md), [D145](../../decisions/working-notes.md), [D167](../../decisions/working-notes.md) | deferred (revisit on a real request) | — | a delete flow with a confirmation that lists files; a trash view with restore |
| L11 | Folder browsing (F58) | A tree of files, each showing what it is attached to and how | [D176](../../decisions/working-notes.md), [D179](../../decisions/working-notes.md) §4 | deferred | — | an admin file tree beside "what the scanner saw" |
| L12 | Library behaviours | Archival libraries; deferrable passes per library; progress per pass; "play this now" jumping the queue; a "rescan this path" button and webhook; "rescan to find missing media" | [D32](../../decisions/working-notes.md), [D94](../../decisions/working-notes.md), [D102](../../decisions/working-notes.md), [D118](../../decisions/working-notes.md), [D241](../../decisions/working-notes.md) §8, [D255](../../decisions/working-notes.md) | designed | — | library settings; per-pass progress on the library page; a priority action on a title; root actions |
| L13 | Duplicates | Files with the same payload named together | [D116](../../decisions/working-notes.md), [D179](../../decisions/working-notes.md) §4 | designed | — | a filter in the admin file views |
| L14 | Missing episodes (F66) | What a season lacks, never offered to get | [D187](../../decisions/working-notes.md) §3 | deferred | — | greyed episode rows |
| L15 | Artwork: types, embedded, season posters, NFO image links | Logos and banners (F51); art inside files; season posters; whitelisted image URLs from NFOs | [D149](../../decisions/working-notes.md), [D212](../../decisions/working-notes.md) §9, [D215](../../decisions/working-notes.md) §2, §4 | deferred / idea | — | a logo on the title page and TV hero; season art |
| L16 | Lyrics (F45) | Timed lyrics | FEATURES F45 | deferred | with music | a lyric view in an audio player |
| L17 | Content safety | Outlier files reported in concrete words, quarantined from default play | [D61](../../decisions/working-notes.md); [content-safety.md](../plans/content-safety.md) | deferred | — | an item badge; an admin review list |
| L18 | Music (F92) | Music libraries, albums, artists, playlists that repeat songs, "my music through the TV" | [D1](../../decisions/working-notes.md), [D176](../../decisions/working-notes.md), [D186](../../decisions/working-notes.md); ROADMAP Phase 7 | undecided | Phase 7 if built | a new library type: album and artist pages, an audio player that persists across pages, a TV audio screen; an album-rule question at library creation |

## 5. Search, discovery and Home

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| H1 | Admin-curated Home rows per role (F42) | "This collection is a row on the kids' Home" | [D149](../../decisions/working-notes.md) §5; web-client.md; tv-client.md | designed | — | a Home layout the server sends; a row editor per role |
| H2 | Rows and libraries in a person's own order | The person arranges rows and the sidebar, and hides rows | [D138](../../decisions/working-notes.md) §2 | designed (M5 may build part) | — | reorder and hide controls on Home and in the menu |
| H3 | Watchlist (F67) | "I intend to watch this", kept per person | [D140](../../decisions/working-notes.md) §3, [D150](../../decisions/working-notes.md) §3 | designed: stored, but no screens of its own | — | an add action on titles; perhaps one row |
| H4 | Favourites and the person's own rating (F65) | as named | [play-state.md](../plans/play-state.md) | designed (stored) | — | heart and stars on the title page; a favourites row or filter |
| H5 | Play statistics and history (F39) | My viewing last month, the household's for the owner, kept for ever; deleting one's own entries | [D81](../../decisions/working-notes.md), [D149](../../decisions/working-notes.md) §1, [D165](../../decisions/working-notes.md), [D173](../../decisions/working-notes.md) §4; [play-statistics.md](../plans/play-statistics.md) | deferred (plan UNSPECIFIED) | — | a statistics page and a history page per person; a household view for the owner |
| H6 | The previous layout stays available | A redesign never removes the way back for at least one release | [D108](../../decisions/working-notes.md), [D139](../../decisions/working-notes.md) | designed | — | a "use the previous layout" setting |
| H7 | Theming and a light palette | Tokens for colour, density and poster shape; a light set later | [D138](../../decisions/working-notes.md) §4, [D251](../../decisions/working-notes.md) §1 | idea (m5.md Q1) | — | an appearance setting |
| H8 | Deep links, widgets, Shortcuts (F62) | Shareable item links; OS integrations | [D88](../../decisions/working-notes.md); FEATURES F62 | designed (links) / deferred | — | a share action on titles |
| H9 | Instant mix and party mode (F64) | as named | FEATURES F64 | deferred | — | an action on titles |

## 6. People, accounts, sharing and permissions

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| U1 | Parental controls and restricted roles (F16) | A role's restriction vector (ratings per system, seven content axes, tags), templates that report what they hide, and anything unrated hidden until someone rates it | [D84](../../decisions/working-notes.md), [D130](../../decisions/working-notes.md), [D131](../../decisions/working-notes.md), [D132](../../decisions/working-notes.md), [D133](../../decisions/working-notes.md), [D134](../../decisions/working-notes.md), [D164](../../decisions/working-notes.md) §2; [parental-controls.md](../plans/parental-controls.md) | designed (cut-a "Out"; Cut A has admin-made roles that choose libraries, [D243](../../decisions/working-notes.md) §4) | — | a role editor; **browse as this role**; select what is wrong and see proposed rules with their cost; a preview that stresses what enters; a tag picker; an admin categorisation queue with bulk tagging; a launch-screen count ("28 items are waiting to be rated"), a number only; a per-person deny list; a broken rule shown as an empty library naming the field |
| U2 | A PIN that switches profile (F48) | On a trusted TV, a child's profile is open and a parent's needs a PIN; never a sign-in | [D150](../../decisions/working-notes.md); [auth.md](../plans/auth.md) | designed | TV | a "who's watching" profile picker; a PIN pad |
| U3 | Ceilings (F31) | Maximum bitrate and simultaneous streams per role; a refusal that names itself | [D149](../../decisions/working-notes.md), [D176](../../decisions/working-notes.md), [D180](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §4 | designed (cut-a "Out") | before any relay we run | fields in the role editor; the refusal in the player; "lower this stream's ceiling" in Now playing |
| U4 | TV sign-in by RFC 8628 (F30) | The TV shows a code and the phone approves, naming the device | [D93](../../decisions/working-notes.md), [D191](../../decisions/working-notes.md) | designed. A phone-made code typed on the TV is built ([D218](../../decisions/working-notes.md) §3, [D221](../../decisions/working-notes.md)); the TV-shows-code form is not | TV | a code screen on the TV; an approve page on the phone |
| U5 | Two factors and passkeys (F47) | as named | FEATURES F47 | deferred (the session model is kept open) | — | an account security page |
| U6 | OIDC and LDAP | Sign in with the household's SSO | [D20](../../decisions/working-notes.md), [D178](../../decisions/working-notes.md) §4 | deferred | — | sign-in buttons; an admin authentication page |
| U7 | Scoped API keys | Keys for the *arr tools, limited to chosen capabilities | [D154](../../decisions/working-notes.md) §3 | designed | — | an admin API keys page |
| U8 | Rebroadcast (F91) | A friend's server plays our library to his household, under his own rules, one hop only | [D174](../../decisions/working-notes.md), [D177](../../decisions/working-notes.md), [D187](../../decisions/working-notes.md) §4, [D243](../../decisions/working-notes.md) §5 | designed, deferred | "its own milestone" | a "may rebroadcast" tick on a role; terms accepted once per grant; how many people a grant serves; "Ben (via Mark)" in Now playing; on his core, add a remote library |
| U9 | The sharing page | How many people outside the household can play the library; whether a grantee sees that the library is filtered; whether a grantee may write their own rules | [D76](../../decisions/working-notes.md), [D187](../../decisions/working-notes.md) §4; [sharing-model.md](../plans/sharing-model.md) "Still open" | designed in part; open questions | — | a sharing page with the count; a "your library is filtered" banner (one answer for children and grantees) |
| U10 | Role templates; per-role capabilities added with features | Family, extended family, friends, guest; seeing other members, download leases, ceilings | [D131](../../decisions/working-notes.md), [D246](../../decisions/working-notes.md) §1 | deferred | — | a template picker; the role editor growing a row per capability |
| U11 | "Your access changed" | A connected client re-syncs without detail | [D168](../../decisions/working-notes.md) §5 | designed | with roles | a quiet toast and refresh |
| U12 | Access schedules | "No TV after 9 pm" | [D130](../../decisions/working-notes.md) (Status: open) | idea | — | a schedule in the role editor |

## 7. Remote access and networking

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| R1 | Our own relay role | The core dials out to a relay on a €5 VPS, so CGNAT needs no port-forward | [D71](../../decisions/working-notes.md), [D72](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §4; [remote-access.md](../plans/remote-access.md) | designed, deferred | ROADMAP Phase 6 | admin relay set-up and status; the client naming the failing hop |
| R2 | A failure names its hop; relay gap annotation | "No network" or "relay up, server not dialled in" or "NAS asleep"; what the relay saw while the core was away | [D152](../../decisions/working-notes.md); FEATURES F53 | designed | with the relay | client error screens; an admin status line |
| R3 | A connection service | Set-up from one QR code: address look-up, an HTTPS name and introductions, carrying no content | [D214](../../decisions/working-notes.md) | idea (recorded, not scheduled) | — | a QR set-up screen on clients; pairing on the core |
| R4 | A hosted tier | Remote access for people who will not rent a VPS | [D25](../../decisions/working-notes.md), [D72](../../decisions/working-notes.md); [overview.md](../overview.md) "The open problems" 1 | open (product decision) | — | a token or billing page; an allowance warning |
| R5 | Remote nodes; a second encoding node | A GPU desktop encodes while the NAS serves | [D193](../../decisions/working-notes.md) §1, §3, [D234](../../decisions/working-notes.md) §9; transcoding.md | planned before a node is remote | — | a nodes page on System listing each node and its encoders |

## 8. Admin and operations

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| A1 | Bug reports | A pseudonymised report, `safe` or `unsafe`, gathered across components | [D74](../../decisions/working-notes.md); [bug-reports.md](../plans/bug-reports.md) | designed | — | a "make a report" action on System or Help |
| A2 | Metrics | A `/metrics` endpoint, off by default | [D91](../../decisions/working-notes.md); [observability.md](../plans/observability.md) | designed | — | a setting |
| A3 | Load graphs | CPU and GPU load over time on System | [D225](../../decisions/working-notes.md) §4 | deferred | — | a graph on System |
| A4 | Stale components | The server says when its FFmpeg, libass or FreeType is stale | [D61](../../decisions/working-notes.md); content-safety.md | designed | — | a System line |
| A5 | Backup retention | How many backups are kept, shown and settable | [D77](../../decisions/working-notes.md); [backup-restore.md](../plans/backup-restore.md) "Still open" | open | — | a field on the backup section |
| A6 | API compatibility | An old client is told "update needed" in one legible message | [D85](../../decisions/working-notes.md); [api-compatibility.md](../plans/api-compatibility.md) | designed | before the first client ships | an "update needed" screen in every client |
| A7 | Release, first run and stores | A first-run legal notice; an admin-customisable login page; no indexing | [D187](../../decisions/working-notes.md) §6–§7, [D192](../../decisions/working-notes.md), [D249](../../decisions/working-notes.md) §5 | planned | public beta | a first-run screen; a login-page setting |
| A8 | Server time zone; update check | as named | [D200](../../decisions/working-notes.md) §1, [D246](../../decisions/working-notes.md) §7 | idea / question | — | settings |

## 9. Clients and devices

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| C1 | The remote TV | The first working path for the remote person's TV: its browser, our cast receiver, a prototype app, or the laptop | [D188](../../decisions/working-notes.md) §3, [D245](../../decisions/working-notes.md); cut-a M6 row | planned | M6 | a 10-foot playback mode; the cast receiver's screens (idle, loading, playing, error) |
| C2 | The TV client (F34) | An Android TV app; the framework chosen by building four screens twice | [D193](../../decisions/working-notes.md) §6; [tv-client.md](../plans/tv-client.md) | designed | ROADMAP Phase 3 | the whole 10-foot UI. **The remote's contract:** left and right always seek 10 s; track menus are visible buttons, one level deep. Captions in two presses; long-press "hide" with undo. Focus rules ([D189](../../decisions/working-notes.md)); a titled list view as well as posters; a server-chosen, user-ordered Home |
| C3 | Casting your own TV | Cast a series and its Next Up queue; any signed-in device can take control | [D88](../../decisions/working-notes.md), [D135](../../decisions/working-notes.md), [D136](../../decisions/working-notes.md), [D137](../../decisions/working-notes.md) | designed | — | a cast button in the player; a queue; "control this TV" |
| C4 | Other receivers (F49) | AirPlay, Fire TV | [D124](../../decisions/working-notes.md) | deferred (the role stays protocol-agnostic) | — | the same cast button, listing receivers of several kinds |
| C5 | Offline download (F22) | A film on a phone with its subtitles and fonts, progress merged on return, a smaller copy if wanted, kept if the server's copy goes | [D87](../../decisions/working-notes.md), [D165](../../decisions/working-notes.md) §5, [D167](../../decisions/working-notes.md), [D169](../../decisions/working-notes.md) | designed (cut-a "Out") | ROADMAP Phase 3+ | a download button with a size choice; a downloads screen with "orphaned" labels and storage; a devices list to forget a lost phone; the delete confirmation's "2 devices hold a copy"; honest wording that a lease does not stop playback |
| C6 | Native phone apps, iOS | as named | [D176](../../decisions/working-notes.md), [D250](../../decisions/working-notes.md) §7 | deferred | — | phone-native screens from the web's flows |
| C7 | Jellyfin adapter; OpenSubsonic | Third-party clients (Infuse, Findroid, Swiftfin; Symfonium for music) | [D18](../../decisions/working-notes.md), [D105](../../decisions/working-notes.md), [D106](../../decisions/working-notes.md); [jellyfin-api-shim.md](../plans/jellyfin-api-shim.md) | designed, deferred | Phases 5 and 7 | an admin toggle |
| C8 | Old clients stay installable | Versioned APKs to roll back a bad update | [D108](../../decisions/working-notes.md) | designed | TV | a downloads page |
| C9 | Android Auto, CarPlay (F63) | as named | FEATURES F63 | deferred | — | platform templates |

## 10. Accessibility and internationalisation

| # | Item | What the person gets | Recorded | Status | Milestone | UI surfaces |
| --- | --- | --- | --- | --- | --- | --- |
| I1 | The full caption control set (F28) | Font (eight), size 50–200%, text, background and window colour and opacity, five edge styles, within two steps of playback; on Cast too | [D90](../../decisions/working-notes.md), [D137](../../decisions/working-notes.md); [accessibility.md](../plans/accessibility.md) | designed | — | a caption style panel reachable from the player; a preview line |
| I2 | Screen readers and TalkBack | A screen reader walks the same focus model | accessibility.md; tv-client.md | designed (requirement) | TV | accessible names everywhere; the focus model |
| I3 | Accessibility as a release gate | Decided before a release anyone else installs | [D188](../../decisions/working-notes.md) §2 | open | public release | all screens |
| I4 | Translations and a language choice | The UI in the person's language, served by the server; RTL from the OS | [D45](../../decisions/working-notes.md), [D51](../../decisions/working-notes.md); [internationalisation.md](../plans/internationalisation.md) | designed | — | a language setting; layouts that survive long strings and mirroring |
| I5 | Metadata in the person's language | Titles, genres and posters per language; sort by original title | [D45](../../decisions/working-notes.md), [D47](../../decisions/working-notes.md) | designed (Korean collation is a known defect) | — | the language setting; a sort option |

## 11. Performance and infrastructure

Little of this reaches the screen. It is listed for completeness.

| # | Item | Recorded | Status | UI surfaces |
| --- | --- | --- | --- | --- |
| N1 | Settings kept in memory; an audit of database access patterns | *memory* `queued-db-performance-work.md` (owner, 2026-10-05) | queued | none |
| N2 | A push channel instead of polling | [D237](../../decisions/working-notes.md) §3 | idea | every live screen updates without a refresh |
| N3 | Move detection by inode; paged listing for huge libraries | [D240](../../decisions/working-notes.md) §3, §8 | deferred | none |
| N4 | arm64 kept open; riscv64 watched | [D205](../../decisions/working-notes.md) | kept open | none |
| N5 | Every dependency pinned to an audited commit | [D220](../../decisions/working-notes.md) | before the first public release | none |
| N6 | Our own FFmpeg and libass builds; served fallback fonts | [D53](../../decisions/working-notes.md), [D54](../../decisions/working-notes.md), [D193](../../decisions/working-notes.md) §4 | deferred | none |
| N7 | Publishing the development image from CI | *memory* `ci-dev-image-shelved.md` | shelved until the public beta | none |

**Declined, for completeness** (FEATURES.md rank 4; none needs room): photos, audiobooks and comics, Live TV and DVR, requests, outbound notifications ([D151](../../decisions/working-notes.md)), DLNA, 3D, Sonos, games and sports libraries, federation beyond rebroadcast, baked poster overlays, a content-source plugin tier, voice assistants, a desktop application, Roku, DRM, decrypting discs, built-in content sources, venue and kiosk modes, autoplaying previews ([D188](../../decisions/working-notes.md) §1), password recovery by email ([D194](../../decisions/working-notes.md)), several roles per user ([D243](../../decisions/working-notes.md) §4), a separate pre-play speed test ([D247](../../decisions/working-notes.md)).

## 12. What the M5 UI must leave room for

Distilled from the surfaces columns, roughly in order of how much rework each would save.

1. **A navigation model that grows.** The Manage group today has six destinations: identify,
   libraries, people, settings, jobs and system. Later features add about twelve more: roles
   and rules, sharing, home rows, the categorisation queue, collections, retention and trash,
   statistics, API keys, nodes, plugins and adapters, bug reports, and the file tree. Watching
   gains collections, music and downloads. A flat dropdown will not hold them. Manage wants
   an index page or a second level, and the phone's bottom bar wants a "More".
2. **A Home the server composes.** Rows come from the server (F42's obligation: the client
   fetches its layout and does not compile it in). They are reordered and hidden by the person,
   and each row is a generic collection-backed row. Home also needs a slot for a count banner
   ([D133](../../decisions/working-notes.md)).
3. **A player whose controls can grow by about a dozen.** Coming:
   - on the picture: skip intro, the next-episode card;
   - on the scrub bar: thumbnails and chapter and segment marks;
   - in the menus: speed, sleep timer, chapters, quality with "Auto" and its reason, "play the
     original", cast, picture-in-picture;
   - in the panels: verified badges and dual selection in the track panel, caption style
     within two steps, sound steps and night mode;
   - in Watch Together: per-person volumes, a roster, notices with undo, chat, and controls
     limited by the guest's rights.

   This argues for a small fixed bar plus panels (one for tracks, one for settings), not a row
   of icons. D268 §4's command seam is where guest rights plug in.
4. **A title page with one primary action and an overflow.** Besides play, start over and
   resume, it collects about a dozen actions:
   - watchlist, favourite, rating;
   - get this ready, download, play in another player, share, cast;
   - edit, identify, merge, delete;
   - track choice before play.

   It also gains sections for versions and editions, extras, people, scores and collections.
5. **Permissions that shape every screen.** An action the role lacks is absent, or shown with
   its reason (F37). There is a "browse as this role" mode. A pending, unrated item is absent
   from a restricted person's every surface, title included ([D134](../../decisions/working-notes.md)). So screens must
   render from what the server says this principal may see and do, never from a hard-coded
   role check.
6. **One focus model, designed for a remote.** [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) R7's roving-focus module under Grid and Reel, [D189](../../decisions/working-notes.md)'s focus rules, nothing reachable only by hover, long-press as a
   menu, captions within two presses. The TV milestone then reuses it instead of retrofitting
   it.
7. **Devices split by role.** A phone companion page (voice plus transport, no picture) separate
   from the player page, for M6 and later native apps. Voice controls must not live only inside
   the player.
8. **An in-app notice area.** Outbound notifications are declined ([D151](../../decisions/working-notes.md)), but these all need
   somewhere to appear: the categorisation count, "your access changed", an invitation
   to a session, quotas, a stale component, an allowance warning, and Watch Together's named
   events. One designed place beats each screen inventing a banner.
9. **Poster tiles with badge slots.** Watched and progress, downloaded or orphaned, score,
   unavailable, and later music's own tiles. Badges are a client render over an untouched
   image (F78), so the tile needs room for them from the start. A titled list view is a
   first-class second layout ([D138](../../decisions/working-notes.md) §7).
10. **Selection mode in grids.** Bulk edit (F60), bulk tagging for the categorisation queue,
    export, retention and adding to a collection all want multi-select and an action bar.
11. **Scoped settings with provenance.** Every effective value shows whether it came from the
    person, the library, the series or the device ([D188](../../decisions/working-notes.md) §1). Device-scoped preferences
    ([D103](../../decisions/working-notes.md)) need a devices list the person can prune.
12. **Tokens as the only theming surface.** A light palette ([D251](../../decisions/working-notes.md) §1) and density are
    token sets, and per-language layouts must survive long strings and right-to-left.

## 13. Possible gaps or conflicts

*This section is the coordinator's judgement.*

- **Notifications.** [D151](../../decisions/working-notes.md) withdrew outbound notifications. [D246](../../decisions/working-notes.md) §4 then wants "an in-app
  notification" to invite a member, and [D168](../../decisions/working-notes.md) §5, [D133](../../decisions/working-notes.md) and [D72](../../decisions/working-notes.md) each want a notice. None
  designs an in-app notice surface (see §12 item 8).
- **Text chat.** [D63](../../decisions/working-notes.md) and [D69](../../decisions/working-notes.md) call it nearly useless and decline it. [D188](../../decisions/working-notes.md) §1, [D246](../../decisions/working-notes.md) §5 and
  watch-together.md "Later" list it as wanted.
- **Get this ready.** m5.md hands it to M5; cut-a's M6 row and [D245](../../decisions/working-notes.md) put it in M6.
- **Device pairing.** cut-a §1 lists it "Out" ([D191](../../decisions/working-notes.md)), but a phone-made code that signs in a
  TV is built ([D218](../../decisions/working-notes.md) §3, [D221](../../decisions/working-notes.md)). Only RFC 8628's TV-shows-the-code form ([D93](../../decisions/working-notes.md)) remains.
- **Watchlist.** [D140](../../decisions/working-notes.md) §3 adds it; [D150](../../decisions/working-notes.md) §3 and FEATURES F67 keep it as storage with no
  screens. Whether it has a Home row is undecided.
- **Missing against Plex, Jellyfin and Emby:**
  - Hand-made playlists, as such, are named only as the rules engine's *smart* playlist;
    explicit Collections may serve, but no plan says so.
  - A household "who's watching" profile picker on a shared device exists only as F48's PIN
    rule.
  - No per-device quality cap or data-saver for a phone on mobile data. [D257](../../decisions/working-notes.md) remembers link
    speed, but nothing lets a person cap it.
  - "Mark as watched / unwatched" by hand was not found in the plans swept (it may be built).
  - A log viewer for the admin was not found.
  - A trailer or pre-roll "cinema mode" was not found.

## 14. Not checked

- Whether each item marked *designed* is already partly built: the code was not read, except to
  confirm the device-code sign-in.
- The plans read for headings and selected sections only: [scenarios.md](../plans/scenarios.md),
  [schema.md](../plans/schema.md), [library-model.md](../plans/library-model.md),
  [scanner.md](../plans/scanner.md), [jobs-rebuild.md](../plans/jobs-rebuild.md),
  [chunks.md](../plans/chunks.md), [stream-service.md](../plans/stream-service.md),
  [identification.md](../plans/identification.md), [rules-engine.md](../plans/rules-engine.md),
  [transcoding.md](../plans/transcoding.md).
- Not opened at all: [device-lab.md](../plans/device-lab.md), [threat-model.md](../plans/threat-model.md),
  [testing-strategy.md](../plans/testing-strategy.md), [performance-targets.md](../plans/performance-targets.md),
  [migrations.md](../plans/migrations.md), [settings.md](../plans/settings.md),
  [artwork.md](../plans/artwork.md), [playback-survey.md](../plans/playback-survey.md), and
  [m4.md](../plans/m4.md).
- Research documents other than those the fifth sweep read (the audits, the UI and voice
  research, feature-sweep, tvos, app-stores, commercial-streaming).
- The decision log was read by four sweeps, each over its range. Their findings were taken as
  reported, and a sample was checked at the source: [D188](../../decisions/working-notes.md), [D267](../../decisions/working-notes.md), [D270](../../decisions/working-notes.md), and the D218 code
  sign-in.
- The memory's handoff notes beyond a grep for "later" and "wants".
