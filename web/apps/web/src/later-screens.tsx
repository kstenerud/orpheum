// SPDX-License-Identifier: MPL-2.0

import { lazy } from "react";

/**
 * The screens whose code loads only when one is first shown, each with what only it needs: a
 * title's and a series' pages, which make a film ready to play, the player and hls.js with it,
 * Watch Together, casting, the server's settings, the device lab and its player, and the UI gallery. They were
 * all in the one script every page loaded before it drew, most of it hls.js, which a TV parses
 * slowly (the audit of 2026-10-25, W1); app.tsx shows Loading… meanwhile (ScreenLoading). A screen
 * here is one the app reaches as a route, never one a home page draws; the status tray loads Watch
 * Together's controls as it opens them (status-tray.tsx).
 */
export const Title = lazy(async () => ({ default: (await import("./title.tsx")).Title }));

export const Series = lazy(async () => ({ default: (await import("./series.tsx")).Series }));

export const Play = lazy(async () => ({ default: (await import("./play.tsx")).Play }));

export const WatchTogether = lazy(async () => ({
  default: (await import("./watch-together.tsx")).WatchTogether,
}));

export const TogetherViewer = lazy(async () => ({
  default: (await import("./together-viewer.tsx")).TogetherViewer,
}));

export const Casting = lazy(async () => ({ default: (await import("./cast-remote.tsx")).Casting }));

export const Lab = lazy(async () => ({ default: (await import("./lab.tsx")).Lab }));

export const LabExtendPlayer = lazy(async () => ({
  default: (await import("./lab-extend-player.tsx")).LabExtendPlayer,
}));

export const Settings = lazy(async () => ({ default: (await import("./settings.tsx")).Settings }));

export const UiGallery = lazy(async () => ({
  default: (await import("./ui-gallery.tsx")).UiGallery,
}));
