# A tap on a full-screen player, and Pause's effect on its overlay (2026-11-04)

> The owner, playing Nosferatu on staging: a tap in full screen brings up the overlay, which is fine,
> but a tap on Pause then leaves the overlay up. Asked: what do competitors do when someone taps
> the screen in full screen? Desk research by a subagent (about 22 searches); the repo fact checked
> by the lead.

## 1. What there is now

The overlay stays up while the film is paused, by design: it never hides by itself while paused or
while a menu is open (D407 §6; `web/apps/web/src/player-overlay-state.ts`). While playing, it hides
3 s after the last press (`fullShownForMs`).

## 2. Competitors

Primary sources only for Media3 (its source), Apple (WWDC22 10147, the Energy Guide), Infuse (its
help) and Emby (staff on its forum); the rest from forums and the tech press. "?" is not confirmed.

| Product | Tap, controls hidden | Double tap | Tap, controls showing | Hides by itself | Desktop click |
| --- | --- | --- | --- | --- | --- |
| Netflix (phone) | shows controls | ±10 s by side | hides them | playing ?; paused shows a title card after a while | ? |
| YouTube | shows controls | ±10 s by side, stacking, 5–60 s set | ? | playing: a few seconds; **paused: stays** (web) | toggles play (common, unsourced) |
| Disney+ | ? | ±10 s by side | ? | ? | ? |
| Prime Video | ? | ? | ? | ? | ? |
| Apple AVPlayerViewController (iOS 16+) | centre tap plays or pauses, even hidden | ? | ? | hides while playing (about 3 s, unverified); paused ? | n/a |
| Android Media3 PlayerView | shows controls | none built in | hides them (`controllerHideOnTouch`) | playing 5 s; **paused: stays until a tap** (`controllerAutoShow`) | n/a |
| Plex (mobile) | ? | ±10 s by side (broken in the 2025 Android app) | ? | ? | ? |
| Jellyfin | ? | ±10 s in the mobile apps | ? | web OSD hides, timing ? | ? |
| Infuse (iOS) | ? | ±10 s by side, stacking; two-finger tap plays or pauses | ? | ? | n/a |
| VLC mobile | shows controls | ±10 s (a setting) | ? | ? | n/a |
| Emby (mobile) | a tap dismisses the OSD | ? | hides it | about 3 s playing; hiding on pause asked for by some, disliked by others | ? |

**Not found anywhere:** a player that hides its overlay when Pause is pressed.

## 3. What is near-universal and what is split

- **Near-universal (touch):** a tap on the picture shows the controls; a double tap on the left or
  right seeks 10 s; the controls hide a few seconds after the last touch while playing.
- **Split:** whether a tap with the controls up hides them (Media3, Emby, Netflix) or pauses (Apple's
  centre tap); whether a desktop click toggles play or the controls (Video.js: touch toggles the
  controls, a click toggles play).
- **Paused:** where found, the controls stay up (YouTube web, Media3).

## 4. Recommendation

1. A tap on the picture with the controls hidden shows them, and does not pause.
2. A tap on the picture (not on a control) with the controls up hides them, paused or playing: the
   way to clear the overlay off a paused frame.
3. Pause keeps the overlay up, as now (D407 §6), and nothing hides it by itself while paused.
4. While playing, the overlay hides 3–5 s after the last touch (3 s now).
5. A double tap on the left or right third seeks 10 s back or forward, taps stacking.

## Sources

developer.apple.com WWDC22 10147 and the Energy Guide's "Hide controls in full-screen video";
androidx/media `PlayerView.java` (`performClick`, `toggleControllerVisibility`); support.firecore.com
215090987 and community.firecore.com 47525; emby.media community 53005, 141379, 52646;
forums.plex.tv 917662, 916383, 672725; github.com/orgs/jellyfin/discussions/17704;
forum.jellyfin.org tid 10999; wiki.videolan.org Mobile_Design_Player; MacRumors 2018-05-31, TechRadar
and 9to5Mac (Netflix double tap); HowToGeek and PhoneArena (YouTube); support.mozilla.org 1149257;
videojs.org "hiding and showing video player controls"; docs.nomercy.tv touch zones.
