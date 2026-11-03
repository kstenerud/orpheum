// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

import type { CastDialog as CastDialogComponent } from "./cast-dialog.tsx";
import { preloadableScreen, resetOnEachTestMount } from "./screen-load.tsx";
import type { TogetherDialog as TogetherDialogComponent } from "./together-dialog.tsx";
import { whenIdle } from "./when-idle.ts";

const loadCastDialog = async (): Promise<{ default: typeof CastDialogComponent }> => ({
  default: (await import("./cast-dialog.tsx")).CastDialog,
});
const loadTogetherDialog = async (): Promise<{ default: typeof TogetherDialogComponent }> => ({
  default: (await import("./together-dialog.tsx")).TogetherDialog,
});

/**
 * The status tray's two dialogs (D384 §2), each in a chunk of its own, as a later screen's code is
 * (later-screens.tsx), shown in a LaterScreen: the casting and Watch Together code each holds is no
 * part of the script every page loads first. Their code is loaded ahead once the first page has
 * shown and the browser is idle (useLoadsTrayDialogsAhead), so a first press opens the dialog at
 * once: it had shown "Loading…" in place of the sign until the code came (the owner, 2026-11-03,
 * plans/toolkit.md item 10). Kept apart from later-screens.tsx, whose screens draw the shell the tray
 * is in.
 *
 * Each is `let`, not `const`, for resetTrayDialogsForTests, as later-screens.tsx's are.
 */
let castDialog = preloadableScreen("cast-dialog", loadCastDialog);
let togetherDialog = preloadableScreen("together-dialog", loadTogetherDialog);
export let CastDialog = castDialog.component;
export let TogetherDialog = togetherDialog.component;

/**
 * How long the first page shows before the tray's dialogs' code is asked for, in ms: past its own
 * first requests, which the dialogs' code never delays, and well before a person reaches for a sign.
 * A justified constant.
 */
const trayDialogsAheadAfterMs = 1000;

/** How long the browser's idle is waited for once trayDialogsAheadAfterMs has passed, at most, in ms. */
const trayDialogsIdleWithinMs = 2000;

/** Loads both dialogs' code ahead, once per page load; a failure is left to the press. */
async function preloadTrayDialogs(): Promise<void> {
  await Promise.all([castDialog.preload(), togetherDialog.preload()]);
}

/** Whether both dialogs' code has loaded, so a press opens either at once. */
export function areTrayDialogsLoaded(): boolean {
  return castDialog.isLoaded() && togetherDialog.isLoaded();
}

/**
 * Loads the tray's dialogs' code (preloadTrayDialogs) once the page using it has shown for
 * trayDialogsAheadAfterMs and the browser is idle; a page left before then loads nothing.
 */
export function useLoadsTrayDialogsAhead(): void {
  useEffect(() => {
    let stopIdle: (() => void) | undefined = undefined;
    const timer = setTimeout(() => {
      stopIdle = whenIdle(() => {
        void preloadTrayDialogs();
      }, trayDialogsIdleWithinMs);
    }, trayDialogsAheadAfterMs);
    return () => {
      clearTimeout(timer);
      stopIdle?.();
    };
  }, []);
}

/**
 * Test-only: gives each dialog here a fresh, unloaded wrapper, as resetLazyScreensForTests does the
 * screens (later-screens.tsx). Never called by runtime code.
 */
export function resetTrayDialogsForTests(): void {
  castDialog = preloadableScreen("cast-dialog", loadCastDialog);
  togetherDialog = preloadableScreen("together-dialog", loadTogetherDialog);
  CastDialog = castDialog.component;
  TogetherDialog = togetherDialog.component;
}
resetOnEachTestMount(resetTrayDialogsForTests);
