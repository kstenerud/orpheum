// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it } from "vitest";

import type { Ask, Poller } from "./polling.ts";
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

async function pollOnce(poller: { readonly asks: readonly Ask[] }): Promise<boolean> {
  const ask = poller.asks.at(-1);
  if (ask === undefined) {
    throw new Error("nothing is polling");
  }
  let wasChanged: boolean | undefined;
  await act(async () => {
    ask((isChanged) => {
      wasChanged = isChanged;
    });
    for (let turn = 0; turn < 5; turn += 1) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  });
  if (wasChanged === undefined) {
    throw new Error("the ask never answered");
  }
  return wasChanged;
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
    expect(await pollOnce(poller)).toBe(false);
    titles = ["Nosferatu", "Wing", "Greed"];
    expect(await pollOnce(poller)).toBe(true);
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Nosferatu", "Wing"], nextCursor: "2" },
    });
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
