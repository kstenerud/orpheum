> **ABOUTME:** How media apps show that a video was watched, partly watched or not yet watched,
> and how a person marks it by hand; what Orpheum has today (positions and a finished threshold, no
> watched state, no mark); and a recommended design for a checkmark in the poster's tray, with the
> points that need the owner set apart from the defaults an agent can take.

# Watched Marks: What the Field Does, and a Design for Orpheum

- **Date:** 2026-10-27. Asked by the owner: *"do we have a way to show that a video has been
  watched? If not, I want that folded in to this batch. Do research first to see how our
  competitors do it. I'm imagining a checkmark (in the icon tray) and then maybe showing the poster
  a little differently (slightly faded maybe?). If it requires my input then I'm fine with bumping
  it to the next batch."*
- **Method:** the code read at `main` an earlier commit; competitors from their own documentation and
  source where it is open (jellyfin-web's `indicators.js`, Jellyfin's `UserDataManager.cs`), and
  from their forums and trackers for complaints. Fetched pages are cached in
  `/yoloai/cache/watched-marks/`.
- **Proposes, decides nothing.** The design below becomes a decision only when the lead records it
  with the owner's answers.

## 1. Summary and recommendation

**No, Orpheum has no way to show that a video was watched.** It keeps where each person is in each
file, and counts a file *finished* past `finished_after_percent` (90) of its length, but only to
keep it out of Continue Watching, to count a series' finished episodes for its progress bar, and to
find Next Up. Nothing stores "watched", nothing shows it, and nothing marks it by hand. Because
"finished" is read from the *current* position, it is also lost the moment a person starts the film
again: restart a finished film, stop at 10 minutes, and it is no longer finished anywhere (§2.2).

**Every library app shows it, and they agree on most of it** (§3): a small mark in the poster's
top-right corner, a progress bar along the bottom for something partly watched, a threshold of
90% (Plex, Jellyfin, Kodi), and *Mark as watched / unwatched* on the item, the season and the
series. **None of the library apps fades a watched poster;** fading is asked for in feed apps
(YouTube clients) where watched means done with. The loudest complaint in the field is about the
mark itself: when Plex swapped its *unwatched* corner for a *watched* checkmark, a long thread
objected that a well-used library becomes *"a sea of green check marks"*, and Plex answered with a
per-account setting to turn the marks off.

### The recommendation

1. **A checkmark in the poster's tray**, the last cue (below the warning, the uncertain match and
   the still-reading cues, as the least urgent), drawn like them on the tray's dark patch, in the
   text colour rather than a warning or status colour, since it is no problem and no activity. Its
   screen-reader name is "Watched". On films, series, episodes (season page rows) and other videos;
   not on extras ([D162](../../decisions/working-notes.md): an extra is a different work).
2. **No fading, at least at first.** The poster card already fades a poster to half opacity to say
   *cannot play now* (`poster-card.css`, `.poster-card__link--dimmed`); a second fade would make two
   states look alike, and [D189](../../decisions/working-notes.md) §6 already says *"a selected
   (checked) item gets a mark, not a darker tile"*. The check carries the meaning; if the owner
   wants a fade too, it can be added after he sees the check on staging (§1.1, question 1).
3. **Watched is a fact that stays.** A title is watched by a person when they have ever finished
   one of its files (played past the threshold, or to its end) or marked it watched, and have not
   marked it unwatched since. Starting it again does not unwatch it: a rewatch shows the check *and*
   the progress bar, and Continue Watching offers it as usual. This is Kodi's play-count model, and
   it avoids Jellyfin's reported fault (§3.2) where re-watching one episode of a completed series
   turns it back to unwatched.
