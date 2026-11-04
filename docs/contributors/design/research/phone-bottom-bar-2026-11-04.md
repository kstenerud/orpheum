# A phone's bottom bar (2026-11-04)

> The owner's question on the UI audit's Q1 and Q2 (ui-audit-2026-11-04.md, "The owner's answers"):
> labelled places over icons, but a phone's screen is small and a place that opens a submenu costs
> taps. How many competitors use a bottom bar, how do their users feel about it, and what would it
> look like? Desk research by a subagent; nothing was checked on a device. Unverified items are
> marked.

## 1. Who uses one

| Service | Phone app's navigation | Confidence |
| --- | --- | --- |
| Netflix | Bottom bar since 2018 (Home, Search, Downloads, More; later New & Hot); the April 2026 iPhone redesign adds My Netflix | press; 2026 set not official |
| YouTube | Bottom bar: Home, Shorts, Create, You | press |
| Plex | Bottom bar replaced the hamburger (2024–25): about five labelled tabs; Library needs a dropdown to pick a library | press, Plex support; final set unconfirmed |
| Paramount+ | Five tabs: Home, Search, Clips, Live TV, Profile | one review, 2026 |
| Disney+ | A bottom bar of four icons (2020 critique) | old, labels unverified |
| Infuse | A tab bar on iPhone | forum; labels unverified |
| Jellyfin, Emby | Official clients not verified; third-party Jellyfin clients use a tab bar | unverified |
| Max, Prime Video, Apple TV, Hulu, Peacock | Not verified on phones | — |

Every phone app verified uses a bottom bar of four or five items. TV is where competitors moved to
a top bar or a side rail. Their mobile websites were not verified.

## 2. What guidance and users say

- Android: a navigation bar is for three to five destinations of equal weight, labelled. Apple's
  HIG: few tabs, a More tab beyond (not read this session). Adobe Spectrum and SAP Fiori: labels,
  icons alone only for universal ones.
- Hidden navigation (a hamburger, and so a More tab) is found about 20% less often (NN/g's study of
  179 people, through secondary summaries; the penalty is smaller on phones). Spotify reported its
  iOS tab bar raised menu-item clicks by 30%.
- Plex's redesign drew mixed reviews: the bar's room, layers of navigation, the Libraries tab's
  dropdown awkward one-handed, requests to customise the bar. The worst backlash was on TV, where
  actions went from two presses to about six, and Plex brought the left menu back: the owner's
  concern about submenus, met in practice.
- Netflix's move from a hamburger to the bar was reported as a simplification; no backlash found.
- Mobile web: iOS 26 Safari reportedly does not draw `position: fixed` content below its floating
  bottom controls (Apple Developer Forums 800798; test on an iPhone). The bar takes
  `env(safe-area-inset-bottom)` with `viewport-fit=cover`. Hiding it on scroll is optional; it must
  come back on any upward scroll. A browser's own chrome already takes 100–140 px; a home-screen
  install does not.

## 3. Options at 360–390 px

Bar 56 px plus the inset; icon 24 px, label 11–12 px; each cell at least 48 px tall and 64 px wide.

**A. Five labelled tabs, Watch Together in Menu (recommended)**

```
+---------------------------------------+
|  [home]  [film]  [tv]  [search] [menu]|
|  Home    Films  Series Search    Menu |
+---------------------------------------+
```

Menu is a page, not a dropdown: Watch Together, Cast, This device, Your account, About, sign out,
then the admin's Manage. Watch Together costs two taps; a live session or invitation shows as a
banner on Home, one tap. Films and Series open straight to their grids, no library dropdown.

**B. Four tabs and More**: Search back to an icon on each page, the problem the audit found. No.

**C. Six tabs**: about 60 px a cell at 360 px, under the 64 px floor, and "Watch Together" does not
fit; past the three-to-five guidance. No.

## Sources

MacRumors 2018-02-26; Android Police 2018-04-17; iPhone in Canada 2026-04-30; 9to5Google
2024-11-16; TechCrunch 2024-11-22; TechHive 2528572; support.plex.tv "navigating the mobile apps";
MobileSyrup 2025-04-01; forums.plex.tv 896427, 897253; Neowin (Plex reverts TV layout); PiunikaWeb
2026-05-04; AFB AccessWorld summer 2026; Pratt IXD 2020-02; developer.android.com navigation-bar;
developer.apple.com HIG tab-bars; Smashing Magazine 2017-05; Adobe Spectrum tab-bar-ios; Apple
Developer Forums 800798; dev.to samuelkraft (Safari 15 tab bars); zenn.dev jackmiwamiwa.
