// SPDX-License-Identifier: MPL-2.0

/**
 * Whether a live service worker controls the page, which carryMediaOver (rtc-media.ts) waits for
 * before it says the page carries its media: a page no worker controls loads past every worker, so
 * what it loads by itself goes to its origin.
 */

/**
 * How long a page waits, its worker ready and asked to claim it, for a worker to control it: one that
 * claims does so at once, so this is a bound for one that never will, not a wait anyone sits through.
 */
export const controlWaitMs = 5000;

/**
 * The page is not controlled by a live worker, ready and asked to claim it, after controlWaitMs: its
 * loads go past every worker, so the media it loads by itself goes to its origin. Chrome can commit a
 * page reloaded as an updated worker activates under the worker before and then stop that one,
 * leaving the page uncontrolled (idiosyncrasies.md).
 */
export class PageNotControlled extends Error {
  constructor() {
    super(
      `no service worker controls the page ${String(controlWaitMs)} ms after it was asked to, so its media cannot go over its path`,
    );
    this.name = "PageNotControlled";
  }
}

/**
 * The browser runs no service worker for the page, so the media it loads by itself cannot go over its
 * path, and the page cannot play: a private window of Firefox, which has no navigator.serviceWorker by
 * design (Mozilla bug 1320796), or a browser that refuses the worker's registration. Such a page was
 * told its network could not reach the server (idiosyncrasies.md, "Firefox runs no service worker in
 * a private window").
 */
export class NoServiceWorker extends Error {
  constructor(cause?: unknown) {
    super(
      cause === undefined
        ? "this browser runs no service worker, so the page cannot carry its media"
        : "this browser refused the page's service worker, so the page cannot carry its media",
      cause === undefined ? undefined : { cause },
    );
    this.name = "NoServiceWorker";
  }
}

/** Whether a live worker controls the page: none, or one the browser stopped for good, does not. */
export function isControlled(workers: Pick<ServiceWorkerContainer, "controller">): boolean {
  return workers.controller !== null && workers.controller.state !== "redundant";
}

/** Whether a live worker controls the page now, or does within waitMs. */
export async function controlledWithin(
  workers: Pick<ServiceWorkerContainer, "controller" | "addEventListener" | "removeEventListener">,
  waitMs: number,
): Promise<boolean> {
  if (isControlled(workers)) {
    return true;
  }
  return await new Promise<boolean>((resolve) => {
    const changed = (): void => {
      if (isControlled(workers)) {
        settle();
      }
    };
    const timer = setTimeout(() => {
      settle();
    }, waitMs);
    function settle(): void {
      clearTimeout(timer);
      workers.removeEventListener("controllerchange", changed);
      resolve(isControlled(workers));
    }
    workers.addEventListener("controllerchange", changed);
  });
}

/**
 * The address a page asks to learn whether its worker answers its loads: the worker answers it
 * itself, and nothing else does, so an answer from the network, or none, is a worker bypassed, as
 * DevTools' "Bypass for network" does (idiosyncrasies.md). Posters and films then 404 at a client
 * origin, saying nothing (agent-failures A51).
 */
export const workerProbePath = "/orpheum-worker-probe";

/** What the worker answers the probe with, which no server says. */
export const workerProbeAnswer = "answered by Orpheum's service worker";

/** Whether a load of url, made at origin, is the probe, which the worker answers itself. */
export function isWorkerProbe(url: URL, origin: string): boolean {
  return url.origin === origin && url.pathname === workerProbePath;
}

/** The worker's answer to the probe. */
export function workerProbeResponse(): Response {
  return new Response(workerProbeAnswer, {
    status: 200,
    headers: { "Content-Type": "text/plain", "Cache-Control": "no-store" },
  });
}

/**
 * How long a page waits for the probe's answer before it takes the silence for a worker bypassed:
 * the worker answers at once, from memory, so this bounds only a load sent to a network that
 * answers nothing. A justified constant.
 */
export const workerProbeWaitMs = 5000;

/**
 * Whether the worker controlling the page is bypassed: the probe, loaded by load, answered by
 * anything but the worker, or by nothing within waitMs. Asked only of a page a worker controls: one
 * none controls has no worker to bypass.
 */
export async function isWorkerBypassed(
  load: (path: string, init: RequestInit) => Promise<Response>,
  waitMs: number = workerProbeWaitMs,
): Promise<boolean> {
  const abort = new AbortController();
  const timer = setTimeout(() => {
    abort.abort();
  }, waitMs);
  try {
    const answer = await Promise.race([
      load(workerProbePath, { cache: "no-store", signal: abort.signal }),
      new Promise<undefined>((resolve) => {
        abort.signal.addEventListener("abort", () => {
          resolve(undefined);
        });
      }),
    ]);
    return answer === undefined || !answer.ok || (await answer.text()) !== workerProbeAnswer;
  } catch {
    return true;
  } finally {
    clearTimeout(timer);
  }
}

/** isWorkerBypassed, the probe loaded by the page's own fetch, which its worker sees first. */
export async function isPageWorkerBypassed(): Promise<boolean> {
  return await isWorkerBypassed(async (path, init) => await fetch(path, init));
}
