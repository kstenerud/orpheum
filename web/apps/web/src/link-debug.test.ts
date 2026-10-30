// SPDX-License-Identifier: MPL-2.0

import type { OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Reaches } from "./client-reaches.ts";
import { LinkDebug, linkCapKey } from "./link-debug.ts";

/** What a socket's owner hears from it. */
type TogetherSocketListeners = Parameters<OpenTogetherSocket>[0];

/** A tab's storage, kept in a map. */
function tabStorageOf(
  kept = new Map<string, string>(),
): Pick<Storage, "getItem" | "setItem" | "removeItem"> & { readonly kept: Map<string, string> } {
  return {
    kept,
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => {
      kept.set(key, value);
    },
    removeItem: (key) => {
      kept.delete(key);
    },
  };
}

/** The tab's debug link on vitest's fake clock, and the page it tells of the browser back online. */
function linkDebugOf(storage = tabStorageOf()): {
  readonly linkDebug: LinkDebug;
  readonly page: EventTarget;
} {
  const page = new EventTarget();
  const linkDebug = new LinkDebug(
    storage,
    {
      nowMs: () => Date.now(),
      setTimeout: (run, ms) => setTimeout(run, ms),
      clearTimeout: (timer) => {
        clearTimeout(timer);
      },
    },
    page,
  );
  return { linkDebug, page };
}

/** How the tab reaches its core, a stand-in: what it fetched, and the sockets it opened. */
function reachesOf(): {
  readonly reaches: Reaches;
  readonly fetched: string[];
  readonly opened: { readonly listeners: TogetherSocketListeners; isClosed: boolean }[];
} {
  const fetched: string[] = [];
  const opened: { readonly listeners: TogetherSocketListeners; isClosed: boolean }[] = [];
  return {
    fetched,
    opened,
    reaches: {
      fetch: (request) => {
        fetched.push(new URL(request.url).pathname);
        return Promise.resolve(new Response("{}"));
      },
      openTogetherSocket: (listeners) => {
        const socket = { listeners, isClosed: false };
        opened.push(socket);
        return {
          send: () => undefined,
          close: () => {
            socket.isClosed = true;
            listeners.onClose(1005);
          },
        };
      },
      pathMoves: {
        count: () => 0,
        to: () => undefined,
        subscribe: () => () => undefined,
        isOverWebrtc: () => false,
      },
      ownScript: undefined,
      onPathBack: () => () => undefined,
    },
  };
}

/** Listeners of a socket, counting its closes. */
function listening(): { readonly listeners: TogetherSocketListeners; readonly closes: number[] } {
  const closes: number[] = [];
  return {
    closes,
    listeners: {
      onOpen: () => undefined,
      onMessage: () => undefined,
      onClose: (code) => closes.push(code),
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
});

afterEach(() => {
  vi.useRealTimers();
});

// Chrome's DevTools throttling never reached the WebRTC carrier (VQ128's run): a debug tool drops the
// tab's connection, and caps its download rate, for the tab alone (D380).
describe("the tab's debug link (D380)", () => {
  it("drops the connection for the time asked: its sockets closed, its requests and sockets refused, then back as after a loss", async () => {
    const { linkDebug, page } = linkDebugOf();
    const { reaches, fetched, opened } = reachesOf();
    const shaped = linkDebug.shape(reaches);
    const live = listening();
    shaped.openTogetherSocket(live.listeners);
    const backOnline = vi.fn();
    page.addEventListener("online", backOnline);

    linkDebug.dropFor(10_000);
    expect(opened[0]?.isClosed).toBe(true);
    expect(live.closes).toEqual([1005]);
    await expect(shaped.fetch(new Request("https://core.test/api/v1/me"))).rejects.toThrow(
      TypeError,
    );
    const again = listening();
    shaped.openTogetherSocket(again.listeners);
    vi.advanceTimersByTime(0);
    expect(again.closes).toEqual([1006]);
    expect(opened).toHaveLength(1);

    vi.advanceTimersByTime(9_999);
    expect(linkDebug.isDropped()).toBe(true);
    expect(backOnline).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(linkDebug.isDropped()).toBe(false);
    expect(backOnline).toHaveBeenCalledOnce();
    await shaped.fetch(new Request("https://core.test/api/v1/me"));
    expect(fetched).toEqual(["/api/v1/me"]);
    shaped.openTogetherSocket(listening().listeners);
    expect(opened).toHaveLength(2);
  });

  it("keeps the cap for the tab, read back as it opens again, and lifts it", () => {
    const storage = tabStorageOf();
    const { linkDebug } = linkDebugOf(storage);
    const changes = vi.fn();
    linkDebug.subscribe(changes);
    expect(linkDebug.capKbps()).toBeUndefined();
    linkDebug.setCap(1000);
    expect(storage.kept.get(linkCapKey)).toBe("1000");
    expect(linkDebugOf(storage).linkDebug.capKbps()).toBe(1000);
    linkDebug.setCap(undefined);
    expect(storage.kept.has(linkCapKey)).toBe(false);
    expect(changes).toHaveBeenCalledTimes(2);
  });
});
