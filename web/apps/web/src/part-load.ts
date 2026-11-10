// SPDX-License-Identifier: MPL-2.0

/**
 * How main.tsx loads its part, the loader or the client, and what it does when the part's code does
 * not come. Its import was left to fail unseen, and the page stayed black: the owner's Firefox, signing
 * in at staging's discovery for the first time, the client's "Unable to preload CSS for
 * …/busy-group-….css" thrown past everything, while a reload showed Home (2026-11-01). Under a
 * loader each of the client's files comes first through the page's new connection to its core, so
 * one that fails on a first visit comes on the next. The part is asked again once, after a pause
 * (screen-load.tsx's screenRetryMs: Vite's preload failing, the module itself was never imported,
 * and asking again loads it); failing again, the page reloads, once in the tab, leaving a note for
 * the page that follows to tell the core's log (reload-note.ts). A second time in the tab the
 * failure is thrown, as before; a part that loads clears the tab's mark, so a later time reloads
 * again.
 */

import type { StringStorage } from "./browser-storage.ts";
import { loaderConfigOf } from "./loader-config.ts";
import { loaderOf } from "./loader-interface.ts";
import { leaveReloadNote } from "./reload-note.ts";

/** How long the part's code is waited for before it is asked again (screen-load.tsx screenRetryMs). */
export const partRetryMs = 1000;

/** The tab's key for the reload a page made because its part's code did not load. */
const reloadedForPartKey = "orpheum.main.reloadedForPart";

/** The tab's key for the reload a loader's page made because its role's code did not load. */
const reloadedForRoleKey = "orpheum.main.reloadedForRole";

/** What loadPart needs of the page. */
export interface PartLoadParts {
  /** This tab's storage, which says whether it reloaded for this before. */
  readonly tab: StringStorage;
  readonly reload: () => void;
  /** Waits ms; setTimeout's, but for a test. */
  readonly wait: (ms: number) => Promise<void>;
}

/** Waits ms on the page's clock. */
export async function waitMs(ms: number): Promise<void> {
  await new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

/**
 * Loads part, named for the note, by load: asked again once after partRetryMs when it fails, then
 * reloading the page once in the tab, else throwing why. A page that reloads never settles.
 */
export async function loadPart<Loaded>(
  part: string,
  load: () => Promise<Loaded>,
  parts: PartLoadParts,
): Promise<Loaded> {
  return await loadMarked(reloadedForPartKey, part, load, parts);
}

/**
 * Loads the loader's role, the code of the origin it is served at, as loadPart loads a part, after
 * it (loader-main.tsx): its own mark in the tab, as the part's loading clears the part's, and a role
 * failing in the page reloaded for it would reload it again, each time.
 */
export async function loadRole<Loaded>(
  role: string,
  load: () => Promise<Loaded>,
  parts: PartLoadParts,
): Promise<Loaded> {
  return await loadMarked(reloadedForRoleKey, role, load, parts);
}

/** Loads part by load as loadPart says, the tab's reload for it marked at mark. */
async function loadMarked<Loaded>(
  mark: string,
  part: string,
  load: () => Promise<Loaded>,
  parts: PartLoadParts,
): Promise<Loaded> {
  try {
    const loaded = await load();
    parts.tab.removeItem(mark);
    return loaded;
  } catch {
    await parts.wait(partRetryMs);
  }
  try {
    const loaded = await load();
    parts.tab.removeItem(mark);
    return loaded;
  } catch (cause: unknown) {
    if (parts.tab.getItem(mark) !== null) {
      throw cause;
    }
    const reason = cause instanceof Error ? cause.message : String(cause);
    parts.tab.setItem(mark, "1");
    leaveReloadNote(
      parts.tab,
      `the page reloaded, once: the code of its ${part} did not load, asked twice (${reason})`,
    );
    parts.reload();
    return await new Promise<never>(() => undefined);
  }
}

/** The page's two parts, each loaded by its own import. */
export interface PageParts {
  readonly loader: () => Promise<unknown>;
  readonly client: () => Promise<unknown>;
}

/**
 * Loads the page's own part by loadPart (D308): one build, two roles. Every core serves it as its own
 * client, and the discovery serves it as the loader, which finds a core and runs that core's own
 * client in its place. The page says which it is: a discovery's page names its loader's
 * configuration, and a client a loader runs finds the loader on the window.
 */
export async function loadOwnPart(
  page: { readonly window: object; readonly document: Pick<Document, "querySelector"> },
  imports: PageParts,
  parts: PartLoadParts,
): Promise<void> {
  const isLoader =
    loaderOf(page.window) === undefined && loaderConfigOf(page.document) !== undefined;
  await (isLoader
    ? loadPart("loader", imports.loader, parts)
    : loadPart("client", imports.client, parts));
}
