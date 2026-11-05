// SPDX-License-Identifier: MPL-2.0

/**
 * The one way a screen's code is loaded as it is first shown (later-screens.tsx), and what the page
 * says when it does not come. A screen whose module failed to load left the page black: the home page
 * of the owner's Chrome, through staging's discovery, loaded, and a film's page went black, its
 * screen's chunks answered 404 at the client origin and React's lazy loading throwing "Failed to
 * fetch dynamically imported module" past everything (2026-10-29). A tab open across a deploy, asking
 * for a chunk whose name is gone, meets the same.
 */

import { reportFailure } from "@orpheum/sdk";
import {
  Component,
  createElement,
  lazy,
  Suspense,
  useState,
  type ComponentType,
  type JSX,
  type LazyExoticComponent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import { clientAssetsPrefix } from "./client-files.ts";
import { copyToClipboard } from "./clipboard.ts";
import { loaderOf } from "./loader-interface.ts";
import { reloadsForNewerClient } from "./newer-client.ts";
import { CopyWhatHappened } from "./what-happened.tsx";

/**
 * How long a screen's code is waited for before it is asked again, once its first load failed: long
 * enough for a worker or a connection starting again, short enough that no one waits on a blank page.
 * A justified constant, not a setting. Chromium, as the HTML standard says, answers an import of a
 * module whose load failed with that failure (idiosyncrasies.md), so there the page that follows is
 * what recovers it.
 */
export const screenRetryMs = 1000;

/** A screen's code that did not load, asked twice: the screen named, and why. */
export class ScreenCodeNotLoaded extends Error {
  readonly screen: string;
  readonly reason: string;

  constructor(screen: string, cause: unknown) {
    const reason = cause instanceof Error ? cause.message : String(cause);
    super(`the code of the ${screen} screen did not load, asked twice: ${reason}`, { cause });
    this.name = "ScreenCodeNotLoaded";
    this.screen = screen;
    this.reason = reason;
  }
}

/** Whether a service worker controls this page, and whether it runs under a discovery's loader. */
export interface PageWorker {
  readonly isUnderLoader: boolean;
  readonly isControlled: boolean;
}

/** This page's worker, as the browser says. */
export function pageWorkerOf(page: Window): PageWorker {
  const { navigator } = page;
  return {
    isUnderLoader: loaderOf(page) !== undefined,
    isControlled: "serviceWorker" in navigator && navigator.serviceWorker.controller !== null,
  };
}

/**
 * How long a screen's code may take to load before it tells the core's log where the time went:
 * far past any ordinary load, which answers from the browser's cache or one quick fetch, and far
 * short of a person wondering why "Loading…" stays up — let alone the owner's Chrome, staging,
 * which showed it for near a minute with nothing told the core's log at all (VQ137, 2026-10-29). A
 * justified constant, not a setting.
 */
export const screenLoadSlowAfterMs = 3000;

/** One try to load a screen's code: how long it took, and what happened, "ok" or why it failed. */
interface LoadAttempt {
  readonly ms: number;
  readonly outcome: string;
}

/** Runs load() once, timed on nowMs, keeping its time and outcome in attempts either way. */
async function timedLoad<T>(
  load: () => Promise<T>,
  nowMs: () => number,
  attempts: LoadAttempt[],
): Promise<T> {
  const startedMs = nowMs();
  try {
    const result = await load();
    attempts.push({ ms: Math.round(nowMs() - startedMs), outcome: "ok" });
    return result;
  } catch (cause) {
    attempts.push({
      ms: Math.round(nowMs() - startedMs),
      outcome: cause instanceof Error ? cause.message : String(cause),
    });
    throw cause;
  }
}

/**
 * Whether entry has the numeric field named: PerformanceResourceTiming's workerStart and
 * transferSize, which a plain PerformanceEntry does not carry.
 */
function numberFieldOf(entry: PerformanceEntry, field: string): number | undefined {
  const value: unknown = Reflect.get(entry, field);
  return typeof value === "number" ? value : undefined;
}

/** entry's URL's path, where it has one it can be read as; undefined where it cannot be parsed. */
function pathnameOf(entry: PerformanceEntry): string | undefined {
  try {
    return new URL(entry.name).pathname;
  } catch {
    return undefined;
  }
}

/** Whether a worker intercepted entry's load at all (PerformanceResourceTiming's workerStart). */
function wasIntercepted(entry: PerformanceEntry): boolean {
  return (numberFieldOf(entry, "workerStart") ?? 0) > 0;
}

/** Whether entry is screen's chunk, loaded at sinceMs or after, that a worker intercepted. */
function isScreensChunk(entry: PerformanceEntry, screen: string, sinceMs: number): boolean {
  const prefix = `${clientAssetsPrefix}${screen}-`;
  return (
    entry.startTime >= sinceMs &&
    (pathnameOf(entry)?.includes(prefix) ?? false) &&
    wasIntercepted(entry)
  );
}

/**
 * How a loader's service worker answered screen's chunk, read from the browser's own record of its
 * load (PerformanceResourceTiming), the entry of it loaded at sinceMs or after: answered from what
 * the worker kept, with no bytes sent for it over the network, else how many it sent for; undefined
 * where the browser says nothing of it, or no worker intercepted its load at all. A best effort,
 * never the worker's own word (the simplest reliable one; a worker's note would need a new message
 * for it, kept to sw.ts's own job).
 */
export function workerAnswerOf(
  screen: string,
  sinceMs: number,
  entries: readonly PerformanceEntry[] = globalThis.performance.getEntriesByType("resource"),
): string | undefined {
  const latest = entries.filter((entry) => isScreensChunk(entry, screen, sinceMs)).at(-1);
  if (latest === undefined) {
    return undefined;
  }
  const transferSize = numberFieldOf(latest, "transferSize") ?? 0;
  return transferSize === 0
    ? "the service worker answered from what it kept"
    : `the service worker answered, ${String(transferSize)} bytes over the network`;
}

/** Each attempt's line for a report: its number from 1, how long it took, and what happened. */
function attemptLines(attempts: readonly LoadAttempt[]): string {
  return attempts
    .map(
      (attempt, index) =>
        `attempt ${String(index + 1)}: ${String(attempt.ms)} ms (${attempt.outcome})`,
    )
    .join("; ");
}

/**
 * Loads a screen's code, asking again once after retryAfterMs when it fails; failing again for a
 * newer client served, the page reloads into it, once (newer-client.ts); else it tells the core's log
 * which screen, why and whether a worker controlled the page, and throws ScreenCodeNotLoaded for
 * LaterScreen to show. Either way, once the total reaches screenLoadSlowAfterMs, it tells the core's
 * log the route, the total, each attempt's time and outcome, and, under a loader, what the browser's
 * own record says of how the worker answered the chunk (VQ137).
 */
export async function loadScreenCode<T>(
  screen: string,
  load: () => Promise<T>,
  retryAfterMs: number = screenRetryMs,
): Promise<T> {
  const nowMs = (): number => performance.now();
  const startedMs = nowMs();
  const attempts: LoadAttempt[] = [];
  const reportIfSlow = (): void => {
    const totalMs = Math.round(nowMs() - startedMs);
    if (totalMs < screenLoadSlowAfterMs) {
      return;
    }
    const servedBy = pageWorkerOf(window).isUnderLoader
      ? workerAnswerOf(screen, startedMs)
      : undefined;
    reportFailure(
      `the ${screen} screen's code took ${String(totalMs)} ms to load: ${attemptLines(attempts)}` +
        (servedBy === undefined ? "" : `; ${servedBy}`),
    );
  };
  try {
    const result = await timedLoad(load, nowMs, attempts);
    reportIfSlow();
    return result;
  } catch {
    await new Promise((resolve) => {
      setTimeout(resolve, retryAfterMs);
    });
  }
  try {
    const result = await timedLoad(load, nowMs, attempts);
    reportIfSlow();
    return result;
  } catch (cause: unknown) {
    reportIfSlow();
    const failed = new ScreenCodeNotLoaded(screen, cause);
    const worker = pageWorkerOf(window);
    if (await reloadsForNewerClient(failed, worker)) {
      // Loading… until the page goes: the newer client it reloads into shows the screen.
      return await new Promise<never>(() => undefined);
    }
    reportFailure(
      new Error(
        `${failed.message} (under a loader: ${yesNo(worker.isUnderLoader)}, a service worker controls the page: ${yesNo(worker.isControlled)})`,
      ),
    );
    throw failed;
  }
}

/**
 * The resets of every module loaded so far that holds state for the tab, which test-dom's mount()
 * and test-walk's mountApp() run.
 */
const lazyResets = new Set<() => void>();

/**
 * Test-only: a module of lazy screens (later-screens.tsx, tray-dialog-code.tsx), or one holding
 * what the tab remembers of its pages (remembered.ts's rememberedInTheTab), gives its reset here as
 * it loads, so a mount meets its screens cold and its pages unremembered without test-dom importing
 * it: an import from the helper made every test related to every screen, half the suite rerun for
 * one changed screen (the test-regime audit of 2026-10-31, R4). Only a module that loaded has state
 * to reset.
 */
export function resetOnEachTestMount(reset: () => void): void {
  lazyResets.add(reset);
}

/** Test-only: runs every reset given to resetOnEachTestMount. Never called by runtime code. */
export function resetLazyScreensOnMount(): void {
  for (const reset of lazyResets) {
    reset();
  }
}

/** A screen whose code loads as it is first shown, by loadScreenCode. */
export function lazyScreen<P extends object>(
  screen: string,
  load: () => Promise<{ default: ComponentType<P> }>,
  retryAfterMs?: number,
): LazyExoticComponent<ComponentType<P>> {
  return lazy(async () => await loadScreenCode(screen, load, retryAfterMs));
}

/** A lazily loaded screen (lazyScreen) whose code can be loaded ahead of its first showing. */
export interface PreloadableScreen<P extends object> {
  /** The screen: once its code has loaded, shown at once, never Loading… in its place. */
  readonly component: ComponentType<P>;
  /**
   * Loads its code ahead, once; a failure is let go, the screen asking again as it is shown (by
   * loadScreenCode, which says so or reloads into a newer client: never from a load nobody asked
   * to see).
   */
  readonly preload: () => Promise<void>;
  /** Whether its code has loaded, ahead or as it was shown. */
  readonly isLoaded: () => boolean;
}

/**
 * A screen whose code loads as it is first shown, as lazyScreen's does, or ahead by preload. A lazy
 * component asked for the first time suspends even when its module is already in, so its LaterScreen
 * shows Loading… for a moment; once the code is in, this shows the screen itself, not through lazy.
 */
export function preloadableScreen<P extends object>(
  screen: string,
  load: () => Promise<{ default: ComponentType<P> }>,
  retryAfterMs?: number,
): PreloadableScreen<P> {
  let loaded: ComponentType<P> | undefined = undefined;
  let preloading: Promise<void> | undefined = undefined;
  const keep = (module: { default: ComponentType<P> }): { default: ComponentType<P> } => {
    loaded ??= module.default;
    return module;
  };
  const Lazy = lazy(async () => keep(await loadScreenCode(screen, load, retryAfterMs)));
  function Screen(props: P): JSX.Element {
    // Chosen once per showing: a screen shown through lazy stays so as its code comes, never
    // swapped for the loaded component, which React would mount afresh, its state lost.
    const [Shown] = useState<ComponentType<P>>(() => loaded ?? Lazy);
    return createElement(Shown, props);
  }
  return {
    component: Screen,
    preload: async () => {
      preloading ??= load().then(
        (module) => {
          keep(module);
        },
        () => undefined,
      );
      await preloading;
    },
    isLoaded: () => loaded !== undefined,
  };
}

/**
 * Where a lazily loaded screen is shown: Loading… while its code loads, and, when it did not load,
 * the page saying so, until resetKey (the route) changes. Any other failure goes on as it would.
 */
export function LaterScreen(props: {
  readonly resetKey: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <ScreenCodeBoundary resetKey={props.resetKey}>
      <Suspense fallback={<ScreenLoading />}>{props.children}</Suspense>
    </ScreenCodeBoundary>
  );
}

/** What shows while a screen's code loads. */
function ScreenLoading(): JSX.Element {
  const { t } = useTranslation();
  return <p>{t("common.loading")}</p>;
}

interface BoundaryState {
  readonly failed: ScreenCodeNotLoaded | undefined;
  readonly other: Error | undefined;
  readonly resetKey: string;
}

/** Catches a screen's code that did not load; React offers this only as a class. */
class ScreenCodeBoundary extends Component<
  { readonly resetKey: string; readonly children: ReactNode },
  BoundaryState
> {
  override state: BoundaryState = {
    failed: undefined,
    other: undefined,
    resetKey: this.props.resetKey,
  };

  static getDerivedStateFromError(error: unknown): Partial<BoundaryState> {
    return error instanceof ScreenCodeNotLoaded
      ? { failed: error, other: undefined }
      : { failed: undefined, other: error instanceof Error ? error : new Error(String(error)) };
  }

  static getDerivedStateFromProps(
    props: { readonly resetKey: string },
    state: BoundaryState,
  ): Partial<BoundaryState> | null {
    return props.resetKey === state.resetKey
      ? null
      : { failed: undefined, other: undefined, resetKey: props.resetKey };
  }

  override render(): ReactNode {
    if (this.state.other !== undefined) {
      // Not this boundary's to show: on to whatever handles it above, as without the boundary.
      throw this.state.other;
    }
    const { failed } = this.state;
    if (failed === undefined) {
      return this.props.children;
    }
    const worker = pageWorkerOf(window);
    return (
      <ScreenNotLoadedPage
        text={screenNotLoadedText(
          { failed, worker, page: window.location.pathname },
          navigator.userAgent,
        )}
        worker={worker}
        reload={reloadPage}
      />
    );
  }
}

/**
 * Loads the page again: under a discovery's loader, the loader's own page, which fetches the core's
 * client as it is now (D308).
 */
function reloadPage(): void {
  window.location.reload();
}

function yesNo(is: boolean): string {
  return is ? "yes" : "no";
}

/** What happened, for its person to copy and send (K16): no person named, no secret. */
export function screenNotLoadedText(
  what: {
    readonly failed: ScreenCodeNotLoaded;
    readonly worker: PageWorker;
    readonly page: string;
  },
  userAgent: string,
): string {
  const { failed, worker, page } = what;
  return [
    "This page could not load: the code of its screen did not come.",
    `Screen: ${failed.screen}`,
    `Page: ${page}`,
    `What failed: ${failed.reason}`,
    `Under the discovery's loader: ${yesNo(worker.isUnderLoader)}`,
    `Service worker controlling the page: ${yesNo(worker.isControlled)}`,
    `Browser: ${userAgent}`,
  ].join("\n");
}

/**
 * The page saying a screen could not load, with Reload and what happened to copy. Under a loader a
 * page no worker controls cannot have the core's client's files, which only the worker answers
 * (D308 §6): as with DevTools' "Bypass for network", or a reload that skipped the worker; it says so.
 */
export function ScreenNotLoadedPage(props: {
  readonly text: string;
  readonly worker: PageWorker;
  readonly reload: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const isWorkerMissing = props.worker.isUnderLoader && !props.worker.isControlled;
  return (
    <div className="screen-not-loaded">
      <p role="alert">{t("screenNotLoaded.said")}</p>
      {isWorkerMissing ? <p>{t("screenNotLoaded.noWorker")}</p> : null}
      <button type="button" onClick={props.reload}>
        {t("screenNotLoaded.reload")}
      </button>
      <CopyWhatHappened text={props.text} copy={copyToClipboard} />
    </div>
  );
}
