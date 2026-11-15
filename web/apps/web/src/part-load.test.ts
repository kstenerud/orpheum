// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi, type Mock } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import {
  exportingFunction,
  loadOwnPart,
  loadPart,
  loadRole,
  moduleAddressOf,
  partAskCapMs,
  partNoteAfterMs,
  partRetryMs,
  shellLoadingText,
  shellNote,
  type PartLoadParts,
  type WaitingNote,
} from "./part-load.ts";
import { en } from "./locales/en.ts";
import { takeReloadNote } from "./reload-note.ts";

function tabOf(kept: Map<string, string>): StringStorage {
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => kept.set(key, value),
    removeItem: (key) => kept.delete(key),
  };
}

function pageOf(
  tab: StringStorage,
  importAt: Mock<(address: string) => Promise<unknown>> = vi
    .fn<(address: string) => Promise<unknown>>()
    .mockRejectedValue(new TypeError("no module is asked for by its address here")),
): PartLoadParts & {
  readonly reload: Mock<() => void>;
  readonly waited: number[];
  readonly importAt: Mock<(address: string) => Promise<unknown>>;
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
    importAt,
    note: quiet,
  };
}

/** What the owner's Firefox met on its first sign-in at staging (2026-11-01). */
const preloadFailed = new Error("Unable to preload CSS for /assets/busy-group-CtCniuzz.css");

/** The role's module, as at a staging deploy's black page (the 11-26 batch's item 1). */
const roleAddress = "https://core.client.example/assets/loader-at-client-origin-D4tYUb8s.js";

/** Chromium's failure of an import of the role's module, which it never asks for again. */
const roleNotFetched = new TypeError(`Failed to fetch dynamically imported module: ${roleAddress}`);

/** The role's module, as the loader's page uses it. */
const roleModule = { startAtClientOrigin: () => "the role" };

/** Says a module is the role's, as loader-main.tsx does. */
const isRoleModule = exportingFunction<typeof roleModule>("startAtClientOrigin");

/** The note a page shows while it waits, which a test that does not look at it gives. */
const quiet: WaitingNote = { show: () => () => undefined };

/** A page's note while it waits for its role's code: how often it was shown, and whether it shows. */
function noteOf(): WaitingNote & { readonly seen: { shown: number; isShowing: boolean } } {
  const seen = { shown: 0, isShowing: false };
  return {
    seen,
    show: () => {
      seen.shown += 1;
      seen.isShowing = true;
      return () => {
        seen.isShowing = false;
      };
    },
  };
}

/** A role's code that comes when the test says. */
function comingRole(): {
  readonly load: () => Promise<typeof roleModule>;
  readonly come: () => void;
} {
  let come: () => void = () => undefined;
  const coming = new Promise<typeof roleModule>((resolve) => {
    come = () => {
      resolve(roleModule);
    };
  });
  return {
    load: async () => await coming,
    come: () => {
      come();
    },
  };
}

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

  it("asks on in the page it reloaded into, each pause longer, reloading no more, until the part comes", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);
    void loadPart("client", failing, pageOf(tab));
    await settled();

    const reloaded = pageOf(tab);
    const part = { the: "client" };
    const later = vi.fn<() => Promise<unknown>>().mockRejectedValueOnce(preloadFailed);
    later.mockRejectedValueOnce(preloadFailed).mockRejectedValueOnce(preloadFailed);
    later.mockRejectedValueOnce(preloadFailed).mockResolvedValue(part);
    await expect(loadPart("client", later, reloaded)).resolves.toBe(part);
    expect(reloaded.reload).not.toHaveBeenCalled();
    expect(later).toHaveBeenCalledTimes(5);
    expect(reloaded.waited).toEqual([
      partRetryMs,
      2 * partRetryMs,
      4 * partRetryMs,
      8 * partRetryMs,
    ]);
  });

  it("asks no less often than each partAskCapMs, in the page it reloaded into", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);
    void loadPart("client", failing, pageOf(tab));
    await settled();

    const reloaded = pageOf(tab);
    const later = vi.fn<() => Promise<unknown>>();
    for (let ask = 0; ask < 9; ask += 1) {
      later.mockRejectedValueOnce(preloadFailed);
    }
    later.mockResolvedValue({});
    await loadPart("client", later, reloaded);

    expect(reloaded.reload).not.toHaveBeenCalled();
    expect(Math.max(...reloaded.waited)).toBe(partAskCapMs);
    expect(reloaded.waited.slice(-2)).toEqual([partAskCapMs, partAskCapMs]);
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

