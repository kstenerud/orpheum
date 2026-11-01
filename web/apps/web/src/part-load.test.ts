// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi, type Mock } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { loadOwnPart, loadPart, partRetryMs, type PartLoadParts } from "./part-load.ts";
import { takeReloadNote } from "./reload-note.ts";

function tabOf(kept: Map<string, string>): StringStorage {
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => kept.set(key, value),
    removeItem: (key) => kept.delete(key),
  };
}

function pageOf(tab: StringStorage): PartLoadParts & {
  readonly reload: Mock<() => void>;
  readonly waited: number[];
} {
  const waited: number[] = [];
  return {
    tab,
    reload: vi.fn<() => void>(),
    waited,
    wait: async (ms) => {
      waited.push(ms);
      await Promise.resolve();
    },
  };
}

/** What the owner's Firefox met on its first sign-in at staging (2026-11-01). */
const preloadFailed = new Error("Unable to preload CSS for /assets/busy-group-CtCniuzz.css");

/** Lets loadPart's awaits run, it never settling once it reloads. */
async function settled(): Promise<void> {
  for (let turn = 0; turn < 10; turn += 1) {
    await Promise.resolve();
  }
}

describe("loadPart", () => {
  it("loads the part once when it comes, reloading nothing", async () => {
    const page = pageOf(tabOf(new Map()));
    const load = vi.fn(async () => await Promise.resolve({}));

    await loadPart("client", load, page);

    expect(load).toHaveBeenCalledTimes(1);
    expect(page.reload).not.toHaveBeenCalled();
  });

  it("asks again once, after a pause, when the part's first load fails", async () => {
    const page = pageOf(tabOf(new Map()));
    const load = vi.fn<() => Promise<unknown>>().mockRejectedValueOnce(preloadFailed);
    load.mockResolvedValue({});

    await loadPart("client", load, page);

    expect(load).toHaveBeenCalledTimes(2);
    expect(page.waited).toEqual([partRetryMs]);
    expect(page.reload).not.toHaveBeenCalled();
  });

  it("reloads the page, once in the tab, when the part fails twice, leaving why for the next page", async () => {
    const tab = tabOf(new Map());
    const page = pageOf(tab);
    const load = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);

    void loadPart("client", load, page);
    await settled();

    expect(page.reload).toHaveBeenCalledTimes(1);
    expect(takeReloadNote(tab)).toBe(
      `the page reloaded, once: the code of its client did not load, asked twice (${preloadFailed.message})`,
    );
  });

  it("throws, reloading no more, when the page it reloaded into fails twice too", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);
    void loadPart("client", failing, pageOf(tab));
    await settled();

    const reloaded = pageOf(tab);
    await expect(loadPart("client", failing, reloaded)).rejects.toBe(preloadFailed);
    expect(reloaded.reload).not.toHaveBeenCalled();
  });

  it("reloads again later in the tab once a load came in between", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);
    void loadPart("client", failing, pageOf(tab));
    await settled();
    await loadPart("client", async () => await Promise.resolve({}), pageOf(tab));

    const later = pageOf(tab);
    void loadPart("client", failing, later);
    await settled();

    expect(later.reload).toHaveBeenCalledTimes(1);
  });
});

/** A page whose head names a discovery's loader configuration, as a discovery's page does (D308). */
function discoveryPage(): Document {
  const page = document.implementation.createHTMLDocument("loader");
  for (const [name, content] of Object.entries({
    "orpheum-discovery-client-api": "https://api.example/client/v1",
    "orpheum-client-origins": "https://{core}.client.example",
    "orpheum-loader-origin": "https://app.example",
  })) {
    const meta = page.createElement("meta");
    meta.name = name;
    meta.content = content;
    page.head.append(meta);
  }
  return page;
}

describe("loadOwnPart", () => {
  it("loads the loader on a discovery's page, and the client on any other", async () => {
    const loader = vi.fn(async () => await Promise.resolve({}));
    const client = vi.fn(async () => await Promise.resolve({}));

    await loadOwnPart(
      { window: {}, document: discoveryPage() },
      { loader, client },
      pageOf(tabOf(new Map())),
    );
    expect(loader).toHaveBeenCalledTimes(1);
    expect(client).not.toHaveBeenCalled();

    const corePage = document.implementation.createHTMLDocument("client");
    await loadOwnPart(
      { window: {}, document: corePage },
      { loader, client },
      pageOf(tabOf(new Map())),
    );
    expect(client).toHaveBeenCalledTimes(1);
  });

  it("reloads a client's page whose code does not come, asked twice, never leaving it black", async () => {
    const page = pageOf(tabOf(new Map()));
    const corePage = document.implementation.createHTMLDocument("client");

    void loadOwnPart(
      { window: {}, document: corePage },
      {
        loader: async () => await Promise.resolve({}),
        client: async () => await Promise.reject(preloadFailed),
      },
      page,
    );
    await settled();

    expect(page.reload).toHaveBeenCalledTimes(1);
  });
});
