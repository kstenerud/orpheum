// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

import { tabStorage, type StringStorage } from "./browser-storage.ts";

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
  /**
   * Notes that the page reloads for the script that made it out of date, so the page that loads
   * next is not out of date for that script again: where a reload cannot bring it, as on a
   * discovery's page, whose client the discovery serves and not the core, the page would otherwise
   * reload forever (2026-10-14).
   */
  readonly beforeReload: () => void;
}

/** The tab's key for the script the page last reloaded for. */
const reloadedForKey = "orpheum.reloadedForClient";

/** A watch of whether a page is out of date; storage is the tab's, where it keeps what it reloaded for. */
export function createUpdateWatch(storage: () => StringStorage = tabStorage): UpdateWatch {
  let isOutdated = false;
  let outdatedBy: string | undefined;
  const listeners = new Set<() => void>();
  const noteServed = (ownScript: string | undefined, served: string | null): void => {
    if (
      ownScript === undefined ||
      served === null ||
      served === ownScript ||
      isOutdated ||
      storage().getItem(reloadedForKey) === served
    ) {
      return;
    }
    isOutdated = true;
    outdatedBy = served;
    for (const listener of listeners) {
      listener();
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
    beforeReload: () => {
      if (outdatedBy !== undefined) {
        storage().setItem(reloadedForKey, outdatedBy);
      }
    },
  };
}

/** This page's watch. */
export const pageUpdateWatch = createUpdateWatch();