4. **The threshold is the existing `finished_after_percent` (90)**, so "watched" and "left
   Continue Watching" never disagree. A file of unknown length is watched only by reaching its end
   (the player's `ended`, already logged as a `completed` event) or by hand, never by assumption
   (play-state.md's rule against Jellyfin's *"assume it was fully played"*). Plays logged as
   unattended never mark anything watched (play-state.md). When credits detection is built, the
   default should become Plex's: the earlier of the threshold and the first credits marker.
5. **Partly watched keeps today's progress bar** along the poster's bottom, which every list
   already draws (`PosterCard`'s `progress`). Episode rows on the season page gain the same bar and
   check; today they show neither.
6. **A series or season gets the check when every episode it holds is watched**, counted as the
   series' progress bar already counts them (the person's ordering, the episodes at places they
   reach). Until then the series keeps its progress bar (finished of total, the owner's 2026-10-25
   choice). **No unwatched-count badge:** the bar already answers "how far am I", counts were the
   most-disliked part of Plex's change, and a count is what Plex's Fire TV screen reader reads out
   instead of titles (*"1… 241… 36"*, [D188](../../decisions/working-notes.md)). A new episode
   arriving takes the series' check away and its bar returns, as it should.
7. **Marking by hand is on the title's page, never on the poster.** A film's, episode's or other
   video's page gets one toggle, *Mark watched* / *Mark unwatched* (a button with `aria-pressed`),
   and its status notes say "Watched" in words beside the check. A season gets *Mark season
   watched / unwatched*, and the series page *Mark series watched / unwatched*. No confirmation
   dialog: like Continue Watching's Hide ([D188](../../decisions/working-notes.md)), the change
   shows at once with an Undo. The tray stays icons only, never controls (the owner, 2026-10-25,
   recorded in `status-cues.tsx`).
8. **A mark is a fresh start for the resume point.** Marking watched takes the title out of
   Continue Watching and its next play starts at the beginning; marking unwatched also starts it at
   the beginning. Nothing is deleted: positions, spans and events stay in the log for statistics
   ([play-statistics.md](../plans/play-statistics.md)); the mark simply sits later than them. This
   is the closed beta's acceptance line, *"a film marked watched shows it in its tray and moves out
   of Continue Watching"* ([closed-beta.md](../plans/closed-beta.md) B2).
9. **Per person**, like positions: each user's own marks ([D125](../../decisions/working-notes.md));
   a signed-in Watch Together participant's viewing marks their own ([D191](../../decisions/working-notes.md));
   a guest marks nothing. When profiles arrive (B2), the mark is the profile's.
10. **Watched is about the work** ([D157](../../decisions/working-notes.md),
    [D173](../../decisions/working-notes.md), [D306](../../decisions/working-notes.md) §8):
    finishing any file of a title marks the title, a replaced or re-acquired copy keeps the check,
    and what [D306](../../decisions/working-notes.md) §8 keeps of a purged file
    (`purged_watching.completed_count`) counts too, which is the first place that kept history is
    ever shown.
11. **Next Up and the series' next episode read the same watched state**, so marking an episode
    watched by hand moves Next Up past it; today they read finished positions only.

**How it might be built** (for the lead, not a decision): one new per-person table of hand marks,
`(user, title, is_watched, marked_at)`, added as the next migration step; "watched by playing"
derived from what is already logged — `completed` events, spans whose end passes the threshold,
the current position past it, and `purged_watching` — and the latest of a hand mark and the latest
completion wins. That needs no backfill and no second copy of the log.
[D185](../../decisions/working-notes.md) §2 measured exactly this rollup (File → Version → Title for
a 50-poster page) at 0.84 ms live, with the warning that the same rollup written as an `EXISTS` the
planner turned into a scan took 7.2 ms: query shape is the risk. The API adds `isWatched` to list
entries, episode bodies and seasons, and `PUT`/`DELETE` routes for the marks of a title, a season
and a series. The tray gets one row in `statusCues`, and `CueSubject` one field. A size of S–M,
as the inventory has it (C-8, S), with the episode rows and season marks the larger half.

### 1.1 Decisions the owner must make

None of these blocks building the check tonight; each has a default (§1.2) that can be changed
after he sees it on staging.

1. **Fade the watched poster, or not?** His own idea. Against it: half opacity already means
   *cannot play now*; D189 §6 asks for a mark, not a darker tile; no library app does it; and Plex's
   users called a well-watched library ugly with marks alone. If yes: how much (a quarter, not half),
   whether it is a desaturation rather than a fade so it differs from *cannot play*, and that it
   lifts on focus (D189 §1: focus never leaves the focused tile dimmed).
2. **Toggle from the poster too?** closed-beta.md B2 says *"one more icon in the poster's tray,
   which a person can toggle"*, but on 2026-10-25 he ruled that tray icons are never controls (they
   cluttered the posters and took a controller's focus). Default: toggle on the page only. If he
   wants it from lists too, a long-press or a context menu is the field's way (Plex, Infuse,
   Prime Video), and that waits for the widget set (D369).
