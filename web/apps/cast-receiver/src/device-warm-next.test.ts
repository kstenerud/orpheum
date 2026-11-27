// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import type { ChunkedPlayback } from "@orpheum/player";
import { createApi, upNextLeadMs, warmUpKeptMs, type OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastDevice } from "./device.ts";

/** A cast of fileId handed on the live connection, as the core sends it, cast 1 in its session. */
function handed(fileId: number, itemId?: number): string {
  return JSON.stringify({
    type: "receive",
    receive: {
      castId: 1,
      membershipCredential: "cast-1",
      fileId,
      ...(itemId === undefined ? {} : { itemId }),
      itemName: `file ${String(fileId)}`,
      fromDeviceId: 2,
      fromDeviceName: "Chrome on Mac",
    },
  });
}

/**
 * A Chromecast handed a cast of film 7, of 1200 s, whose next episode is file 8 (item 8): each
 * playback it asks for, by its file and cause, each it lets go of, and the films it started. A
 * playback of a file failing names fails.
 */
function castOfFilm7(failing: readonly number[] = []): {
  readonly heard: readonly string[];
  readonly started: readonly string[];
  readonly at: (toS: number) => void;
  readonly hand: (fileId: number, itemId?: number) => void;
  readonly clock: { ms: number };
  readonly stop: () => void;
} {
  const heard: string[] = [];
  const asks = new Map<number, number>();
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: async (request) => {
      const path = new URL(request.url).pathname;
      if (request.method === "POST" && path.endsWith("/playback")) {
        const body = (await request.json()) as { fileId: number; cause: string };
        heard.push(`ask file ${String(body.fileId)} for ${body.cause}`);
        if (failing.includes(body.fileId)) {
          return Response.json({ title: "no" }, { status: 500 });
        }
        const count = (asks.get(body.fileId) ?? 0) + 1;
        asks.set(body.fileId, count);
        const mediaToken = `${String(body.fileId)}-${String(count)}`;
        return Response.json({
          delivery: "direct",
          url: `/media/${mediaToken}`,
          notDirectReasons: [],
          mediaToken,
          resumePositionMs: 0,
          onLadder: { url: `/chunks/${mediaToken}/master.m3u8`, startAtBps: 4_000_000 },
        });
      }
      if (request.method === "DELETE") {
        heard.push(`let go of ${path.split("/").at(-1) ?? ""}`);
        return new Response(null, { status: 204 });
      }
      if (path.endsWith("/files/7/up-next")) {
        const episode = { itemId: 8, fileId: 8, name: "Bag", groupNumber: 1, numberInGroup: 3 };
        return Response.json({
          upNext: { episode: { ...episode, isPartway: false }, ordering: "aired" },
        });
      }
      return Response.json({});
    },
  });
  const video = document.createElement("video");
  let atS = 0;
  Object.defineProperty(video, "duration", { get: () => 1200 });
  Object.defineProperty(video, "currentTime", { get: () => atS, set: () => undefined });
  Object.defineProperty(video, "ended", { get: () => atS >= 1200 });
  document.body.append(video);
  const at = (toS: number): void => {
    atS = toS;
    video.dispatchEvent(new Event("timeupdate"));
  };
  const sockets: Parameters<OpenTogetherSocket>[0][] = [];
  const started: string[] = [];
  const clock = { ms: 0 };
  const device = new CastDevice({
    api,
    token: "token",
    reached: {
      fetch: () => Promise.reject(new Error("no film's bytes in this test")),
      openSocket: (listeners) => {
        sockets.push(listeners);
        return { send: () => undefined, close: () => undefined };
      },
      close: () => undefined,
    },
    video,
    note: () => undefined,
    onRefused: () => undefined,
    startsFilm: (_video, url) => {
      started.push(new URL(url).pathname);
      return { destroy: () => undefined } as unknown as ChunkedPlayback;
    },
    nowMs: () => clock.ms,
  });
  const stop = device.start();
  const [live] = sockets;
  live?.onOpen();
  live?.onMessage(handed(7));
  const hand = (fileId: number, itemId?: number): void => {
    live?.onMessage(handed(fileId, itemId));
  };
  return { heard, started, at, hand, clock, stop };
}

