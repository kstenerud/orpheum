// SPDX-License-Identifier: MPL-2.0

import {
  castReportEveryMs,
  createApi,
  putOnCommands,
  type GroupView,
  type OpenTogetherSocket,
} from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastDevice, filmToFollow, groupToFollow, handedWhat } from "./device.ts";
import { reachedAt } from "./reach.ts";

/** A group as the server tells it, playing fileId at positionMs. */
function groupOf(fields: Partial<GroupView>): GroupView {
  return {
    state: "playing",
    positionMs: 0,
    rate: 1,
    fileId: 7,
    event: undefined,
    seekedFromMs: 0,
    byMemberId: undefined,
    isHeldPlaying: false,
    ...fields,
  };
}

describe("a Chromecast following its cast's session (D434)", () => {
  it("loads the session's film at where the group stands now, playing it along the follower's", () => {
    expect(filmToFollow(groupOf({ positionMs: 1000 }), 61_000, 0)).toEqual({
      step: "load",
      fileId: 7,
      positionMs: 61_000,
    });
    expect(filmToFollow(groupOf({ state: "paused", positionMs: 5000 }), undefined, 0)).toEqual({
      step: "load",
      fileId: 7,
      positionMs: 5000,
    });
    // On the group's film already, its playing, pausing and seeking are followGroup's (D555 §3).
    expect(filmToFollow(groupOf({ state: "paused" }), 30_000, 7)).toBeUndefined();
  });

  it("stops where the session plays nothing, and does nothing before it is told the group", () => {
    expect(filmToFollow(groupOf({ fileId: 0 }), 0, 7)).toEqual({ step: "stop" });
    expect(filmToFollow(groupOf({ fileId: 0 }), 0, 0)).toBeUndefined();
    expect(filmToFollow(undefined, undefined, 7)).toBeUndefined();
  });
});

describe("a Chromecast taking a cast, which it leads (D337: the group has no film until it plays one)", () => {
  it("puts the handed film on its session at where it started it, paused where the sender was", () => {
    expect(putOnCommands({ fileId: 1504 }, 185_000, false)).toEqual([
      { command: "changeItem", fileId: 1504, positionMs: 185_000 },
    ]);
    expect(putOnCommands({ fileId: 1504 }, 185_000, true)).toEqual([
      { command: "changeItem", fileId: 1504, positionMs: 185_000 },
      { command: "pause" },
    ]);
  });

  it("puts the handed film on as the title it was opened from, and loads the group's as the group plays it (B0 question 20)", () => {
    // One file filed both as a film and as an episode plays as the one opened: as the episode, its
    // Up next plays on, where the film's would end the cast's group.
    expect(putOnCommands({ fileId: 1504, itemId: 2020 }, 185_000, false)).toEqual([
      { command: "changeItem", fileId: 1504, itemId: 2020, positionMs: 185_000 },
    ]);
    expect(filmToFollow(groupOf({ positionMs: 1000, itemId: 2020 }), 1000, 0)).toEqual({
      step: "load",
      fileId: 7,
      itemId: 2020,
      positionMs: 1000,
    });
  });

  it("does not stop its film while the session is not yet on it, and follows it once it is", () => {
    expect(filmToFollow(groupToFollow(groupOf({ fileId: 0 }), 7), 0, 7)).toBeUndefined();
    expect(filmToFollow(groupToFollow(groupOf({ fileId: 9 }), 7), 0, 7)).toBeUndefined();
    const paused = groupOf({ state: "paused" });
    expect(groupToFollow(paused, 7)).toBe(paused);
    // Handed nothing, ready for the next: the group's stop is followed.
    expect(filmToFollow(groupToFollow(groupOf({ fileId: 0 }), 0), 0, 7)).toEqual({ step: "stop" });
  });

  it("takes a new cast, joins the session its cast moved to, puts on another film handed under the same cast, and ignores the same again", () => {
    const none = { castId: undefined, credential: "", fileId: 0 };
    const taken = { castId: 3, credential: "alone", fileId: 7 };
    const handed = { castId: 3, fileId: 7, membershipCredential: "alone" };
    expect(handedWhat(handed, none)).toBe("take");
    expect(handedWhat({ ...handed, castId: 4 }, taken)).toBe("take");
    expect(handedWhat({ ...handed, membershipCredential: "together" }, taken)).toBe("join");
    expect(handedWhat({ ...handed, fileId: 8 }, taken)).toBe("putOn");
    expect(handedWhat({ ...handed, fileId: 0 }, taken)).toBe("putOn");
    expect(handedWhat(handed, taken)).toBe("same");
  });
});

describe("the receiver's own core, reached by its name", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("opens the live and cast connections' socket on that core, as the web client does", () => {
    const opened: string[] = [];
    vi.stubGlobal(
      "WebSocket",
      class {
        constructor(address: URL) {
          opened.push(address.href);
        }
        addEventListener(): void {
          // A stand-in: nothing arrives.
        }
      },
    );
    reachedAt("https://core.example").openSocket({
      onOpen: () => undefined,
      onMessage: () => undefined,
      onClose: () => undefined,
    });
    expect(opened).toEqual(["wss://core.example/api/v1/socket"]);
  });
});

