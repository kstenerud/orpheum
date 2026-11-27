// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import type { ChunkedPlayback } from "@orpheum/player";
import { createApi, type OpenTogetherSocket } from "@orpheum/sdk";
import { describe, expect, it, vi } from "vitest";

import { CastDevice } from "./device.ts";

/**
 * A Chromecast handed a cast of file 7, its start on the ladder held by its node's speed as isHeld
 * says: whether each film it started was told so.
 */
function castHeld(isHeld: boolean): {
  readonly told: (boolean | undefined)[];
  readonly stop: () => void;
} {
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: (request) => {
      const path = new URL(request.url).pathname;
      if (request.method === "POST" && path.endsWith("/playback")) {
        return Promise.resolve(
          Response.json({
            delivery: "direct",
            url: "/media/7-1",
            notDirectReasons: [],
            mediaToken: "7-1",
            resumePositionMs: 0,
            onLadder: {
              url: "/chunks/7-1/master.m3u8",
              startAtBps: 1_896_000,
              isStartHeldByNodeSpeed: isHeld,
            },
          }),
        );
      }
      return Promise.resolve(Response.json({}));
    },
  });
  const video = document.createElement("video");
  document.body.append(video);
  const sockets: Parameters<OpenTogetherSocket>[0][] = [];
  const told: (boolean | undefined)[] = [];
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
    startsFilm: (_video, _url, options) => {
      told.push(options.isStartHeldByNodeSpeed);
      return { destroy: () => undefined } as unknown as ChunkedPlayback;
    },
    nowMs: () => 0,
  });
  const stop = device.start();
  const [live] = sockets;
  live?.onOpen();
  live?.onMessage(
    JSON.stringify({
      type: "receive",
      receive: {
        castId: 1,
        membershipCredential: "cast-1",
        fileId: 7,
        itemName: "file 7",
        fromDeviceId: 2,
        fromDeviceName: "Chrome on Mac",
      },
    }),
  );
  return { told, stop };
}

// A Chromecast's start the core held by its node's speed jumps once to its link's quality as a
// browser's does (start-quality-by-node-speed.md §4, S4).
describe("a Chromecast's start its node's speed held", () => {
  it("tells its player the start was held, and only when its offer says so", async () => {
    for (const isHeld of [true, false]) {
      const cast = castHeld(isHeld);
      await vi.waitFor(() => {
        expect(cast.told).toEqual([isHeld]);
      });
      cast.stop();
      document.body.replaceChildren();
    }
  });
});
