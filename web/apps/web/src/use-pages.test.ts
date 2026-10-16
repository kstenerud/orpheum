// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Ask, AskOutcome, Poller } from "./polling.ts";
import { mount } from "./test-dom.tsx";
import { appendPage, loadAgain, merged, usePages, type Page, type Pages } from "./use-pages.ts";

/**
 * A poller a test runs by hand: each ask a hook starts is kept, and the test runs it and reads what
 * it answered; polling.test.ts holds when backgroundChangesPoller runs them (D237).
 */
function handPoller(): Poller & { readonly asks: Ask[] } {
  const asks: Ask[] = [];
  return {
    asks,
    start: (ask) => {
      asks.push(ask);
      return () => undefined;
    },
  };
}

/** A server's list, two to a page, read by the index after the cursor. */
function pagedBy2(titles: () => readonly string[]): (cursor: string) => Promise<Page<string>> {
  return (cursor) => {
    const from = cursor === "" ? 0 : Number(cursor);
    const all = titles();
    const next = from + 2 < all.length ? String(from + 2) : "";
    return Promise.resolve({ items: all.slice(from, from + 2), nextCursor: next });
  };
}

async function pollOnce(poller: { readonly asks: readonly Ask[] }): Promise<AskOutcome> {
  const ask = poller.asks.at(-1);
  if (ask === undefined) {
    throw new Error("nothing is polling");
  }
  let cameTo: AskOutcome | undefined;
  await act(async () => {
    ask((outcome) => {
      cameTo = outcome;
    });
    for (let turn = 0; turn < 5; turn += 1) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  });
  if (cameTo === undefined) {
    throw new Error("the ask never answered");
  }
  return cameTo;
}

describe("usePages", () => {
  it("reads again every page a person scrolled through, from the start", async () => {
    const asked: string[] = [];
    const load = pagedBy2(() => ["A", "B", "C", "D", "E"]);
    const again = await loadAgain((cursor) => {
      asked.push(cursor);
      return load(cursor);
    }, 3);
    expect(merged(again)).toEqual({ items: ["A", "B", "C", "D"], nextCursor: "4" });
    expect(asked).toEqual(["", "2"]);
  });

  it("shows what a scan adds without a reload, and says when nothing changed (D237)", async () => {
    const poller = handPoller();
    let titles = ["Wing", "Greed"];
    const load = pagedBy2(() => titles);
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Wing", "Greed"], nextCursor: "" },
    });
    expect(await pollOnce(poller)).toBe("unchanged");
    titles = ["Nosferatu", "Wing", "Greed"];
    expect(await pollOnce(poller)).toBe("changed");
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Nosferatu", "Wing"], nextCursor: "2" },
    });
    mounted.unmount();
  });
  it("reads a list whose first read failed again on the next poll, and shows it (2026-10-06)", async () => {
    const poller = handPoller();
    let isServerUp = false;
    const titles = pagedBy2(() => ["Wing", "Greed"]);
    const load = (cursor: string): Promise<Page<string>> =>
      isServerUp ? titles(cursor) : Promise.reject(new TypeError("Failed to fetch"));
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const mounted = await mount(createElement(Probe));
    expect(pages?.loadedSoFar).toMatchObject({ state: "failed", willAskAgain: true });
    expect(await pollOnce(poller)).toBe("failed");
    isServerUp = true;
    expect(await pollOnce(poller)).toBe("changed");
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Wing", "Greed"], nextCursor: "" },
    });
    mounted.unmount();
    written.mockRestore();
  });

  it("shows again a list whose page more failed, though the poll reads what it showed before", async () => {
    const poller = handPoller();
    let isServerUp = true;
    const titles = pagedBy2(() => ["Wing", "Greed", "The Gold Rush"]);
    const load = (cursor: string): Promise<Page<string>> =>
      isServerUp ? titles(cursor) : Promise.reject(new TypeError("Failed to fetch"));
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const mounted = await mount(createElement(Probe));
    isServerUp = false;
    await act(async () => {
      pages?.loadMore();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(pages?.loadedSoFar.state).toBe("failed");
    isServerUp = true;
    expect(await pollOnce(poller)).toBe("changed");
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Wing", "Greed"], nextCursor: "2" },
    });
    mounted.unmount();
    written.mockRestore();
  });
});

describe("a poll of a list scrolled far (D306 §7)", () => {
  it("reads only the pages in view, each by its own cursor, and keeps the rest as they were", async () => {
    const poller = handPoller();
    let titles = ["A", "B", "C", "D", "E", "F"];
    const asked: string[] = [];
    const titlesBy2 = pagedBy2(() => titles);
    const load = (cursor: string): Promise<Page<string>> => {
      asked.push(cursor);
      return titlesBy2(cursor);
    };
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    for (let more = 0; more < 2; more += 1) {
      await act(async () => {
        pages?.loadMore();
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["A", "B", "C", "D", "E", "F"], nextCursor: "" },
    });
    // In view: E and F, the third page. A title added first is not read: its page is not in view.
    pages?.showRange(4, 5);
    titles = ["A0", "A", "B", "C", "D", "E", "F2"];
    asked.length = 0;
    expect(await pollOnce(poller)).toBe("changed");
    expect(asked).toEqual(["4"]);
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["A", "B", "C", "D", "D", "E"], nextCursor: "6" },
    });
    // Back at the top, the first page is read again, and shows what was added there.
    pages?.showRange(0, 1);
    asked.length = 0;
    expect(await pollOnce(poller)).toBe("changed");
    expect(asked).toEqual([""]);
    expect(pages?.loadedSoFar).toMatchObject({ value: { items: ["A0", "A", "C", "D", "D", "E"] } });
    mounted.unmount();
  });

  it("shows an item read twice once, given its id, and keeps an unchanged item the one shown", async () => {
    const poller = handPoller();
    let titles = [
      { id: 1, name: "Wing" },
      { id: 2, name: "Greed" },
      { id: 3, name: "The Gold Rush" },
    ];
    const load = (cursor: string): Promise<Page<{ id: number; name: string }>> => {
      const from = cursor === "" ? 0 : Number(cursor);
      return Promise.resolve({
        items: titles.slice(from, from + 2),
        nextCursor: from + 2 < titles.length ? String(from + 2) : "",
      });
    };
    let pages: Pages<{ id: number; name: string }> | undefined;
    function Probe(): null {
      pages = usePages(load, poller, (title) => title.id);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    await act(async () => {
      pages?.loadMore();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const alien =
      pages?.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items[0] : undefined;
    // Greed's name changes, and The Gold Rush moves up into the first page while the second keeps it.
    titles = [
      { id: 1, name: "Wing" },
      { id: 3, name: "The Gold Rush" },
      { id: 2, name: "Greed!" },
    ];
    expect(await pollOnce(poller)).toBe("changed");
    const shown = pages?.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items : [];
    expect(shown.map((title) => title.name)).toEqual(["Wing", "The Gold Rush"]);
    expect(shown[0]).toBe(alien);
    mounted.unmount();
  });
});

describe("appendPage", () => {
  it("adds a page after what came before, and takes its cursor", () => {
    expect(appendPage({ items: [1, 2], nextCursor: "a" }, { items: [3], nextCursor: "" })).toEqual({
      items: [1, 2, 3],
      nextCursor: "",
    });
  });
});