describe("a Chromecast's casts, one after another", () => {
  /** A cast handed on the live connection, as the core sends it. */
  function receiveText(castId: number): string {
    return JSON.stringify({
      type: "receive",
      receive: {
        castId,
        membershipCredential: `cast-${String(castId)}`,
        fileId: 7,
        itemName: "film 7",
        fromDeviceId: 2,
        fromDeviceName: "Chrome on Mac",
      },
    });
  }

  it("keeps its cast as the cast's own session is let go for the part it moved to, ending nothing (D555 §2)", () => {
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    const notes: string[] = [];
    const device = new CastDevice({
      api: createApi({
        origin: "https://core.example",
        getToken: () => "token",
        fetch: () => Promise.resolve(new Response("{}", { status: 503 })),
      }),
      token: "token",
      reached: {
        fetch: () => Promise.reject(new Error("no film's bytes in this test")),
        openSocket: (listeners) => {
          sockets.push(listeners);
          return { send: () => undefined, close: () => undefined };
        },
        close: () => undefined,
      },
      video: {
        addEventListener: () => undefined,
        currentTime: 0,
        paused: true,
        seeking: false,
        readyState: 0,
      } as unknown as HTMLVideoElement,
      note: (line) => {
        notes.push(line);
      },
      onRefused: () => undefined,
    });
    const stop = device.start();
    const [live] = sockets;
    live?.onOpen();
    live?.onMessage(receiveText(1));
    const castSession = sockets[1];
    castSession?.onOpen();
    castSession?.onMessage(JSON.stringify({ type: "ended", reason: "castMoved" }));
    expect(notes.filter((line) => line.startsWith("the cast ended"))).toEqual([]);
    stop();
  });

  it("takes the next cast after one is stopped", async () => {
    const asked: string[] = [];
    const api = createApi({
      origin: "https://core.example",
      getToken: () => "token",
      fetch: (request) => {
        asked.push(`${request.method} ${new URL(request.url).pathname}`);
        // No film starts: what is tested is the casts, not the player.
        return Promise.resolve(new Response("{}", { status: 503 }));
      },
    });
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    const video = {
      addEventListener: () => undefined,
      currentTime: 0,
      paused: true,
      seeking: false,
      readyState: 0,
    } as unknown as HTMLVideoElement;
    const notes: string[] = [];
    const device = new CastDevice({
      api,
      token: "token",
      reached: {
        // The film's bytes are never asked for: no film starts.
        fetch: () => Promise.reject(new Error("no film's bytes in this test")),
        openSocket: (listeners) => {
          sockets.push(listeners);
          return { send: () => undefined, close: () => undefined };
        },
        close: () => undefined,
      },
      video,
      note: (line) => {
        notes.push(line);
      },
      onRefused: () => undefined,
    });
    const stop = device.start();
    const [live] = sockets;
    live?.onOpen();
    live?.onMessage(receiveText(1));
    const castSession = sockets[1];
    castSession?.onOpen();
    // The sender stops the cast: its session ends, the receiver ready for the next.
    castSession?.onMessage(JSON.stringify({ type: "ended", reason: "stopped" }));
    live?.onMessage(receiveText(2));
    await vi.waitFor(() => {
      expect(asked).toContain("POST /api/v1/me/casts/2/accept");
    });
    expect(notes.filter((line) => line.startsWith("the cast ended"))).toEqual([
      "the cast ended: stopped",
    ]);
    stop();
  });
});

describe("a Chromecast's castee says where it is each 5 s, its silence ending the cast (D555 §3)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reports its state to the cast's session each castReportEveryMs", () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const api = createApi({
      origin: "https://core.example",
      getToken: () => "token",
      fetch: () => Promise.resolve(new Response("{}", { status: 503 })),
    });
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    const sent: string[][] = [];
    const video = {
      addEventListener: () => undefined,
      currentTime: 0,
      paused: true,
      seeking: false,
      readyState: 0,
    } as unknown as HTMLVideoElement;
    const device = new CastDevice({
      api,
      token: "token",
      reached: {
        fetch: () => Promise.reject(new Error("no film's bytes in this test")),
        openSocket: (listeners) => {
          const heard: string[] = [];
          sockets.push(listeners);
          sent.push(heard);
          return { send: (text: string) => heard.push(text), close: () => undefined };
        },
        close: () => undefined,
      },
      video,
      note: () => undefined,
      onRefused: () => undefined,
    });
    const stop = device.start();
    sockets[0]?.onOpen();
    sockets[0]?.onMessage(
      JSON.stringify({
        type: "receive",
        receive: {
          castId: 1,
          membershipCredential: "cast-1",
          fileId: 0,
          itemName: "",
          fromDeviceId: 2,
          fromDeviceName: "Chrome on Mac",
        },
      }),
    );
    const castSession = sockets[1];
    castSession?.onOpen();
    castSession?.onMessage(
      JSON.stringify({
        v: 3,
        type: "welcome",
        seq: 1,
        sessionId: 1,
        memberId: 2,
        deviceId: 12,
        group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
        roster: [],
      }),
    );
    // Nothing to play, it is ready at once: its remote is told the cast runs (D555 §4).
    expect((sent[1] ?? []).filter((text) => text.includes('"type":"castState"'))).toEqual([
      expect.stringContaining('"castState":"running"'),
    ]);
    const states = (): number =>
      (sent[1] ?? []).filter((text) => text.includes('"type":"state"')).length;
    const before = states();
    for (let report = 1; report <= 3; report += 1) {
      vi.advanceTimersByTime(castReportEveryMs);
      expect(states() - before).toBeGreaterThanOrEqual(report);
      castSession?.onMessage(JSON.stringify({ v: 3, type: "pong", tMono: 0 }));
    }
    stop();
  });
});
