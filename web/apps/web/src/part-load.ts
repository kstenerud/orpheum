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
 * the page that follows to tell the core's log (reload-note.ts). A part that loads clears the tab's
 * mark, so a later time reloads again.
 *
 * A module that failed is asked again under an address of its own (moduleAddressOf): Chromium keeps
 * a failed import failed for the document's life and asks nothing again for it (idiosyncrasies: "A
 * dynamic import that failed fails again without a request"). The page reloaded for its code no
 * longer gives up: it threw there, and the page stayed black with nothing said after a deploy, until
 * the person reloaded it by hand (the owner's Firefox, the loader's role at a client's origin,
 * staging, the 11-26 batch's item 1). It asks again by itself, each pause twice the one before up to
 * partAskCapMs, until the code comes, and never reloads again, so it cannot loop.
 *
 * While a loader's page waits for its role's code it shows a note, Loading…, drawn by what has
 * loaded already (loader-main.tsx), never by the code it waits for: the page was black for up to
 * half a minute while it asked on, and the owner chose "show loading" (the plan's "The black page's
 * wait, this batch", 2026-11-15). The note comes only once the wait has lasted partNoteAfterMs, and
 * goes as the code comes, before the page draws.
 */

import type { StringStorage } from "./browser-storage.ts";
import { loaderConfigOf } from "./loader-config.ts";
import { loaderOf } from "./loader-interface.ts";
import { leaveReloadNote } from "./reload-note.ts";

/** How long the part's code is waited for before it is asked again (screen-load.tsx screenRetryMs). */
export const partRetryMs = 1000;

/** The longest pause between the asks of a page reloaded for its part's code. */
export const partAskCapMs = 30_000;

/**
 * How long a page waits for its role's code before it shows its note: twice partRetryMs, so the
 * ordinary recovery, the code asked again once and coming at once, never flashes it, while a page
 * that asks on, or whose asks are slow, shows it within a few seconds. A justified constant, not a
 * setting.
 */
export const partNoteAfterMs = 2 * partRetryMs;

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
  /** Imports the module at address: importModuleAt, but for a test. */
  readonly importAt: (address: string) => Promise<unknown>;
}

/**
 * Waits ms on the page's clock: the SDK's waitMs, written again here for the shell's entry alone,
 * which loads no SDK so the page's first bytes stay small (home-imports.test.ts, the architecture
 * audit of 2026-11-10, Startup 5). Everything else waits with the SDK's.
 */
export async function shellWaitMs(ms: number): Promise<void> {
  await new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

/** Imports the module at address, a failed module's code asked for again under an address of its own. */
export async function importModuleAt(address: string): Promise<unknown> {
  const loaded: unknown = await import(/* @vite-ignore */ address);
  return loaded;
}

/**
 * The address of the module a failed import names, without its query: Chromium's "Failed to fetch
 * dynamically imported module: …" and Firefox's "error loading dynamically imported module: …" each
 * end with it. Undefined where the failure names none (Safari's, or Vite's failed preload of a
 * stylesheet, which asking by the same import mends).
 */
export function moduleAddressOf(failure: unknown): string | undefined {
  const reason = failure instanceof Error ? failure.message : String(failure);
  const named = /dynamically imported module: (\S+)$/u.exec(reason)?.[1];
  return named?.split("?")[0];
}

/**
 * Says a module asked for again is the code that exports name, a function, as a role's module does
 * (loader-main.tsx).
 */
export function exportingFunction<Module>(
  name: keyof Module & string,
): (loaded: unknown) => loaded is Module {
  return (loaded: unknown): loaded is Module =>
    typeof loaded === "object" &&
    loaded !== null &&
    typeof Reflect.get(loaded, name) === "function";
}

/** Whatever a part's module is: a part is loaded for what it does, not for what it hands back. */
function isAnyPart(loaded: unknown): loaded is unknown {
  return loaded !== undefined;
}

/**
 * Loads part, named for the note, by load: asked again once after partRetryMs when it fails, then
 * reloading the page once in the tab; the page reloaded for it asks on until it comes. A page that
 * reloads never settles.
 */
export async function loadPart(
  part: string,
  load: () => Promise<unknown>,
  parts: PartLoadParts,
): Promise<unknown> {
  return await loadMarked({ mark: reloadedForPartKey, part, isLoaded: isAnyPart }, load, parts);
}

/** A loader's role's code: its import, and how a module asked for again is told for it. */
export interface RoleCode<Loaded> {
  readonly load: () => Promise<Loaded>;
  /** Says a module asked for again under an address of its own is the role's (exportingFunction). */
  readonly isRole: (loaded: unknown) => loaded is Loaded;
}

/**
 * Loads the loader's role, the code of the origin it is served at, as loadPart loads a part, after
 * it (loader-main.tsx): its own mark in the tab, as the part's loading clears the part's, and a role
 * failing in the page reloaded for it would reload it again, each time. Waiting partNoteAfterMs
 * for it, the page shows its note, until the role's code comes.
 */
export async function loadRole<Loaded>(
  role: string,
  code: RoleCode<Loaded>,
  parts: RoleLoadParts,
): Promise<Loaded> {
  const takeNoteAway = noteAfterWaiting(parts.note);
  try {
    return await loadMarked(
      { mark: reloadedForRoleKey, part: role, isLoaded: code.isRole },
      code.load,
      parts,
    );
  } finally {
    takeNoteAway();
  }
}

/**
 * The note a page shows while it waits for its role's code, Loading…: drawn by what has loaded
 * already, never by the code it waits for.
 */
export interface WaitingNote {
  /** Shows the note, handing back what takes it away. */
  readonly show: () => () => void;
}

/** What loadRole needs of the page: loadPart's, and the note it shows while it waits. */
export interface RoleLoadParts extends PartLoadParts {
  readonly note: WaitingNote;
}

/**
 * Shows note once partNoteAfterMs has passed, handing back what calls it off, or takes it away once
 * shown: the page's own setTimeout, as shellWaitMs's.
 */
function noteAfterWaiting(note: WaitingNote): () => void {
  let takeAway: (() => void) | undefined = undefined;
  const timer = setTimeout(() => {
    takeAway = note.show();
  }, partNoteAfterMs);
  return () => {
    clearTimeout(timer);
    takeAway?.();
  };
}

/** What loadMarked loads: its part, named for the note, the tab's mark for its reload, and how it is told. */
interface Marked<Loaded> {
  readonly mark: string;
  readonly part: string;
  readonly isLoaded: (loaded: unknown) => loaded is Loaded;
}

/** Loads part by load as loadPart says, the tab's reload for it marked at mark. */
async function loadMarked<Loaded>(
  marked: Marked<Loaded>,
  load: () => Promise<Loaded>,
  parts: PartLoadParts,
): Promise<Loaded> {
  // The module that failed, once a failure names it: asked for again under addresses of its own,
  // though a later failure (the module asked for not the part's) names none.
  let address: string | undefined;
  let failure: unknown;
  try {
    return loaded(marked, await load(), parts);
  } catch (cause: unknown) {
    address = moduleAddressOf(cause);
  }
  await parts.wait(partRetryMs);
  try {
    return loaded(marked, await askedAgain(marked, { load, address, asked: 2 }, parts), parts);
  } catch (cause: unknown) {
    failure = cause;
    address = moduleAddressOf(cause) ?? address;
  }
  if (parts.tab.getItem(marked.mark) === null) {
    const reason = failure instanceof Error ? failure.message : String(failure);
    parts.tab.setItem(marked.mark, "1");
    leaveReloadNote(
      parts.tab,
      `the page reloaded, once: the code of its ${marked.part} did not load, asked twice (${reason})`,
    );
    parts.reload();
    return await new Promise<never>(() => undefined);
  }
  let pause = partRetryMs;
  for (let asked = 3; ; asked += 1) {
    pause = Math.min(pause * 2, partAskCapMs);
    await parts.wait(pause);
    try {
      return loaded(marked, await askedAgain(marked, { load, address, asked }, parts), parts);
    } catch (cause: unknown) {
      address = moduleAddressOf(cause) ?? address;
    }
  }
}

/** The part's code, come: the tab's mark cleared, so a later failure reloads again. */
function loaded<Loaded>(marked: Marked<Loaded>, code: Loaded, parts: PartLoadParts): Loaded {
  parts.tab.removeItem(marked.mark);
  return code;
}

/**
 * The part's code asked for again, the asked-th ask: when a failure named its module, at address
 * under an address of its own, which a browser that keeps the failed one asks for afresh; else by
 * load.
 */
async function askedAgain<Loaded>(
  marked: Marked<Loaded>,
  again: {
    readonly load: () => Promise<Loaded>;
    readonly address: string | undefined;
    readonly asked: number;
  },
  parts: PartLoadParts,
): Promise<Loaded> {
  const { address } = again;
  if (address === undefined) {
    return await again.load();
  }
  const code = await parts.importAt(`${address}?asked=${String(again.asked)}`);
  if (!marked.isLoaded(code)) {
    throw new Error(`${address} is not the code of its ${marked.part}`);
  }
  return code;
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
