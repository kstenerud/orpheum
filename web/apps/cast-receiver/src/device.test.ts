// SPDX-License-Identifier: MPL-2.0

import { createApi, type GroupView } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  keepSession,
  keptSessionOf,
  pairThisDevice,
  pairingPollEveryMs,
  seekPastMs,
  stepsToFollow,
  type StringStorage,
} from "./device.ts";
import { reachedAt } from "./reach.ts";

/** A storage in memory. */
function memoryStorage(): StringStorage & { readonly items: Map<string, string> } {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

/** A group as the server tells it, playing fileId at positionMs. */
function groupOf(fields: Partial<GroupView>): GroupView {
  return {
    state: "Playing",
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

describe("a Chromecast's kept session", () => {
  it("is kept across launches, and forgotten once refused", () => {
    const storage = memoryStorage();
    expect(keptSessionOf(storage)).toBeUndefined();
    keepSession(storage, { token: "t-1", locator: "loc-1" });
    expect(keptSessionOf(storage)).toEqual({ token: "t-1", locator: "loc-1" });
    keepSession(storage, undefined);
    expect(keptSessionOf(storage)).toBeUndefined();
  });

  it("is none where what is kept is unreadable", () => {
    const storage = memoryStorage();
    storage.setItem("orpheum.cast.session", "{not json");
    expect(keptSessionOf(storage)).toBeUndefined();
    storage.setItem("orpheum.cast.session", JSON.stringify({ token: "", locator: "x" }));
    expect(keptSessionOf(storage)).toBeUndefined();
  });
});

describe("a Chromecast's pairing (D288, its code carried over the Cast channel)", () => {
  /** A core answering a pairing: waiting, then checking with a check code, then states, in turn. */
  function pairingCore(states: readonly string[]): {
    readonly asked: string[];
    readonly fetch: (request: Request) => Promise<Response>;
  } {
    const asked: string[] = [];
    let polls = 0;
    const answer = (body: unknown): Response =>
      new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } });
    return {
      asked,
      fetch: async (request) => {
        const path = new URL(request.url).pathname;
        asked.push(`${path} ${await request.text()}`);
        if (path.endsWith("/pairings")) {
          return answer({ code: "abcd-efgh", secret: "s3cret", expiresAt: 0 });
        }
        if (path.endsWith("/pairings/state")) {
          const state = states[Math.min(polls, states.length - 1)];
          polls += 1;
          return answer(state === "waiting" ? { state } : { state, checkCode: "42-17" });
        }
        return answer({ token: "own-token", locator: "own-loc", deviceSecret: "d", user: {} });
      },
    };
  }

  it("tells its code, shows the check code once entered, and takes its own session once matched", async () => {
    const core = pairingCore(["waiting", "checking", "checking", "activated"]);
    const api = createApi({ origin: "https://core.example", getToken: () => undefined, ...core });
    const told: string[] = [];
    const waits: number[] = [];
    const session = await pairThisDevice(api, {
      tellCode: (code) => told.push(`code ${code}`),
      showCheckCode: (checkCode) => told.push(`check ${checkCode}`),
      waitMs: async (ms) => {
        waits.push(ms);
        await Promise.resolve();
      },
    });
    expect(session).toEqual({ token: "own-token", locator: "own-loc" });
    expect(told).toEqual(["code abcd-efgh", "check 42-17"]);
    expect(waits).toEqual([pairingPollEveryMs, pairingPollEveryMs, pairingPollEveryMs]);
    expect(core.asked[0]).toBe('/api/v1/pairings {"platform":"chromecast"}');
    expect(core.asked.at(-1)).toBe('/api/v1/pairings/session {"secret":"s3cret"}');
  });

  it("fails where the pairing ends unconfirmed, taking no session", async () => {
    const core = pairingCore(["checking", "ended"]);
    const api = createApi({ origin: "https://core.example", getToken: () => undefined, ...core });
    await expect(
      pairThisDevice(api, {
        tellCode: () => undefined,
        showCheckCode: () => undefined,
        waitMs: async () => {
          await Promise.resolve();
        },
      }),
    ).rejects.toThrow("the pairing ended unconfirmed");
    expect(core.asked.some((each) => each.includes("/pairings/session"))).toBe(false);
  });
});

describe("a Chromecast following its cast's session (D434)", () => {
  const stopped = { fileId: 0, positionMs: 0, isPlaying: false };

  it("loads the session's film at where the group stands now, and plays it where the group plays", () => {
    expect(stepsToFollow(groupOf({ positionMs: 1000 }), 61_000, stopped)).toEqual([
      { step: "load", fileId: 7, positionMs: 61_000 },
      { step: "play" },
    ]);
    expect(stepsToFollow(groupOf({ state: "Paused", positionMs: 5000 }), 5000, stopped)).toEqual([
      { step: "load", fileId: 7, positionMs: 5000 },
    ]);
  });

  it("plays, pauses and seeks as its remote has the group do", () => {
    const playing = { fileId: 7, positionMs: 30_000, isPlaying: true };
    expect(stepsToFollow(groupOf({ state: "Paused" }), 30_000, playing)).toEqual([
      { step: "pause" },
    ]);
    expect(stepsToFollow(groupOf({}), 30_000, { ...playing, isPlaying: false })).toEqual([
      { step: "play" },
    ]);
    expect(stepsToFollow(groupOf({}), 30_000 + seekPastMs + 1, playing)).toEqual([
      { step: "seek", positionMs: 30_000 + seekPastMs + 1 },
    ]);
    expect(stepsToFollow(groupOf({}), 30_000 + seekPastMs, playing)).toEqual([]);
  });

  it("stops where the session plays nothing, and does nothing before it is told the group", () => {
    const playing = { fileId: 7, positionMs: 30_000, isPlaying: true };
    expect(stepsToFollow(groupOf({ fileId: 0 }), 0, playing)).toEqual([{ step: "stop" }]);
    expect(stepsToFollow(groupOf({ fileId: 0 }), 0, stopped)).toEqual([]);
    expect(stepsToFollow(undefined, undefined, playing)).toEqual([]);
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
        static readonly OPEN = 1;
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
