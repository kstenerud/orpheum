// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { createApi, upNextLeadMs, type OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastDevice } from "./device.ts";

/** A cast of film 7 handed on the live connection, as the core sends it. */
const receiveFilm7 = JSON.stringify({
  type: "receive",
  receive: {
    castId: 1,
    membershipCredential: "cast-1",
    fileId: 7,
    itemName: "film 7",
    fromDeviceId: 2,
    fromDeviceName: "Chrome on Mac",
  },
});

describe("a Chromecast's Up next, for the film its cast plays (D471)", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("asks what plays after the film it starts, shows it near the end, and takes it away once the film stops", async () => {
    const asked: string[] = [];
    const api = createApi({
      origin: "https://core.example",
      getToken: () => "token",
      fetch: (request) => {
        const path = new URL(request.url).pathname;
        asked.push(`${request.method} ${path}`);
        if (path.endsWith("/playback")) {
          return Promise.resolve(
            Response.json({ url: "https://core.example/media/7.m3u8", resumePositionMs: 0 }),
          );
        }
        if (path.endsWith("/up-next")) {
          const episode = {
            itemId: 8,
            fileId: 8,
            name: "Bag",
            groupNumber: 1,
            numberInGroup: 3,
            groupName: "Disc 1",
          };
          return Promise.resolve(
            Response.json({
              upNext: { episode: { ...episode, isPartway: false }, ordering: "dvd" },
            }),
          );
        }
        return Promise.resolve(Response.json({}));
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
    const shown = (): string | undefined => {
      const drawn = document.querySelector<HTMLElement>(".receiver__up-next");
      return drawn?.hidden === false ? drawn.textContent : undefined;
    };
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
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
      onTaken: () => undefined,
    });
    const stop = device.start();
    const [live] = sockets;
    live?.onOpen();
    live?.onMessage(receiveFilm7);
    await vi.waitFor(() => {
      expect(asked).toContain("GET /api/v1/files/7/up-next");
    });
    at(1200 - upNextLeadMs / 1000 + 1);
    await vi.waitFor(() => {
      expect(shown()).toBe("Up next: Disc 1 · 3 · BagChoose it on your phone or computer");
    });
    // The sender stops the cast: the film stops, and its Up next goes with it.
    const castSession = sockets[1];
    castSession?.onOpen();
    castSession?.onMessage(JSON.stringify({ type: "ended", reason: "stopped" }));
    expect(shown()).toBeUndefined();
    at(1200);
    expect(shown()).toBeUndefined();
    stop();
  });
});