3. **Watched or unwatched as the marked state?** Plex moved from marking *unwatched* (an orange
   corner) to marking *watched* (a check), and the biggest thread of complaints in this research is
   about that move; Infuse still marks unwatched. He asked for a checkmark, so the default is a
   check. Whether a person may turn the marks off (Plex's *Watched Indicators* setting: movies & TV,
   movies, TV, none; Infuse's *Watched Indicators* switch) can wait for the complaint.
4. **Unwatched counts on series and seasons?** Default none (point 6 above).

### 1.2 Defaults an agent can take without him

Every numbered point of §1 not listed in §1.1: the threshold (the existing 90% setting), unknown
length, unattended plays, rewatch keeps watched, the series rule and the season page's rows, marks
on pages with undo and no dialog, a mark resetting the resume point, per person, title-level
rollup with D306's carried history, Next Up reading it, the check's place and colour in the tray,
and no fading. Two smaller choices inside them, for the record: a series' check counts episodes the
way its progress bar does (so specials count only where the person's ordering lists them), and a
season or series mark covers only the episodes present now, not ones that arrive later.

**So it can be built tonight without the owner**, with the fade left as his call on staging. It
needs a migration step (the window is open for appended steps; the new golden has standing
approval) and a decision recorded by the lead under the next free D-number.

## 2. What exists today

### 2.1 Play state: positions, events, spans

- **Tables** (baseline `0048`): `play_position` (one row per user and file, the guarded position),
  `play_event` (`started`, `paused`, `seeked`, `completed`, `abandoned`), `play_span` (what was
  seen, with `is_unattended`), `progress_hide` (Continue Watching's Hide,
  [D188](../../decisions/working-notes.md)), `purged_watching` (completions and time kept when a
  purged file's history is folded onto its version, [D306](../../decisions/working-notes.md) §8)
  and `watchlist`. There is **no watched or played column or table** and no mark.
- **Thresholds** are settings: `in_progress_after_percent` (5) and `finished_after_percent` (90)
  (`internal/core/settings/registry.go`), read by `ResumeThresholds` in
  `internal/core/library/home.go`. `IsFinished` is *the current position past 90% of a known
  length*; a file of unknown length is never finished by its position.
- **`completed` events** are sent by the web player only on the video's `ended`
  (`web/apps/web/src/play-state.ts`), so only when a file plays to its very end. They are read
  by one query, `FoldWatchingOfPurgedFiles`; and `purged_watching` is written and never read.
- **The design** in [play-state.md](../plans/play-state.md) already says completion attaches to
  the file and rolls up freely to version, edition and title, that a replaced file keeps "watched"
  (*"Watched survives: it is about the work"*), that hiding is not watching, and that unattended
  plays never mark anything watched. The plan expected a watched state; the build stopped at
  positions.

### 2.2 What reads "finished", and how it breaks

- **Continue Watching** (`ListRecentPlayPositions`, `home.go`) shows a work whose latest position
  is between the two thresholds and not hidden since.
- **Lists' progress** (`internal/core/library/progress.go`): a film's `watchedPercent` when it is
  partway; a series' `episodeCount` and `finishedEpisodeCount`, from which the web draws the
  series' bar (`browse.tsx`, `watchedPercentOf`, the owner's 2026-10-25 choice).
- **Next Up** (`internal/core/library/next_up.go`) and [D324](../../decisions/working-notes.md)'s
  interest order count a member finished by the same position test.
- **The flaw:** all three read the *current* position. Watch a film to the end (position past
  90%), start it again a week later and stop at 10 minutes: the position is now 10%, so the film is
  in Continue Watching (fine) but no longer finished anywhere — a series' bar drops, and Next Up
  steps back to that episode. A watched state that only playing past the threshold or a hand mark
  can set, and only a hand mark can clear, fixes this.

### 2.3 The web client

- **`PosterCard`** (`poster-card.tsx`) is every list's poster: the art, a progress bar along its
  bottom, the status tray over its top-right corner, and an `isDimmed` flag that sets the poster to
  half opacity for *cannot play now*.
- **The status tray** (`status-cues.tsx`) is a static, ordered list of cues: cannot play (the
  warning triangle, [D278](../../decisions/working-notes.md)), uncertain match
  ([D301](../../decisions/working-notes.md) §4), still reading ([D304](../../decisions/working-notes.md) §4).
  Each cue is an icon on a dark patch with a screen-reader name, never a control, and has a note
  for the title's page. A new status is one new row, in its place.
- **The title page** shows no progress or watched state; **the season page's episode rows**
  (`series.tsx`, `EpisodeBody`) carry no play state at all; **seasons** carry only their episode
  count.
- **No checkmark icon** exists in the app yet.

### 2.4 Plans and decisions that name it

- [closed-beta.md](../plans/closed-beta.md) B2: *"Watched marks, and marking watched — the owner's
  note — S — One more icon in the poster's tray, which a person can toggle"*; acceptance: *"a film
  marked watched shows it in its tray and moves out of Continue Watching"*.
- [D313](../../decisions/working-notes.md) §3: B2 includes *"watched marks, shown in the poster's
  tray"*.
- [inventory-2026-10-27.md](../plans/inventory-2026-10-27.md) C-8: *"A watched mark in the
  poster's tray, toggled by the person; a film marked watched leaves Continue Watching"*, decided,
  size S, ninth of the most valuable unbuilt features.
- [D185](../../decisions/working-notes.md) §2: the watched-badge rollup measured, no cache needed.
- [D306](../../decisions/working-notes.md) §8: watching follows the title and its edition; what
  carries is whether it was watched and how often.
- [D188](../../decisions/working-notes.md): hiding is not watching; no state shown only by colour;
  a card's accessible name is its title, never a badge's count.
- [D189](../../decisions/working-notes.md): focus never darkens; a checked item gets a mark, not a
  darker tile.
- [D230](../../decisions/working-notes.md): for carrying a language choice, a *started* episode
  counts as watched. That is a different, narrower sense and should keep its own wording.
- feature-standing-2026-10-18.md lists *"Watched badges, mark watched: No (positions kept)"*
  against Yes for Plex, Jellyfin, Emby and Infuse.

## 3. What the field does

### 3.1 Plex

- **Marks** ([Mark as Watched or Unwatched](https://support.plex.tv/articles/201018487-mark-as-watched-or-unwatched/),
  last modified 2024-12-05): *"A small check mark will be displayed in the top right corner of the
  poster for watched items when viewing a library."* Unwatched films and episodes have no mark;
  *"the lack of a progress bar or a watched label indicates that the item has not been watched."*
  A show or season poster shows *"the number of unwatched episodes."* Partly watched shows *"a
  visual progress bar at the bottom"*, and the Play button becomes *Resume*. On a detail page a
  "watched" label sits beneath the poster, on TV clients below the title.
- **Turning them off:** *Watched Indicators for Personal Media*, account-level, with *"Movies & TV
  Shows (default), Movies, TV Shows, None"*; detail pages keep the check on watched episodes
  regardless.
- **By hand:** the item, *"an entire season"* and *"an entire television series"*, from the
  details page's action bar or the three-dot menu; a series or season asks for confirmation;
  multi-select in the web library has a *Toggle watched* button.
- **Threshold** ([Credits Detection](https://support.plex.tv/articles/credits-detection/)): *"the
  default and 'classic' behavior is a 90% threshold"*, adjustable; *Video play completion
  behavior* can use the first or final credits marker, and its default is *"earliest between
  threshold percent and first credits marker"*.
- **Complaints.** Around 2022–2023 Plex replaced its orange *unwatched* corner triangle with a
  *watched* checkmark. The thread
  [Watched Checkmark VS Unwatched Orange Corner](https://forums.plex.tv/t/watched-checkmark-vs-unwatched-orange-corner/854364)
  runs past 17 pages: *"Why would I want to see that on every single poster in my collection? An
  unwatched badge at least lets me know at a glance what content is new to me"*; *"Having the UI
  get uglier and more cumbersome as you use it properly is such a backwards and anti-user way to go
  about it"*; *"My 'reward' for watching a movie was a clean poster."* A second request
  ([Undo change to posters showing unwatched count](https://forums.plex.tv/t/undo-change-to-posters-showing-unwatched-count-return-watched-indicators-to-orange-color/847644),
  2023-07-18): *"My entire library is now a sea of green check marks. I hate it"*; *"It is heaps
  easier to find unwatched content when it is what is marked."* The indicators setting above is
  Plex's answer.

### 3.2 Jellyfin

- **Marks** (jellyfin-web `src/components/indicators/indicators.js`, read from source): a series or
  season with unplayed episodes shows a round **count** badge (`countIndicator`, capped at "99+");
  otherwise an item played (or at 100%) shows a round **check** (`playedIndicator`), both top-right.
  A progress bar shows while `PlayedPercentage` is under 100. No dimming.
- **Threshold** (`Emby.Server.Implementations/Library/UserDataManager.cs`, `UpdatePlayState`):
  under `MinResumePct` (5) the position is dropped; past `MaxResumePct` (90), or within a second of
  the end, the item is **Played** and its position cleared; an item shorter than
  `MinResumeDurationSeconds` (300) is marked played at any position; an item of unknown runtime is
  marked played (*"we'll just have to assume it was fully played"*). play-state.md already rejects
  the last two.
- **By hand:** *Mark played / unplayed* on items, seasons and series.
- **Complaints.** [jellyfin-web#143](https://github.com/jellyfin/jellyfin-web/issues/143)
  (2019-02-18): the blue check on every watched series clutters the TV Shows page; the answer was
  custom CSS (`.playedIndicator {display: none;}`), the forum's standing advice
  ([How to disable "watched" checkmarks on posters?](https://forum.jellyfin.org/t-how-to-disable-watched-checkmarks-on-posters),
  [How to remove episode count from top right of show](https://forum.jellyfin.org/t-how-to-remove-episode-count-from-top-right-of-show)).
  [Discussion #16922](https://github.com/orgs/jellyfin/discussions/16922) (2026-05-24): a series
  marked played turns back to unplayed when one episode is watched again, *"when you later decide
  to randomly watch one episode of this show, then if you do the episode automatically switches
  back to 'Unplayed'"*. And *"Hiding is not watching"*: removing from Continue Watching without
  marking played was its second most-wanted request (1,722 votes; [ui-competitors.md](ui-competitors.md)).

### 3.3 Emby

The web client draws a green check on watched posters and a count on collections, and its users,
like Jellyfin's, hide them with custom CSS, sometimes for films only
([Emby Community](https://emby.media/community/topic/37863-question-about-my-custom-css/)).
Same thresholds as Jellyfin, which forked from it.

### 3.4 Kodi

- **Threshold** ([HOW-TO: Modify automatic watch and resume points](https://kodi.wiki/view/HOW-TO:Modify_automatic_watch_and_resume_points)):
  `playcountminimumpercent` 90, `ignoresecondsatstart` 180, `ignorepercentatend` 8. Setting it to
  101 never marks anything watched automatically.
- **Watched is a play count**, not a flag: watching again adds to it and never clears it, so a
  rewatch cannot unwatch anything.
- **Marks:** the default skin (Estuary) draws a tick on watched items; a series' *watched / total*
  episodes appear only in list views, *"there's no space for it"* in poster and icon views
  ([forum](https://forum.kodi.tv/showthread.php?tid=225647)); users ask how to hide the tick
  ([forum](https://forum.kodi.tv/showthread.php?tid=129237)). The context menu marks an item, a
  season or a whole series watched or unwatched.

### 3.5 Infuse

- **Marks:** an orange corner on **unwatched** posters, an orange line along the bottom while
  partly watched; *Settings > General > Watched Indicators* switches them off
  ([Firecore support, 2023-11-19](https://community.firecore.com/t/hideout-the-unwatched-indicator/45976)).
  Collections whose films are all watched still showing the unwatched corner is a recurring
  report ([Firecore](https://community.firecore.com/t/movie-collections-are-displayed-as-unwatched-even-though-all-the-movies-in-them-are-watched/18357)).
- **By hand:** *Mark as…* on the item; from 7.4.3, a whole season or series by long-pressing its
  poster ([Firecore](https://community.firecore.com/t/improve-marking-entire-series-season-as-watched-unwatched/37958?page=3)).
- **Complaints:** marks that revert after a sync, *"marked indicators change back randomly to an
  earlier state"* ([Firecore, 2016](https://community.firecore.com/t/infuse-5-watched-indicators-broken/13812));
  Trakt-synced marks overwritten by a partial position
  ([Firecore](https://community.firecore.com/t/mark-as-watched-stopped-working-when-using-trakt/57864)).
  Both are the "position overrules the mark" fault that §1 point 8 avoids by ordering by time.

### 3.6 The streaming services

- **Netflix:** no watched mark and no *Mark as watched*; episode thumbnails carry a red progress
  bar, full when finished
  ([MakeUseOf](https://www.makeuseof.com/netflix-how-to-reset-watch-progress/)); *Remove from row*
  takes a title out of Continue Watching without touching history
  ([How-To Geek](https://www.howtogeek.com/netflix-remove-from-continue-watching/)).
- **Disney+:** a progress bar under episodes, no way to mark watched; third-party extensions fill
  the gap ([Disney Plus Watched Marker](https://chromewebstore.google.com/detail/disney-plus-watched-marke/eabelgojcdnpcjogoibpeohbbkcdgkca),
  [Disney+ Mark Watched](https://greasyfork.org/en/scripts/515467-disney-mark-watched)), and its
  App Store reviews complain of *"No automatic episode tracker"* (play-state.md).
- **Prime Video:** press and hold the play button of an episode or season for *Mark watched /
  Mark unwatched* ([Amazon forum](https://www.amazonforum.com/s/question/0D56Q0000CLlUcZSQV/how-to-mark-watched-episodes));
  *Hide this video* for Continue Watching
  ([Guiding Tech](https://www.guidingtech.com/clear-continue-watching-amazon-prime/)).
- **Apple TV app:** *"Mark as Watched or Mark as Unwatched: Re-categorises the item"*, from a
  thumbnail's More menu ([Apple Support](https://support.apple.com/en-nz/guide/tvapp-mac/atvbe9ddc3f/mac));
  a blue dot marks unwatched episodes in its library
  ([Apple Community](https://discussions.apple.com/thread/1961321)).
- **YouTube:** a red bar along the thumbnail's bottom, full width when watched to the end; no
  mark. Its third-party clients are where **dimming** is asked for: NewPipe
  ([#7653](https://github.com/TeamNewPipe/NewPipe/issues/7653), 2022: the red bar is *"hard to
  distinguish"* in a busy feed) and Grayjay
  ([#300](https://github.com/futo-org/grayjay-android/issues/300)). In a feed, watched means done
  with; in a home library, films are rewatched, which is why the library apps mark rather than fade.

### 3.7 A spec from outside

John Siracusa's [Unsolicited Streaming App Spec](https://hypercritical.co/2022/02/15/streaming-apps)
(2022): *"Keep track of what the user has done, and when. Which things has the user watched? Were
they watched entirely or partially?"* and *"When viewing a list of episodes, put something on the
screen to indicate which ones have been viewed and which ones haven't. Consider showing a user's
progress within an episode as well."* Orpheum's season page does neither today.

### 3.8 Across the field

| | Watched | Partly | Unwatched | Series / season | By hand | Fades |
| --- | --- | --- | --- | --- | --- | --- |
| Plex | check, top-right | bar | nothing | unwatched count | item, season, series; confirm | no |
| Jellyfin | check, top-right | bar | nothing | unwatched count, else check | item, season, series | no |
| Emby | check | bar | nothing | count | item, season, series | no |
| Kodi | tick | bar | nothing | watched/total in lists only | item, season, series | no |
| Infuse | nothing | orange line | orange corner | corner until all watched | item; season, series by long-press | no |
| Netflix, Disney+ | full bar | bar | nothing | — | no | no |
| Prime Video | — | bar | — | — | episode, season | no |
| Apple TV app | — | bar | blue dot | — | item | no |
| YouTube | full bar | bar | nothing | — | no | no (asked for in clients) |

**Threshold:** 90% at Plex, Jellyfin, Emby and Kodi; Plex alone uses credits, with the earlier of
the two by default. **Rewatching:** Kodi counts plays and never unwatches; Jellyfin's users report
a rewatch turning a completed series back to unplayed.

## 4. The television and accessibility

- **At ten feet** the tray's cues are already sized to read over art on a dark patch; the check
  must be the same size as the others and stay out of focus order (cues are not controls). The
  check is a shape anyone can read from a sofa; a fade is what disappears first on a dim set or a
  washed-out projector, another reason it can only support the check, not replace it.
- **Not colour or fading alone** ([WCAG 2.2 SC 1.4.1](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html),
  [D188](../../decisions/working-notes.md)): the check is a shape, named "Watched" for a screen
  reader by `PosterCueIcon`'s hidden label, and said in words on the title's page. A non-text icon
  needs 3:1 against what is behind it ([SC 1.4.11](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html)),
  which the tray's dark patch gives over any art.
- **The card's name stays its title** (D188): the cue's label follows it, never replaces it, and
  no count is read out.
- **Focus** (D189): if a fade is ever added, the focused poster shows at full strength.
