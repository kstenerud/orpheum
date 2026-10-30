// SPDX-License-Identifier: MPL-2.0

import { useSyncExternalStore } from "react";

/**
 * Which of the status tray's dialogs is open over the page, if any (D384 §2): Cast's, to choose a
 * device or stop, or Watch Together's, to run the session. Each opens from its sign and never moves
 * the page; an old address of the pages they replace may open one too. One at a time, in this tab.
 */
export type TrayDialog = "cast" | "together";

let open: TrayDialog | undefined;
const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** Opens dialog, in place of any other open. */
export function openTrayDialog(dialog: TrayDialog): void {
  open = dialog;
  tellChanged();
}

/** Closes whichever tray dialog is open. */
export function closeTrayDialog(): void {
  if (open !== undefined) {
    open = undefined;
    tellChanged();
  }
}

/** The tray dialog open now, if any. */
export function currentTrayDialog(): TrayDialog | undefined {
  return open;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The tray dialog open now, re-rendering as one opens or closes. */
export function useTrayDialog(): TrayDialog | undefined {
  return useSyncExternalStore(subscribe, currentTrayDialog, currentTrayDialog);
}
