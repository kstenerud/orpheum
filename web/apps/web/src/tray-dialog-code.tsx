// SPDX-License-Identifier: MPL-2.0

import type { CastDialog as CastDialogComponent } from "./cast-dialog.tsx";
import { lazyScreen } from "./screen-load.tsx";
import type { TogetherDialog as TogetherDialogComponent } from "./together-dialog.tsx";

const loadCastDialog = async (): Promise<{ default: typeof CastDialogComponent }> => ({
  default: (await import("./cast-dialog.tsx")).CastDialog,
});
const loadTogetherDialog = async (): Promise<{ default: typeof TogetherDialogComponent }> => ({
  default: (await import("./together-dialog.tsx")).TogetherDialog,
});

/**
 * The status tray's two dialogs (D384 §2), whose code loads as each is first opened, as a later
 * screen's does (later-screens.tsx), shown in a LaterScreen: the casting and Watch Together code each
 * holds is no part of the script every page loads first. Kept apart from later-screens.tsx, whose
 * screens draw the shell the tray is in.
 *
 * Each is `let`, not `const`, for resetTrayDialogsForTests, as later-screens.tsx's are.
 */
export let CastDialog = lazyScreen("cast-dialog", loadCastDialog);
export let TogetherDialog = lazyScreen("together-dialog", loadTogetherDialog);

/**
 * Test-only: gives each dialog here a fresh, unresolved React.lazy wrapper, as
 * resetLazyScreensForTests does the screens (later-screens.tsx). Never called by runtime code.
 */
export function resetTrayDialogsForTests(): void {
  CastDialog = lazyScreen("cast-dialog", loadCastDialog);
  TogetherDialog = lazyScreen("together-dialog", loadTogetherDialog);
}