describe("loadRole (web architecture audit, 2026-11-10)", () => {
  it("hands back the role's code, asked again once after a pause", async () => {
    const page = pageOf(tabOf(new Map()));
    const role = { startAtLoaderOrigin: "the role" };
    const load = vi.fn<() => Promise<typeof role>>().mockRejectedValueOnce(preloadFailed);
    load.mockResolvedValue(role);

    await expect(
      loadRole(
        "loader's role",
        { load, isRole: exportingFunction<typeof role>("startAtLoaderOrigin") },
        { ...page, note: quiet },
      ),
    ).resolves.toBe(role);
    expect(page.waited).toEqual([partRetryMs]);
  });

  it("asks on, reloading no more, in the page reloaded for it, though the loader's part loaded", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<unknown>>().mockRejectedValue(preloadFailed);
    const first = pageOf(tab);
    void loadRole(
      "loader's role",
      { load: failing, isRole: isRoleModule },
      { ...first, note: quiet },
    );
    await settled();
    expect(first.reload).toHaveBeenCalledTimes(1);

    // The page reloaded for it: the loader's part loads, then its role fails again, and comes later.
    const reloaded = pageOf(tab);
    await loadPart("loader", async () => await Promise.resolve({}), reloaded);
    const later = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValueOnce(preloadFailed);
    later.mockRejectedValueOnce(preloadFailed).mockResolvedValue(roleModule);
    await expect(
      loadRole(
        "loader's role",
        { load: later, isRole: isRoleModule },
        { ...reloaded, note: quiet },
      ),
    ).resolves.toBe(roleModule);
    expect(reloaded.reload).not.toHaveBeenCalled();
  });

  it("asks for a module that failed under an address of its own, as Chromium asks for no failed one again", async () => {
    const importAt = vi.fn<(address: string) => Promise<unknown>>().mockResolvedValue(roleModule);
    const page = pageOf(tabOf(new Map()), importAt);
    const load = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValue(roleNotFetched);

    await expect(
      loadRole("loader's role", { load, isRole: isRoleModule }, { ...page, note: quiet }),
    ).resolves.toBe(roleModule);
    expect(load).toHaveBeenCalledTimes(1);
    expect(importAt).toHaveBeenCalledWith(`${roleAddress}?asked=2`);
    expect(page.reload).not.toHaveBeenCalled();
  });

  it("asks on under a new address each time in the page reloaded for it, taking no other module for the role", async () => {
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValue(roleNotFetched);
    void loadRole(
      "loader's role",
      { load: failing, isRole: isRoleModule },
      { ...pageOf(tab), note: quiet },
    );
    await settled();

    const importAt = vi
      .fn<(address: string) => Promise<unknown>>()
      .mockRejectedValueOnce(
        new TypeError(`Failed to fetch dynamically imported module: ${roleAddress}?asked=2`),
      )
      .mockResolvedValueOnce({ somethingElse: "not the role" })
      .mockResolvedValue(roleModule);
    const reloaded = pageOf(tab, importAt);
    await expect(
      loadRole(
        "loader's role",
        { load: failing, isRole: isRoleModule },
        { ...reloaded, note: quiet },
      ),
    ).resolves.toBe(roleModule);
    expect(importAt.mock.calls.map(([address]) => address)).toEqual([
      `${roleAddress}?asked=2`,
      `${roleAddress}?asked=3`,
      `${roleAddress}?asked=4`,
    ]);
    expect(reloaded.reload).not.toHaveBeenCalled();
  });
});

