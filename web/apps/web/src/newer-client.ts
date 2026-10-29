// SPDX-License-Identifier: MPL-2.0

/**
 * A page left open across an update reloads itself, once, when a screen's code it had not loaded
 * yet is no longer served: the newer client's files have other names, and the old page asked for
 * "/assets/title-D5G0YPLU.js" and said "This page could not load" (the owner's Mac, staging,
 * 2026-10-29). A reload brings the newer client, at the same address. It happens only when the cause
 * is a newer client being served, and once per build of the page in a tab, so a reload that did not
 * help shows the page that says so, as does any other cause.
 */

import { reportFailure, type Api } from "@orpheum/sdk";

import { pageUpdateWatch, type UpdateWatch } from "./app-update.ts";
import { tabStorage, type StringStorage } from "./browser-storage.ts";
import { keptClientFiles } from "./client-files.ts";

/** What a page knows of the client the server serves, for a screen whose code did not load. */
export interface NewerClient {
  /** The script of the client this page runs; none for a development page, which never reloads. */
  readonly ownScript: string | undefined;
  /** Whether the server serves a client other than this page's, asked of it now when not yet known. */
  readonly isServed: () => Promise<boolean>;
  /**
   * Whether the client's files the loader last named to the worker (D308) name some files but not
   * path: the client that names it is not the one served there any more.
   */
  readonly isLeftOutByWorker: (path: string) => Promise<boolean>;
  /** The tab's storage, where the build the page last reloaded from is kept. */
  readonly storage: () => StringStorage;
  readonly reload: () => void;
}

/** The tab's key for the build of the client the page last reloaded from for a screen's code. */
const reloadedFromKey = "orpheum.screenCode.reloadedFrom";

/** The tab's key for what the page reloaded for, told to the core's log by the page that follows. */
const reloadNoteKey = "orpheum.screenCode.reloadNote";

/** The client this page asks of, set as the app mounts; none in a test that sets none. */
let pageNewerClient: NewerClient | undefined;

/** Sets what this page asks of the client served, for a screen whose code does not load. */
export function watchForNewerClient(client: NewerClient | undefined): void {
  pageNewerClient = client;
}

/** The newer client a page reaches by api, running ownScript, as the update watch learns it. */
export function newerClientOver(
  api: Api,
  ownScript: string | undefined,
  watch: UpdateWatch = pageUpdateWatch,
): NewerClient {
  return {
    ownScript,
    isServed: async () => {
      if (!watch.isOutdated()) {
        // Every answer names the client served (app-update.ts): the health check is the lightest.
        await api.GET("/health").catch(() => undefined);
      }
      return watch.isOutdated();
    },
    isLeftOutByWorker: async (path) => {
      if (!("caches" in window)) {
        return false;
      }
      try {
        const files = await keptClientFiles(window.caches);
        return files.size > 0 && !files.has(path);
      } catch {
        return false;
      }
    },
    storage: tabStorage,
    reload: () => {
      watch.beforeReload();
      window.location.reload();
    },
  };
}

/** The client's file a failed import names, "assets/title-D5G0YPLU.js", where it names one. */
export function assetNamedBy(reason: string): string | undefined {
  return /\/(assets\/[\w.-]+\.js)\b/u.exec(reason)?.[1];
}

/**
 * Reloads the page for a screen's code that did not load, for reason, when a newer client is served
 * and this tab has not reloaded from this page's build for it yet; whether it did. Never under a
 * loader whose page no worker controls: there the worker, not the client, is missing.
 */
export async function reloadsForNewerClient(
  failed: { readonly screen: string; readonly reason: string },
  worker: { readonly isUnderLoader: boolean; readonly isControlled: boolean },
  client: NewerClient | undefined = pageNewerClient,
): Promise<boolean> {
  const ownScript = client?.ownScript;
  if (client === undefined || ownScript === undefined) {
    return false;
  }
  if (worker.isUnderLoader && !worker.isControlled) {
    return false;
  }
  const storage = client.storage();
  if (storage.getItem(reloadedFromKey) === ownScript) {
    return false;
  }
  const why = await newerClientWhy(client, failed.reason);
  if (why === undefined) {
    return false;
  }
  storage.setItem(reloadedFromKey, ownScript);
  storage.setItem(
    reloadNoteKey,
    `the page reloaded for a newer client, once: the code of the ${failed.screen} screen did not load (${failed.reason}); ${why}; the page ran ${ownScript}`,
  );
  client.reload();
  return true;
}

/** Why the client served is newer than the page's, if it is. */
async function newerClientWhy(client: NewerClient, reason: string): Promise<string | undefined> {
  if (await client.isServed()) {
    return "the server serves another client";
  }
  const asset = assetNamedBy(reason);
  if (asset !== undefined && (await client.isLeftOutByWorker(asset))) {
    return `the client's files the service worker holds no longer name ${asset}`;
  }
  return undefined;
}

/** Tells the core's log, once, what the page before this one reloaded for, if it did. */
export function reportReloadForNewerClient(storage: StringStorage = tabStorage()): void {
  const note = storage.getItem(reloadNoteKey);
  if (note === null) {
    return;
  }
  storage.removeItem(reloadNoteKey);
  reportFailure(note);
}
