// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import type { ChunkedPlayback } from "@orpheum/player";
import { createApi, playerLoadedChunkEvent, type OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastDevice } from "./device.ts";

/** A cast of film 7 handed on the live connection, the sender at 30 s, as the core sends it. */
const receiveFilm7 = JSON.stringify({
  type: "receive",
  receive: {
    castId: 1,
    membershipCredential: "cast-1",
    fileId: 7,
    itemName: "film 7",
    positionMs: 30_000,
    fromDeviceId: 2,
    fromDeviceName: "Chrome on Mac",
  },
});

/** The video a test watches: where it stands, and what was asked of it. */
interface WatchedVideo {
  readonly video: HTMLVideoElement;
  readonly asked: string[];
}

/** A video standing at 30 s with all of film 7 loaded, keeping each play, pause and seek asked of it. */
function watchedVideo(): WatchedVideo {
  const asked: string[] = [];
  const video = document.createElement("video");
  let atS = 30;
  let isPaused = true;
  Object.defineProperty(video, "duration", { get: () => 1200 });
  Object.defineProperty(video, "readyState", { get: () => 4 });
  Object.defineProperty(video, "seeking", { get: () => false });
  Object.defineProperty(video, "ended", { get: () => false });
  Object.defineProperty(video, "paused", { get: () => isPaused });
  Object.defineProperty(video, "buffered", {
    get: () => ({ length: 1, start: () => 0, end: () => 1200 }),
  });
  Object.defineProperty(video, "currentTime", {
    get: () => atS,
    set: (s: number) => {
      asked.push(`seek ${String(s)}`);
      atS = s;
    },
  });
  video.play = () => {
    asked.push("play");
    isPaused = false;
    return Promise.resolve();
  };
  video.pause = () => {
    asked.push("pause");
    isPaused = true;
  };
  document.body.append(video);
  return { video, asked };
}

/** A group message of the cast's session at seq, its group playing film 7 as fields say. */
function groupText(seq: number, fields: object): string {
  return JSON.stringify({
    v: 3,
    type: "group",
    seq,
    group: { state: "playing", positionMs: 30_000, rate: 1, fileId: 7, ...fields },
  });
}

/**
 * A Chromecast that took the cast of film 7 and started it, welcomed in the cast's session as its
 * leader: its video, its cast session's socket, and what it sent there.
 */
async function leadingFilm7(): Promise<
  WatchedVideo & {
    readonly session: Parameters<OpenTogetherSocket>[0];
    readonly sent: string[];
    /** The live connection's socket, then each session socket opened, in order. */
    readonly sockets: Parameters<OpenTogetherSocket>[0][];
    /** How many sockets were closed. */
    readonly closed: { count: number };
    readonly stop: () => void;
  }
> {
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: (request) =>
      Promise.resolve(
        new URL(request.url).pathname.endsWith("/playback")
          ? Response.json({ url: "https://core.example/media/7.m3u8", resumePositionMs: 0 })
          : Response.json({}),
      ),
  });
  const watched = watchedVideo();
  const sockets: Parameters<OpenTogetherSocket>[0][] = [];
  const sent: string[] = [];
  const started: string[] = [];
  const closed = { count: 0 };
  const device = new CastDevice({
    api,
    token: "token",
    reached: {
      fetch: () => Promise.reject(new Error("no film's bytes in this test")),
      openSocket: (listeners) => {
        sockets.push(listeners);
        return {
          send: (text: string) => sent.push(text),
          close: () => {
            closed.count += 1;
          },
        };
      },
      close: () => undefined,
    },
    video: watched.video,
    note: () => undefined,
    onRefused: () => undefined,
    startsFilm: (_video, url) => {
      started.push(url);
      return { destroy: () => undefined } as unknown as ChunkedPlayback;
    },
  });
  const stop = device.start();
  const [live] = sockets;
  live?.onOpen();
  live?.onMessage(receiveFilm7);
  await vi.waitFor(() => {
    expect(started).toEqual(["https://core.example/media/7.m3u8"]);
  });
  const session = sockets[1];
  if (session === undefined) {
    throw new Error("the cast's session was not opened");
  }
  session.onOpen();
  session.onMessage(
    JSON.stringify({
      v: 3,
      type: "welcome",
      seq: 1,
      sessionId: 1,
      memberId: 2,
      deviceId: 12,
      group: { state: "playing", positionMs: 30_000, rate: 1, fileId: 7 },
      roster: [{ id: 2, name: "Karl", devices: [{ id: 12, role: "video", isLeader: true }] }],
    }),
  );
  return { ...watched, session, sent, sockets, closed, stop };
}

describe("a Chromecast plays its cast's film along with the session as every player does (D555 §3)", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("pauses, plays and seeks as its remote has the group do, by the SDK's follower", async () => {
    const { session, asked, stop } = await leadingFilm7();
    asked.length = 0;
    session.onMessage(
      groupText(2, { state: "paused", positionMs: 30_000, event: "paused", byMemberId: 2 }),
    );
    expect(asked).toEqual(["pause"]);
    // A seek of 2 s, which its own follower left alone below 3 s; the remote asked for it.
    session.onMessage(
      groupText(3, {
        state: "paused",
        positionMs: 32_000,
        event: "seeked",
        seekedFromMs: 30_000,
        byMemberId: 2,
      }),
    );
    expect(asked).toEqual(["pause", "seek 32"]);
    // A group waiting for its leader to play: in a cast the receiver leads it, so it plays (D371 §1).
    session.onMessage(groupText(4, { state: "waiting", positionMs: 32_000, event: "played" }));
    expect(asked).toEqual(["pause", "seek 32", "play"]);
    stop();
  });

  it("tells its session each chunk its player loaded, as the Play page's player does", async () => {
    const { video, sent, stop } = await leadingFilm7();
    video.dispatchEvent(
      new CustomEvent(playerLoadedChunkEvent, {
        detail: {
          track: "video",
          loadMs: 800,
          loadedKbps: 9000,
          qualityKbps: 4200,
          filmMs: 34_000,
        },
      }),
    );
    expect(sent.filter((text) => text.includes('"chunkLoaded"'))).toHaveLength(1);
    stop();
  });

  // Its person started a Watch Together session while casting, and the core handed the cast again in
  // that session: the receiver stayed in the one it left, silent in the new one, and its cast ended
  // as gone 20 s later (the Chromecast on staging, 2026-11-25).
  it("joins the session its cast is handed in again, leaving the one it played in (D555 §7)", async () => {
    const { sockets, sent, closed, stop } = await leadingFilm7();
    const [live] = sockets;
    const moved = JSON.parse(receiveFilm7) as { receive: { membershipCredential: string } };
    moved.receive.membershipCredential = "together-5";
    live?.onMessage(JSON.stringify(moved));
    expect(closed.count).toBe(1);
    expect(sockets).toHaveLength(3);
    sent.length = 0;
    sockets[2]?.onOpen();
    expect(sent.some((text) => text.includes('"together-5"'))).toBe(true);
    // The same hand again, as the live connection comes back: nothing more.
    live?.onMessage(JSON.stringify(moved));
    expect(sockets).toHaveLength(3);
    stop();
  });
});
