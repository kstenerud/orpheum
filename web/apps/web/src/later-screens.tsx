// SPDX-License-Identifier: MPL-2.0

import type { CastReady as CastReadyComponent } from "./cast-ready.tsx";
import type { Casting as CastingComponent } from "./cast-remote.tsx";
import type { LabExtendPlayer as LabExtendPlayerComponent } from "./lab-extend-player.tsx";
import type { Lab as LabComponent } from "./lab.tsx";
import type { Play as PlayComponent } from "./play.tsx";
import type { Series as SeriesComponent } from "./series.tsx";
import type { Settings as SettingsComponent } from "./settings.tsx";
import type { Title as TitleComponent } from "./title.tsx";
import type { TogetherViewer as TogetherViewerComponent } from "./together-viewer.tsx";
import type { UiGallery as UiGalleryComponent } from "./ui-gallery.tsx";
import type { WatchTogether as WatchTogetherComponent } from "./watch-together.tsx";

import { lazyScreen } from "./screen-load.tsx";

/** Each screen's own loader, named so resetLazyScreensForTests can give it a fresh lazy() too. */
const loadTitle = async (): Promise<{ default: typeof TitleComponent }> => ({
  default: (await import("./title.tsx")).Title,
});
const loadSeries = async (): Promise<{ default: typeof SeriesComponent }> => ({
  default: (await import("./series.tsx")).Series,
});
const loadPlay = async (): Promise<{ default: typeof PlayComponent }> => ({
  default: (await import("./play.tsx")).Play,
});
const loadWatchTogether = async (): Promise<{ default: typeof WatchTogetherComponent }> => ({
  default: (await import("./watch-together.tsx")).WatchTogether,
});
const loadTogetherViewer = async (): Promise<{ default: typeof TogetherViewerComponent }> => ({
  default: (await import("./together-viewer.tsx")).TogetherViewer,
});
const loadCasting = async (): Promise<{ default: typeof CastingComponent }> => ({
  default: (await import("./cast-remote.tsx")).Casting,
});
const loadCastReady = async (): Promise<{ default: typeof CastReadyComponent }> => ({
  default: (await import("./cast-ready.tsx")).CastReady,
});
const loadLab = async (): Promise<{ default: typeof LabComponent }> => ({
  default: (await import("./lab.tsx")).Lab,
});
const loadLabExtendPlayer = async (): Promise<{ default: typeof LabExtendPlayerComponent }> => ({
  default: (await import("./lab-extend-player.tsx")).LabExtendPlayer,
});
const loadSettings = async (): Promise<{ default: typeof SettingsComponent }> => ({
  default: (await import("./settings.tsx")).Settings,
});
const loadUiGallery = async (): Promise<{ default: typeof UiGalleryComponent }> => ({
  default: (await import("./ui-gallery.tsx")).UiGallery,
});

/**
 * The screens whose code loads only when one is first shown, each with what only it needs: a
 * title's and a series' pages, which make a film ready to play, the player and hls.js with it,
 * Watch Together, casting and a cast's ready page, the server's settings, the device lab and its player, and the UI gallery. They were
 * all in the one script every page loaded before it drew, most of it hls.js, which a TV parses
 * slowly (the audit of 2026-10-25, W1); app.tsx shows each in a LaterScreen: Loading… meanwhile,
 * and a page that says so, with Reload, when its code does not load (screen-load.tsx). A screen
 * here is one the app reaches as a route, never one a home page draws; the status tray loads Watch
 * Together's controls as it opens them (status-tray.tsx).
 *
 * Each is `let`, not `const`: resetLazyScreensForTests (below) reassigns it to a fresh, unresolved
 * React.lazy wrapper (lazyScreen). Runtime code never calls that; production makes each only once, here, at
 * module load, same as a const would.
 */
export let Title = lazyScreen("title", loadTitle);
export let Series = lazyScreen("series", loadSeries);
export let Play = lazyScreen("play", loadPlay);
export let WatchTogether = lazyScreen("watch-together", loadWatchTogether);
export let TogetherViewer = lazyScreen("together-viewer", loadTogetherViewer);
export let Casting = lazyScreen("casting", loadCasting);
export let CastReady = lazyScreen("cast-ready", loadCastReady);
export let Lab = lazyScreen("lab", loadLab);
export let LabExtendPlayer = lazyScreen("lab-extend-player", loadLabExtendPlayer);
export let Settings = lazyScreen("settings", loadSettings);
export let UiGallery = lazyScreen("ui-gallery", loadUiGallery);

/**
 * Test-only: gives every screen here a fresh, unresolved React.lazy wrapper, so a mount through
 * test-dom's mount() meets each cold, its code loading and its Suspense fallback showing, regardless
 * of what an earlier test in the same file already resolved. Without this, a test could pass only
 * because an earlier one in its file had already warmed the same singleton (the shared module state
 * found for status-tray.tsx's own lazy import, test-architecture-2026-10-26.md; here the same risk
 * for every screen in this file, none of which has hit it yet). Never called by runtime code.
 */
export function resetLazyScreensForTests(): void {
  Title = lazyScreen("title", loadTitle);
  Series = lazyScreen("series", loadSeries);
  Play = lazyScreen("play", loadPlay);
  WatchTogether = lazyScreen("watch-together", loadWatchTogether);
  TogetherViewer = lazyScreen("together-viewer", loadTogetherViewer);
  Casting = lazyScreen("casting", loadCasting);
  CastReady = lazyScreen("cast-ready", loadCastReady);
  Lab = lazyScreen("lab", loadLab);
  LabExtendPlayer = lazyScreen("lab-extend-player", loadLabExtendPlayer);
  Settings = lazyScreen("settings", loadSettings);
  UiGallery = lazyScreen("ui-gallery", loadUiGallery);
}
