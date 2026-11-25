// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { createApi, type OpenTogetherSocket } from "@orpheum/sdk";
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

/** An episode the core says plays after film 7. */
const nextEpisode = {
  episode: {
    itemId: 8,
    fileId: 8,
    name: "Bag",
    groupNumber: 1,
    numberInGroup: 3,
    isPartway: false,
  },
  ordering: "aired",
};

/**
 * A receiver taking the cast of film 7, after which upNext plays (none for undefined), its film then
 * played to its end; what it asked the core, each as "METHOD path body".
 */
async function playedToItsEnd(upNext: object | undefined): Promise<string[]> {
  const asked: string[] = [];
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: async (request) => {
      const path = new URL(request.url).pathname;
      asked.push(`${request.method} ${path} ${await request.text()}`.trim());
      if (path.endsWith("/playback")) {
        return Response.json({ url: "https://core.example/media/7.m3u8", resumePositionMs: 0 });
      }
      if (path.endsWith("/up-next")) {
        return Response.json(upNext === undefined ? {} : { upNext });
      }
      return Response.json({});
    },
  });
  const video = document.createElement("video");
  Object.defineProperty(video, "duration", { get: () => 1200 });
  Object.defineProperty(video, "currentTime", { get: () => 1200, set: () => undefined });
  Object.defineProperty(video, "ended", { get: () => true });
  document.body.append(video);
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
  });
  const stop = device.start();
  const [live] = sockets;
  live?.onOpen();
  live?.onMessage(receiveFilm7);
  await vi.waitFor(() => {
    expect(asked).toContain("GET /api/v1/files/7/up-next");
  });
  // Its answer read, as it is long before a film's end.
  await new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
  video.dispatchEvent(new Event("ended"));
  await new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
  stop();
  return asked;
}

describe("a Chromecast at its cast's film's end", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("has the cast play nothing where nothing plays after the film, waiting ready for the next", async () => {
    // The plan file's "A film played to its end with nothing next goes back to its page", its cast
    // part: the receiver's black screen, still connected, as a browser receiver waits ready.
    const asked = await playedToItsEnd(undefined);
    expect(asked).toContain("POST /api/v1/me/casts/1/play {}");
  });

  it("leaves the cast as it is where an episode plays next, which the caster's Up next moves it on to", async () => {
    const asked = await playedToItsEnd(nextEpisode);
    expect(asked.filter((each) => each.includes("/me/casts/1/play"))).toEqual([]);
  });
});
