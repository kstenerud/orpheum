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
  lazy,
  Suspense,
  type ComponentType,
  type JSX,
  type LazyExoticComponent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { loaderOf } from "./loader-interface.ts";
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
 * Loads a screen's code, asking again once after retryAfterMs when it fails; failing again, it tells
 * the core's log which screen, why and whether a worker controlled the page, and throws
 * ScreenCodeNotLoaded for LaterScreen to show.
 */
export async function loadScreenCode<T>(
  screen: string,
  load: () => Promise<T>,
  retryAfterMs: number = screenRetryMs,
): Promise<T> {
  try {
    return await load();
  } catch {
    await new Promise((resolve) => {
      setTimeout(resolve, retryAfterMs);
    });
  }
  try {
    return await load();
  } catch (cause: unknown) {
    const failed = new ScreenCodeNotLoaded(screen, cause);
    const worker = pageWorkerOf(window);
    reportFailure(
      new Error(
        `${failed.message} (under a loader: ${yesNo(worker.isUnderLoader)}, a service worker controls the page: ${yesNo(worker.isControlled)})`,
      ),
    );
    throw failed;
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
