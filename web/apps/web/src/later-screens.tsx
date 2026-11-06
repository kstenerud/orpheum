// SPDX-License-Identifier: MPL-2.0

import type { CastReady as CastReadyComponent } from "./cast-ready.tsx";
import type { LabExtendPlayer as LabExtendPlayerComponent } from "./lab-extend-player.tsx";
import type { Lab as LabComponent } from "./lab.tsx";
import type { Play as PlayComponent } from "./play.tsx";
import type { Series as SeriesComponent } from "./series.tsx";
import type { Settings as SettingsComponent } from "./settings.tsx";
import type { Title as TitleComponent } from "./title.tsx";
import type { TogetherViewer as TogetherViewerComponent } from "./together-viewer.tsx";
import type { UiGallery as UiGalleryComponent } from "./ui-gallery.tsx";
import type { Identify as IdentifyComponent } from "./identify.tsx";
import type { Jobs as JobsComponent } from "./jobs.tsx";
import type { System as SystemComponent } from "./system.tsx";
import type { HowToUpdate as HowToUpdateComponent } from "./update-page.tsx";
import type { NeedsAttention as NeedsAttentionComponent } from "./needs-attention.tsx";
import type { Users as UsersComponent } from "./users.tsx";
import type { UserPage as UserPageComponent } from "./user.tsx";
import type { Libraries as LibrariesComponent } from "./libraries.tsx";
import type { LibraryPage as LibraryPageComponent } from "./library-manage.tsx";
import type { CreateLibraryPage as CreateLibraryPageComponent } from "./create-library.tsx";
import type { Files as FilesComponent } from "./files.tsx";

import { lazyScreen, resetOnEachTestMount } from "./screen-load.tsx";

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
const loadTogetherViewer = async (): Promise<{ default: typeof TogetherViewerComponent }> => ({
  default: (await import("./together-viewer.tsx")).TogetherViewer,
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
const loadIdentify = async (): Promise<{ default: typeof IdentifyComponent }> => ({
  default: (await import("./identify.tsx")).Identify,
});
const loadJobs = async (): Promise<{ default: typeof JobsComponent }> => ({
  default: (await import("./jobs.tsx")).Jobs,
});
const loadSystem = async (): Promise<{ default: typeof SystemComponent }> => ({
  default: (await import("./system.tsx")).System,
});
const loadHowToUpdate = async (): Promise<{ default: typeof HowToUpdateComponent }> => ({
  default: (await import("./update-page.tsx")).HowToUpdate,
});
const loadNeedsAttention = async (): Promise<{ default: typeof NeedsAttentionComponent }> => ({
  default: (await import("./needs-attention.tsx")).NeedsAttention,
});
const loadUsers = async (): Promise<{ default: typeof UsersComponent }> => ({
  default: (await import("./users.tsx")).Users,
});
const loadUserPage = async (): Promise<{ default: typeof UserPageComponent }> => ({
  default: (await import("./user.tsx")).UserPage,
});
const loadLibraries = async (): Promise<{ default: typeof LibrariesComponent }> => ({
  default: (await import("./libraries.tsx")).Libraries,
});
const loadLibraryPage = async (): Promise<{ default: typeof LibraryPageComponent }> => ({
  default: (await import("./library-manage.tsx")).LibraryPage,
});
const loadCreateLibraryPage = async (): Promise<{
  default: typeof CreateLibraryPageComponent;
}> => ({
  default: (await import("./create-library.tsx")).CreateLibraryPage,
});
const loadFiles = async (): Promise<{ default: typeof FilesComponent }> => ({
  default: (await import("./files.tsx")).Files,
});

/**
 * The screens whose code loads only when one is first shown, each with what only it needs: a
 * title's and a series' pages, which make a film ready to play, the player and hls.js with it,
 * a cast's ready page, the server's settings and the admin's other screens (D361's libraries, users,
 * jobs, matching, what needs attention, System and How to update, which only an admin opens), the
 * device lab and its player, and the UI gallery, the lab's and the gallery's words with them
 * (lab-words.ts, gallery-words.ts). They were
 * all in the one script every page loaded before it drew, most of it hls.js, which a TV parses
 * slowly (the audit of 2026-10-25, W1); app.tsx shows each in a LaterScreen: Loading… meanwhile,
 * and a page that says so, with Reload, when its code does not load (screen-load.tsx). A screen
 * here is one the app reaches as a route, never one a home page draws; the status tray's dialogs load
 * so too, as it opens them (tray-dialog-code.tsx).
 *
 * Each is `let`, not `const`: resetLazyScreensForTests (below) reassigns it to a fresh, unresolved
 * React.lazy wrapper (lazyScreen). Runtime code never calls that; production makes each only once, here, at
 * module load, same as a const would.
 */
export let Title = lazyScreen("title", loadTitle);
export let Series = lazyScreen("series", loadSeries);
export let Play = lazyScreen("play", loadPlay);
export let TogetherViewer = lazyScreen("together-viewer", loadTogetherViewer);
export let CastReady = lazyScreen("cast-ready", loadCastReady);
export let Lab = lazyScreen("lab", loadLab);
export let LabExtendPlayer = lazyScreen("lab-extend-player", loadLabExtendPlayer);
export let Settings = lazyScreen("settings", loadSettings);
export let UiGallery = lazyScreen("ui-gallery", loadUiGallery);
export let Identify = lazyScreen("identify", loadIdentify);
export let Jobs = lazyScreen("jobs", loadJobs);
export let System = lazyScreen("system", loadSystem);
export let HowToUpdate = lazyScreen("update", loadHowToUpdate);
export let NeedsAttention = lazyScreen("needs-attention", loadNeedsAttention);
export let Users = lazyScreen("users", loadUsers);
export let UserPage = lazyScreen("user", loadUserPage);
export let Libraries = lazyScreen("libraries", loadLibraries);
export let LibraryPage = lazyScreen("library", loadLibraryPage);
export let CreateLibraryPage = lazyScreen("new-library", loadCreateLibraryPage);
export let Files = lazyScreen("files", loadFiles);

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
  TogetherViewer = lazyScreen("together-viewer", loadTogetherViewer);
  CastReady = lazyScreen("cast-ready", loadCastReady);
  Lab = lazyScreen("lab", loadLab);
  LabExtendPlayer = lazyScreen("lab-extend-player", loadLabExtendPlayer);
  Settings = lazyScreen("settings", loadSettings);
  UiGallery = lazyScreen("ui-gallery", loadUiGallery);
  Identify = lazyScreen("identify", loadIdentify);
  Jobs = lazyScreen("jobs", loadJobs);
  System = lazyScreen("system", loadSystem);
  HowToUpdate = lazyScreen("update", loadHowToUpdate);
  NeedsAttention = lazyScreen("needs-attention", loadNeedsAttention);
  Users = lazyScreen("users", loadUsers);
  UserPage = lazyScreen("user", loadUserPage);
  Libraries = lazyScreen("libraries", loadLibraries);
  LibraryPage = lazyScreen("library", loadLibraryPage);
  CreateLibraryPage = lazyScreen("new-library", loadCreateLibraryPage);
  Files = lazyScreen("files", loadFiles);
}
resetOnEachTestMount(resetLazyScreensForTests);