describe("loadRole's note while the page waits (the owner's \"show loading\", 2026-11-15)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows the note in the page reloaded for the role's code once it has waited partNoteAfterMs, and takes it away as the code comes", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const tab = tabOf(new Map());
    const failing = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValue(preloadFailed);
    void loadRole(
      "loader's role",
      { load: failing, isRole: isRoleModule },
      { ...pageOf(tab), note: quiet },
    );
    await settled();

    // The page reloaded for it: two asks fail, and the third waits until the code comes.
    const note = noteOf();
    const role = comingRole();
    const later = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValueOnce(preloadFailed);
    later.mockRejectedValueOnce(preloadFailed).mockImplementation(role.load);
    const waited = loadRole(
      "loader's role",
      { load: later, isRole: isRoleModule },
      { ...pageOf(tab), note },
    );
    await settled();
    await settled();
    expect(later).toHaveBeenCalledTimes(3);
    vi.advanceTimersByTime(partNoteAfterMs - 1);
    expect(note.seen.shown).toBe(0);
    vi.advanceTimersByTime(1);
    expect(note.seen).toEqual({ shown: 1, isShowing: true });

    role.come();
    await expect(waited).resolves.toBe(roleModule);
    expect(note.seen).toEqual({ shown: 1, isShowing: false });
  });

  it("shows the note while the first page's ask is slow, before any reload", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const note = noteOf();
    const role = comingRole();
    const page = pageOf(tabOf(new Map()));
    const waited = loadRole(
      "loader's role",
      { load: role.load, isRole: isRoleModule },
      { ...page, note },
    );
    vi.advanceTimersByTime(partNoteAfterMs);
    expect(note.seen.isShowing).toBe(true);

    role.come();
    await expect(waited).resolves.toBe(roleModule);
    expect(note.seen.isShowing).toBe(false);
    expect(page.reload).not.toHaveBeenCalled();
  });

  it("never shows the note when the role's code comes asked again once, before partNoteAfterMs", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const note = noteOf();
    const load = vi.fn<() => Promise<typeof roleModule>>().mockRejectedValueOnce(preloadFailed);
    load.mockResolvedValue(roleModule);

    await loadRole(
      "loader's role",
      { load, isRole: isRoleModule },
      { ...pageOf(tabOf(new Map())), note },
    );
    vi.advanceTimersByTime(10 * partNoteAfterMs);

    expect(note.seen.shown).toBe(0);
  });
});

describe("moduleAddressOf", () => {
  it("reads the module's address from Chromium's failure and Firefox's, without its query", () => {
    expect(moduleAddressOf(roleNotFetched)).toBe(roleAddress);
    expect(
      moduleAddressOf(
        new TypeError(`error loading dynamically imported module: ${roleAddress}?asked=3`),
      ),
    ).toBe(roleAddress);
    expect(moduleAddressOf(preloadFailed)).toBeUndefined();
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

describe("the shell's note while it waits for its part's code (the owner's \"Untranslated is fine\", 2026-11-15)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  /** A core's page as index.html is: its root holding the note for agents. */
  function shellPage(): Document {
    const page = document.implementation.createHTMLDocument("client");
    const root = page.createElement("div");
    root.id = "root";
    const forAgents = page.createElement("p");
    forAgents.className = "for-agents";
    forAgents.textContent =
      "An AI agent setting up Orpheum finds its setup help at /agent-setup.md.";
    root.append(forAgents);
    page.body.append(root);
    return page;
  }

  it("is common.loading's English, as the shell loads no translation", () => {
    expect(shellLoadingText).toBe(en.common.loading);
  });

  it("shows Loading… alone in the root once the client's code has been waited for partNoteAfterMs, and takes it away as the code comes", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const page = shellPage();
    const client = comingRole();
    const waited = loadOwnPart(
      { window: {}, document: page },
      { loader: async () => await Promise.resolve({}), client: client.load },
      { ...pageOf(tabOf(new Map())), note: shellNote(page) },
    );
    const root = page.getElementById("root");
    vi.advanceTimersByTime(partNoteAfterMs - 1);
    expect(root?.textContent).not.toContain(shellLoadingText);
    vi.advanceTimersByTime(1);
    expect([...(root?.children ?? [])].map((shown) => [shown.tagName, shown.textContent])).toEqual([
      ["P", shellLoadingText],
    ]);

    client.come();
    await waited;
    expect(root?.textContent).not.toContain(shellLoadingText);
  });

  it("never shows it when the client's code comes at once", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const page = shellPage();
    await loadOwnPart(
      { window: {}, document: page },
      {
        loader: async () => await Promise.resolve({}),
        client: async () => await Promise.resolve({}),
      },
      { ...pageOf(tabOf(new Map())), note: shellNote(page) },
    );
    vi.advanceTimersByTime(10 * partNoteAfterMs);
    expect(page.getElementById("root")?.textContent).not.toContain(shellLoadingText);
  });
});
