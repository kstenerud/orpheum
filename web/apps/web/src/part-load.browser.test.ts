// SPDX-License-Identifier: MPL-2.0

/**
 * A role's code that does not come across a deploy, in Chromium and Firefox, under a worker that
 * refuses its module a few times and then serves it (test-refusing-worker.ts). The page reloaded for
 * it threw, and the page stayed black for good (the owner's Firefox at staging's client origin, the
 * 11-26 batch's item 1): Chromium asks for a module that failed no more in the same document, and
 * Firefox's asks were spent before the module came. Each page here is a document of its own, as a
 * reload makes one.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { exportingFunction, loadRole, type PartLoadParts } from "./part-load.ts";
import { refusedRolePath } from "./test-refusing-worker.ts";

/** The pages and workers a test set up, taken down after it. */
const made: { frames: HTMLIFrameElement[]; registrations: ServiceWorkerRegistration[] } = {
  frames: [],
  registrations: [],
};

afterEach(async () => {
  for (const frame of made.frames.splice(0)) {
    frame.remove();
  }
  await Promise.all(made.registrations.splice(0).map((registration) => registration.unregister()));
});

/** The worker's script, refusing the role's module refusals times. */
function refusingWorker(refusals: number): string {
  return `/apps/web/src/test-refusing-worker.ts?refusals=${String(refusals)}`;
}

/** script registered as a module service worker over scope, once it is active. */
async function activeWorker(script: string, scope: string): Promise<void> {
  const registration = await navigator.serviceWorker.register(script, { type: "module", scope });
  made.registrations.push(registration);
  const worker = registration.installing ?? registration.waiting ?? registration.active;
  if (worker === null) {
    throw new Error(`${script} did not install`);
  }
  await new Promise<void>((resolve, reject) => {
    const settled = (): void => {
      if (worker.state === "activated") {
        resolve();
      } else if (worker.state === "redundant") {
        reject(new Error(`${script} failed to install`));
      }
    };
    worker.addEventListener("statechange", settled);
    settled();
  });
}

/** A page's import of the module at an address, made in the page's own document. */
type PageImport = (address: string) => Promise<unknown>;

function isPageImport(found: unknown): found is PageImport {
  return typeof found === "function";
}

/** A new page at scope, under its worker: a document of its own, whose import the test makes. */
async function pageAt(scope: string): Promise<PageImport> {
  const frame = document.createElement("iframe");
  made.frames.push(frame);
  const loaded = new Promise((resolve) => {
    frame.addEventListener("load", resolve, { once: true });
  });
  frame.src = scope;
  document.body.append(frame);
  await loaded;
  const page = frame.contentWindow;
  if (page?.navigator.serviceWorker.controller === null) {
    throw new Error("the page under the worker's scope is not controlled by it");
  }
  await vi.waitFor(() => {
    expect(isPageImport(Reflect.get(page ?? {}, "importAt"))).toBe(true);
  });
  const found: unknown = Reflect.get(page ?? {}, "importAt");
  if (!isPageImport(found)) {
    throw new Error("the page has no import");
  }
  return found;
}

/** The tab both pages share, which keeps the mark of the reload across it. */
function tabOf(kept: Map<string, string>): StringStorage {
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => kept.set(key, value),
    removeItem: (key) => kept.delete(key),
  };
}

/** What loadRole is given in a page: its import, no real pause, and a reload the test counts. */
function partsOf(
  tab: StringStorage,
  importAt: PageImport,
): PartLoadParts & { readonly reloads: number[] } {
  const reloads: number[] = [];
  return {
    tab,
    reload: () => {
      reloads.push(1);
    },
    reloads,
    wait: async () => {
      await Promise.resolve();
    },
    importAt,
  };
}

/** The role's module, as the page uses it. */
interface RoleModule {
  readonly startTheRole: () => string;
}

/** Says a module is the role's, as loader-main.tsx does. */
const isRole = exportingFunction<RoleModule>("startTheRole");

/** The page's import of the role's module at address, as loader-main.tsx imports it. */
function roleImport(pageImport: PageImport, address: string): () => Promise<RoleModule> {
  return async () => {
    const loaded = await pageImport(address);
    if (!isRole(loaded)) {
      throw new Error(`${address} is not the role's module`);
    }
    return loaded;
  };
}

describe("loadRole across a deploy, under a worker", () => {
  it("loads the role in the page reloaded for it, though its first asks there are refused too, reloading once", async () => {
    const scope = "/apps/web/src/part-load.ts?page=refused-role";
    // Two asks refused in the first page, two in the page reloaded for it; then it comes.
    await activeWorker(refusingWorker(4), scope);
    const tab = tabOf(new Map());
    const role = `${window.location.origin}${refusedRolePath}`;

    const firstImport = await pageAt(scope);
    const first = partsOf(tab, firstImport);
    void loadRole("loader's role", { load: roleImport(firstImport, role), isRole }, first);
    await vi.waitFor(() => {
      expect(first.reloads).toHaveLength(1);
    });

    const reloadedImport = await pageAt(scope);
    const reloaded = partsOf(tab, reloadedImport);
    const loaded = await loadRole(
      "loader's role",
      { load: roleImport(reloadedImport, role), isRole },
      reloaded,
    );

    expect(loaded.startTheRole()).toBe("the role");
    expect(reloaded.reloads).toHaveLength(0);
  });
});
