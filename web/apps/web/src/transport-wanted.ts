// SPDX-License-Identifier: MPL-2.0

/**
 * Whether a tab asked for the WebRTC path, read with nothing of the path loaded: the client's entry
 * reads it on every start and loads the path itself (transport-choice.ts), the WebRTC SDK with it,
 * only when it was asked for (the web client's architecture audit of 2026-11-10, Startup 6).
 */

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

/** The address parameter that asks for the WebRTC path: `?transport=webrtc`. */
export const transportParameter = "transport";

/** What the WebRTC path is called where the console says requests moved to it. */
export const pathName = "WebRTC";

/** Where the choice is kept for the rest of the tab's life, as the address loses it on navigating. */
const transportStorageKey = "orpheum.transport";

/**
 * Whether this tab sends its requests over WebRTC data channels (D273 §8). Until the discovery
 * signals for a core HTTPS cannot reach (plans/webrtc-transport.md W4), the path is asked for by
 * `?transport=webrtc`, which tests and the owner use to exercise it; the tab keeps the choice.
 */
export function wantsWebrtc(search: string, tabStorage: StringStorage | undefined): boolean {
  const storage = storageThatNeverThrows(tabStorage);
  const asked = new URLSearchParams(search).get(transportParameter);
  if (asked !== null) {
    storage.setItem(transportStorageKey, asked);
  }
  return storage.getItem(transportStorageKey) === "webrtc";
}
