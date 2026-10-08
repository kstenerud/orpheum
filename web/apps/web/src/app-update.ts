// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

/** A watcher of every request and answer the API makes. */
type Middleware = Parameters<Api["use"]>[0];

/** The header each answer names the server's web client's script in (api.ClientScriptHeader). */
export const clientScriptHeader = "X-Orpheum-Client";

/** The built client's own script on page, "index-DCLi77_b.js"; undefined for a development page. */
export function ownScriptOf(page: Pick<Document, "querySelector">): string | undefined {
  const entry = page
    .querySelector('script[type="module"][src^="/assets/index-"]')
    ?.getAttribute("src");
  return /^\/assets\/(index-[A-Za-z0-9_-]+\.js)$/u.exec(entry ?? "")?.[1];
}

/** Whether this page is out of date, learned from the answers it watches, told to listeners. */
export interface UpdateWatch {
  readonly isOutdated: () => boolean;
  readonly subscribe: (listener: () => void) => () => void;
  /** Takes the client an answer named, served, null for none, against the page's own, ownScript. */
  readonly noteServed: (ownScript: string | undefined, served: string | null) => void;
  /**
   * Watches every answer for the server's client being other than this page's own, ownScript: a
   * tab left open across an update kept running a bug fixed under it (the owner, 2026-10-08:
   * "Maybe the client should send its version and then force a reload if there's a mismatch?"). A
   * development page, with no built script, is never out of date.
   */
  readonly middleware: (ownScript: string | undefined) => Middleware;
}

/** A watch of whether a page is out of date. */
export function createUpdateWatch(): UpdateWatch {
  let isOutdated = false;
  const listeners = new Set<() => void>();
  const noteServed = (ownScript: string | undefined, served: string | null): void => {
    if (ownScript !== undefined && served !== null && served !== ownScript && !isOutdated) {
      isOutdated = true;
      for (const listener of listeners) {
        listener();
      }
    }
  };
  return {
    isOutdated: () => isOutdated,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    noteServed,
    middleware: (ownScript) => ({
      onResponse({ response }) {
        noteServed(ownScript, response.headers.get(clientScriptHeader));
        return undefined;
      },
    }),
  };
}

/** This page's watch. */
export const pageUpdateWatch = createUpdateWatch();