/** Lets every request sent so far be answered, and what each answer starts be done. */
async function settled(): Promise<void> {
  for (let round = 0; round < 5; round += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

/** Has film 7 play into its last half minute, where its Up next shows, and every ask answered. */
async function nearItsEnd(cast: ReturnType<typeof castOfFilm7>): Promise<void> {
  await vi.waitFor(() => {
    expect(cast.started).toEqual(["/chunks/7-1/master.m3u8"]);
  });
  await settled(); // what plays next, read
  cast.at(1200 - upNextLeadMs / 1000 + 1);
  cast.at(1200 - upNextLeadMs / 1000 + 2);
  cast.at(1200 - upNextLeadMs / 1000 + 3);
  await settled();
}

describe("a Chromecast's next episode, warmed as its Up next shows (the owner, 2026-11-27)", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("asks the next episode's playback once, for Up next, near the film's end, leaving the film playing's alone", async () => {
    const cast = castOfFilm7();
    await vi.waitFor(() => {
      expect(cast.started).toHaveLength(1);
    });
    await settled();
    cast.at(600); // mid-film: nothing is warmed yet
    await settled();
    expect(cast.heard).toEqual(["ask file 7 for playPage"]);
    await nearItsEnd(cast);
    expect(cast.heard).toEqual(["ask file 7 for playPage", "ask file 8 for upNext"]);
    cast.stop();
  });

  it("plays the next episode, once the cast moves on to it, on the decision warmed, asking nothing again", async () => {
    const cast = castOfFilm7();
    await nearItsEnd(cast);
    cast.hand(8, 8);
    await vi.waitFor(() => {
      expect(cast.started).toEqual(["/chunks/7-1/master.m3u8", "/chunks/8-1/master.m3u8"]);
    });
    await settled();
    expect(cast.heard).toEqual(["ask file 7 for playPage", "ask file 8 for upNext"]);
    cast.stop();
  });

  it("lets go of the warmed playback where the cast moves on to another film instead, which it asks for", async () => {
    const cast = castOfFilm7();
    await nearItsEnd(cast);
    cast.hand(9);
    await vi.waitFor(() => {
      expect(cast.started).toEqual(["/chunks/7-1/master.m3u8", "/chunks/9-1/master.m3u8"]);
    });
    await settled();
    expect(cast.heard.slice(0, 2)).toEqual(["ask file 7 for playPage", "ask file 8 for upNext"]);
    expect(new Set(cast.heard.slice(2))).toEqual(
      new Set(["let go of 8-1", "ask file 9 for playPage"]),
    );
    expect(cast.heard).toHaveLength(4);
    cast.stop();
  });

  it("asks afresh for a warm-up kept too long, letting it go, and for one that failed", async () => {
    const stale = castOfFilm7();
    await nearItsEnd(stale);
    stale.clock.ms = warmUpKeptMs;
    stale.hand(8, 8);
    await vi.waitFor(() => {
      expect(stale.started).toEqual(["/chunks/7-1/master.m3u8", "/chunks/8-2/master.m3u8"]);
    });
    await settled();
    expect(stale.heard.slice(0, 2)).toEqual(["ask file 7 for playPage", "ask file 8 for upNext"]);
    expect(new Set(stale.heard.slice(2))).toEqual(
      new Set(["ask file 8 for warmUpStale", "let go of 8-1"]),
    );
    expect(stale.heard).toHaveLength(4);
    stale.stop();
    document.body.replaceChildren();

    const failed = castOfFilm7([8]);
    await nearItsEnd(failed);
    failed.hand(8, 8);
    await settled();
    expect(failed.heard).toEqual([
      "ask file 7 for playPage",
      "ask file 8 for upNext",
      "ask file 8 for warmUpFailed",
    ]);
    failed.stop();
  });

  it("lets go of the warmed playback as the cast ends, the film playing's own left to the core", async () => {
    const cast = castOfFilm7();
    await nearItsEnd(cast);
    cast.stop();
    await settled();
    expect(cast.heard).toEqual([
      "ask file 7 for playPage",
      "ask file 8 for upNext",
      "let go of 8-1",
    ]);
  });
});
