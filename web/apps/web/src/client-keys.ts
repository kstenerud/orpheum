// SPDX-License-Identifier: MPL-2.0

/**
 * The keys the client keeps in the browser's storage, as one list (the owner, 2026-10-17: "clear
 * anything attributed to it that it doesn't recognize"): at its start the client removes every
 * other key under Orpheum's prefix, which an older release left. A key is named here, or it is
 * cleared at the next start; client-keys.test.ts fails for a key the source names that this does not.
 */

import { removeUnownedKeys } from "./browser-storage.ts";

/** The keys the client keeps by name. */
const clientKeys = new Set([
  // The session on the core's own origin (session.ts, its prefix "orpheum.").
  "orpheum.session",
  "orpheum.deviceId",
  "orpheum.deviceProof",
  "orpheum.locator",
  // Watch Together (together-storage.ts): a tab's hosting and a viewer's membership.
  "orpheum.together.host",
  "orpheum.together.viewer",
  // That the session a guest by link alone was in ended in this tab, for its page loaded again.
  "orpheum.together.endedHere",
  // A device being activated by a code (pairing.ts).
  "orpheum.pairing",
  // The tab's path choice (transport-choice.ts) and the script it last reloaded for (app-update.ts).
  "orpheum.transport",
  "orpheum.reloadedForClient",
  // The build the tab reloaded from for a screen's code no longer served, and why (newer-client.ts).
  "orpheum.screenCode.reloadedFrom",
  "orpheum.screenCode.reloadNote",
  // A debugging aid that starts each film on its own picture, with no preload (watching.ts).
  "orpheum.debug.startOnFilmsPicture",
  // The tab's debug cap on its download rate (link-debug.ts, D380).
  "orpheum.debug.linkCapKbps",
  // The realm a person asked the page to remember (discovery-start.ts).
  "orpheum.discovery.realm",
  // The custom Cast receiver's app id the lab's cast probe launches (lab-cast.tsx).
  "orpheum.lab.castAppId",
  // The core's LAN address the lab's throughput test loads from (lab-throughput.tsx).
  "orpheum.lab.lanAddress",
  // Which face button of a game controller is OK on this browser (controller-choice.ts).
  "orpheum.controller.ok",
  // Whether the tab sends a cast, so a film it plays goes on to its receiver (cast.ts).
  "orpheum.cast.sending",
  // That the tab receives a cast, for its page loaded again (cast-receiving-mark.ts).
  "orpheum.cast.receiving",
  // The latency of this device's sound through Web Audio, as last measured (sound-latency.ts).
  "orpheum.sound.latencyMs",
  // That the note on the browser's local-network prompt was dismissed (local-network-note.tsx).
  "orpheum.localNetwork.noteDismissed",
]);

/**
 * The prefixes of keys others keep on the client's origin: under a discovery's loader, the session
 * it keeps for the core, by the core's id (session.ts corePrefix), and its own (D308).
 */
const kept = ["orpheum.core.", "orpheum.loader."];

/** Whether the client, or the loader running it, keeps key. */
export function isClientKey(key: string): boolean {
  return clientKeys.has(key) || kept.some((prefix) => key.startsWith(prefix));
}

/** Removes every key under Orpheum's prefix that no part of this release keeps, from each storage. */
export function removeLeftOverKeys(storages: readonly (Storage | undefined)[]): void {
  for (const storage of storages) {
    removeUnownedKeys(storage, isClientKey);
  }
}
