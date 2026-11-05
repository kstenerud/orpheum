// SPDX-License-Identifier: MPL-2.0

import { preloadableScreen, resetOnEachTestMount } from "./screen-load.tsx";
import type { TogetherDialog as TogetherDialogComponent } from "./together-dialog.tsx";

const loadTogetherDialog = async (): Promise<{ default: typeof TogetherDialogComponent }> => ({
  default: (await import("./together-dialog.tsx")).TogetherDialog,
});

/**
 * Watch Together's dialog in a chunk of its own, one of the tray's two (tray-dialog-code.tsx), apart
 * from Cast's so that the sign over the film (together-over-the-film.tsx, D433) opens the same
 * dialog without reaching casting's code: through it, the player would import itself (the
 * no-circular rule). The tray loads it ahead with Cast's.
 *
 * `let`, not `const`, for resetTogetherDialogForTests, as later-screens.tsx's are.
 */
let togetherDialog = preloadableScreen("together-dialog", loadTogetherDialog);
export let TogetherDialog = togetherDialog.component;

/** Loads the dialog's code ahead; a failure is left to the press. */
export async function preloadTogetherDialog(): Promise<void> {
  await togetherDialog.preload();
}

/** Whether the dialog's code has loaded, so a press opens it at once. */
export function isTogetherDialogLoaded(): boolean {
  return togetherDialog.isLoaded();
}

/**
 * Test-only: gives the dialog a fresh, unloaded wrapper, as resetLazyScreensForTests does the
 * screens (later-screens.tsx). Never called by runtime code.
 */
export function resetTogetherDialogForTests(): void {
  togetherDialog = preloadableScreen("together-dialog", loadTogetherDialog);
  TogetherDialog = togetherDialog.component;
}
resetOnEachTestMount(resetTogetherDialogForTests);
