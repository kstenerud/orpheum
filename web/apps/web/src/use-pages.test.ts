// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Ask, AskOutcome, Poller } from "./polling.ts";
import { mount } from "./test-dom.tsx";
import { appendPage, loadAgain, usePages, type Page, type Pages } from "./use-pages.ts";

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
    expect(again).toEqual({ items: ["A", "B", "C", "D"], nextCursor: "4" });
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
    expect(pages?.loadedSoFar.state).toBe("failed");
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

describe("appendPage", () => {
  it("adds a page after what came before, and takes its cursor", () => {
    expect(appendPage({ items: [1, 2], nextCursor: "a" }, { items: [3], nextCursor: "" })).toEqual({
      items: [1, 2, 3],
      nextCursor: "",
    });
  });
});
