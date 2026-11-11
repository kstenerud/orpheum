// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Ask, AskOutcome, Poller } from "./polling.ts";
import { mount } from "./test-dom.tsx";
import {
  appendPage,
  itemsToCome,
  loadAgain,
  merged,
  usePages,
  type Page,
  type Pages,
} from "./use-pages.ts";

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
    let titles = ["Arkon", "Greed"];
    const load = pagedBy2(() => titles);
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Arkon", "Greed"], nextCursor: "" },
    });
    expect(await pollOnce(poller)).toBe("unchanged");
    titles = ["Aegis", "Arkon", "Greed"];
    expect(await pollOnce(poller)).toBe("changed");
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Aegis", "Arkon"], nextCursor: "2" },
    });
    mounted.unmount();
  });
  it("shows nothing a read its poll gave up answers late (the owner, 2026-10-24)", async () => {
    const poller = handPoller();
    let titles = ["Arkon", "Greed"];
    const load = pagedBy2(() => titles);
    let pages: Pages<string> | undefined;
    function Probe(): null {
      pages = usePages(load, poller);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    titles = ["Aegis", "Arkon", "Greed"];
    const aborter = new AbortController();
    aborter.abort();
    await act(async () => {
      poller.asks.at(-1)?.(() => undefined, aborter.signal);
      for (let turn = 0; turn < 5; turn += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    });
    expect(pages?.loadedSoFar).toEqual({
      state: "loaded",
      value: { items: ["Arkon", "Greed"], nextCursor: "" },
    });
    mounted.unmount();
  });

  it("reads a list whose first read failed again on the next poll, and shows it (2026-10-06)", async () => {
    const poller = handPoller();
    let isServerUp = false;
    const titles = pagedBy2(() => ["Arkon", "Greed"]);
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
      value: { items: ["Arkon", "Greed"], nextCursor: "" },
    });
    mounted.unmount();
    written.mockRestore();
  });

  it("shows again a list whose page more failed, though the poll reads what it showed before", async () => {
    const poller = handPoller();
    let isServerUp = true;
    const titles = pagedBy2(() => ["Arkon", "Greed", "Valdora"]);
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
      value: { items: ["Arkon", "Greed"], nextCursor: "2" },
    });
    mounted.unmount();
    written.mockRestore();
  });
});

describe("a page more (the Fire TV's trace, 2026-11-11)", () => {
  it("is shown in pieces, each drawn as it is shown, and the list stays loading until all are", async () => {
    const more = Array.from({ length: 30 }, (_, index) => `Title ${String(index)}`);
    const load = (cursor: string): Promise<Page<string>> =>
      Promise.resolve(
        cursor === ""
          ? { items: ["Arkon", "Greed"], nextCursor: "2" }
          : { items: more, nextCursor: "" },
      );
    let pages: Pages<string> | undefined;
    const drawn: string[] = [];
    function List(): React.JSX.Element {
      pages = usePages(load);
      const items = pages.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items : [];
      drawn.push(`${String(items.length)}${pages.isLoadingMore ? " loading" : ""}`);
      return createElement(
        "ul",
        null,
        items.map((title) => createElement("li", { key: title }, title)),
      );
    }
    const mounted = await mount(createElement(List));
    await act(async () => {
      pages?.loadMore();
      for (let turn = 0; turn < 5; turn += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    });
    // A dozen a piece, each its own commit, drawn before the next is shown (in-pieces.test.ts holds
    // when the main thread is handed back between them).
    expect(drawn).toContain("14 loading");
    expect(drawn).toContain("26 loading");
    expect(drawn.at(-1)).toBe("32");
    expect(mounted.container.querySelectorAll("li")).toHaveLength(32);
    mounted.unmount();
  });

  it("is read page after page, with no view asking, for a list read whole", async () => {
    const titles = ["Arkon", "Greed", "Valdora", "Zardoz", "Zarvo"];
    let pages: Pages<string> | undefined;
    let isReadWhole = true;
    const load = pagedBy2(() => titles);
    function List(): React.JSX.Element {
      pages = usePages(load, undefined, { isReadWhole });
      return createElement(
        "p",
        null,
        String(pages.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items.length : 0),
      );
    }
    const settle = async (): Promise<void> => {
      await act(async () => {
        for (let turn = 0; turn < 10; turn += 1) {
          await new Promise((resolve) => setTimeout(resolve, 0));
        }
      });
    };
    let mounted = await mount(createElement(List));
    await settle();
    expect(mounted.container.textContent).toBe("5");
    expect(pages?.isLoadingMore).toBe(false);
    mounted.unmount();
    isReadWhole = false;
    mounted = await mount(createElement(List));
    await settle();
    expect(mounted.container.textContent).toBe("2");
    mounted.unmount();
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
      { id: 1, name: "Arkon" },
      { id: 2, name: "Greed" },
      { id: 3, name: "Valdora" },
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
      pages = usePages(load, poller, { idOf: (title) => title.id });
      return null;
    }
    const mounted = await mount(createElement(Probe));
    await act(async () => {
      pages?.loadMore();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const arkon =
      pages?.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items[0] : undefined;
    // Greed's name changes, and Valdora moves up into the first page while the second keeps it.
    titles = [
      { id: 1, name: "Arkon" },
      { id: 3, name: "Valdora" },
      { id: 2, name: "Greed!" },
    ];
    expect(await pollOnce(poller)).toBe("changed");
    const shown = pages?.loadedSoFar.state === "loaded" ? pages.loadedSoFar.value.items : [];
    expect(shown.map((title) => title.name)).toEqual(["Arkon", "Valdora"]);
    expect(shown[0]).toBe(arkon);
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

describe("itemsToCome", () => {
  it("is the list's total less those come, none once the last page has, or with no total", () => {
    expect(itemsToCome({ items: [1, 2], nextCursor: "2", total: 5 })).toBe(3);
    expect(itemsToCome({ items: [1, 2], nextCursor: "", total: 5 })).toBe(0);
    expect(itemsToCome({ items: [1, 2], nextCursor: "2" })).toBe(0);
    expect(itemsToCome({ items: [1, 2, 3], nextCursor: "3", total: 2 })).toBe(0);
  });

  it("takes the total the last page says, read after the list grew, or one before where it says none", () => {
    const pages = [
      { cursor: "", items: [1, 2], nextCursor: "2", total: 5 },
      { cursor: "2", items: [3, 4], nextCursor: "4", total: 7 },
    ];
    expect(itemsToCome(merged(pages))).toBe(3);
    expect(itemsToCome(merged([...pages, { cursor: "4", items: [5], nextCursor: "5" }]))).toBe(2);
  });
});
