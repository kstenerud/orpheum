// SPDX-License-Identifier: MPL-2.0

import type {
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { loadedPlaylistLoader } from "./loaded-playlists.ts";

/** The addresses the stand-in carrier was asked for. */
const carried: string[] = [];

/** A stand-in for hls.js's loader, which only notes what it is asked for. */
class Carrier implements Loader<LoaderContext> {
  context: LoaderContext | null = null;
  stats: LoaderStats = {
    aborted: false,
    loaded: 0,
    retry: 0,
    total: 0,
    chunkCount: 0,
    bwEstimate: 0,
    loading: { start: 0, first: 0, end: 0 },
    parsing: { start: 0, end: 0 },
    buffering: { start: 0, first: 0, end: 0 },
  };

  destroy(): void {
    // nothing held
  }

  abort(): void {
    // nothing under way
  }

  load(context: LoaderContext): void {
    carried.push(context.url);
  }
}

/** Asks for url as hls.js asks for a playlist, by a loader of its own; returns the texts it was answered. */
function ask(
  loaderClass: ReturnType<typeof loadedPlaylistLoader>,
  url: string,
): { texts: string[]; loader: Loader<LoaderContext> } {
  const texts: string[] = [];
  const callbacks: LoaderCallbacks<LoaderContext> = {
    onSuccess: (response) =>
      texts.push(typeof response.data === "string" ? response.data : "bytes"),
    onError: () => undefined,
    onTimeout: () => undefined,
  };
  const loader = new loaderClass({} as HlsConfig);
  loader.load({ url, responseType: "text" } as LoaderContext, {} as LoaderConfiguration, callbacks);
  return { texts, loader };
}

// The Play page hls.js runs in, whose address hls.js makes every playlist's whole against.
vi.stubGlobal("location", new URL("https://orpheum.test/play/1"));
const base = "https://orpheum.test/api/v1/bytes/media/chunks/tok/";

beforeEach(() => {
  carried.length = 0;
});

// A Fire TV's start waited on the master playlist, then on its quality's and its sound's, each a
// round trip after the last (start-chain-2026-11-16.md): the decision carries them, and hls.js is
// handed them as loaded (D502 §3).
describe("the playlists a playback's start was answered with", () => {
  const loaded = {
    master: { url: "/api/v1/bytes/media/chunks/tok/master.m3u8", text: "#EXTM3U\nmaster" },
    picture: {
      url: "/api/v1/bytes/media/chunks/tok/picture/stream.m3u8",
      text: "#EXTM3U\npicture",
    },
  };

  it("answers hls.js's ask for each from memory, whole against the page's address, asking the core nothing", async () => {
    const loaderClass = loadedPlaylistLoader(loaded, Carrier);
    const master = ask(loaderClass, `${base}master.m3u8`);
    const picture = ask(loaderClass, `${base}picture/stream.m3u8`);
    expect(master.texts).toEqual([]);
    await Promise.resolve();
    expect(master.texts).toEqual(["#EXTM3U\nmaster"]);
    expect(picture.texts).toEqual(["#EXTM3U\npicture"]);
    expect(master.loader.stats.loaded).toBe("#EXTM3U\nmaster".length);
    expect(carried).toEqual([]);
  });

  it("asks the core for a playlist it was not answered with, and for one asked again", async () => {
    const loaderClass = loadedPlaylistLoader(loaded, Carrier);
    ask(loaderClass, `${base}master.m3u8`);
    ask(loaderClass, `${base}master.m3u8`);
    ask(loaderClass, `${base}sound0-own/stream.m3u8`);
    await Promise.resolve();
    expect(carried).toEqual([`${base}master.m3u8`, `${base}sound0-own/stream.m3u8`]);
  });

  it("tells nothing of a playlist whose load hls.js gave up", async () => {
    const loaderClass = loadedPlaylistLoader(loaded, Carrier);
    const master = ask(loaderClass, `${base}master.m3u8`);
    master.loader.abort();
    await Promise.resolve();
    expect(master.texts).toEqual([]);
  });
});
