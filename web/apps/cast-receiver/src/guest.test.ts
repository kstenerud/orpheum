// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";

import { apiPrefix, type HeldSession, type OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { receiverFormats } from "./device.ts";
import { castAsGuest, guestRequestOf, type GuestAnswer } from "./guest.ts";

const reach = {
  clientApi: "https://discovery.example/client/v1",
  coreId: "k7xq2",
  fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw",
  clientOrigin: "https://client.example",
  locator: "the-senders-locator",
};

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

/** The core's welcome on the live connection: the person's devices connected, the guest's among them. */
const welcomeText = JSON.stringify({
  type: "devices",
  deviceIds: [2, 9],
  inFrontDeviceIds: [2],
  receivingDeviceIds: [9],
});

/** A video element that plays nothing. */
const video = {
  addEventListener: () => undefined,
  currentTime: 0,
  paused: true,
  seeking: false,
  readyState: 0,
} as unknown as HTMLVideoElement;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("a sender's launch under a grant (D465 §2)", () => {
  it("is a grant and its core's reach, which holds no session", () => {
    expect(guestRequestOf({ type: "guest", grant: "g", reach })).toEqual({
      grant: "g",
      reach,
      showsLog: false,
    });
    expect(guestRequestOf({ type: "guest", grant: "", reach })).toBeUndefined();
    expect(
      guestRequestOf({ type: "guest", grant: "g", reach: { ...reach, locator: "" } }),
    ).toBeUndefined();
    expect(guestRequestOf({ type: "device", grant: "g", reach })).toBeUndefined();
  });

  it("shows the log on screen only where the sender says so: an older sender, saying nothing, has it hidden", () => {
    const launch = { type: "guest", grant: "g", reach };
    expect(guestRequestOf({ ...launch, showsLog: true })?.showsLog).toBe(true);
    expect(guestRequestOf({ ...launch, showsLog: false })?.showsLog).toBe(false);
    expect(guestRequestOf({ ...launch, showsLog: "yes" })?.showsLog).toBe(false);
    expect(guestRequestOf(launch)?.showsLog).toBe(false);
  });
});

describe("a Google Cast device casting under a grant (D465 §2)", () => {
  /** A core answering the trade with device 9's session, and every other call as unavailable. */
  function guestCore(asked: string[]): (request: Request) => Promise<Response> {
    return async (request) => {
      const path = new URL(request.url).pathname;
      const signedIn = request.headers.get("Authorization") ?? "signed out";
      asked.push(`${request.method} ${path} ${signedIn} ${await request.text()}`);
      if (path.endsWith("/guest-receivers")) {
        const session = { token: "the-guests-token", locator: "the-guests-locator", deviceId: 9 };
        return new Response(JSON.stringify(session), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      // No film starts: what is tested is the cast, not the player.
      return new Response("{}", { status: 503 });
    };
  }

  it("trades its grant, answers which device it is, and takes its cast without pairing, keeping nothing", async () => {
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });
    const asked: string[] = [];
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    const answers: GuestAnswer[] = [];
    let traded: (() => HeldSession | undefined) | undefined;
    let isClosed = false;
    const stop = await castAsGuest(
      { grant: "a guest grant", reach, showsLog: false },
      {
        reach: (tradedSession) => {
          traded = tradedSession;
          return Promise.resolve({
            fetch: guestCore(asked),
            openSocket: (listeners) => {
              sockets.push(listeners);
              return { send: () => undefined, close: () => undefined };
            },
            close: () => {
              isClosed = true;
            },
          });
        },
        video,
        note: () => undefined,
        answer: (answer) => answers.push(answer),
        onTaken: () => undefined,
        goHome: () => undefined,
      },
    );
    // Not ready at the trade: a cast to it is refused until the core has welcomed its connection.
    expect(answers).toEqual([]);
    expect(asked).toEqual(['POST /api/v1/guest-receivers signed out {"grant":"a guest grant"}']);
    // Its later offers are proven by the session the trade gave it.
    expect(traded?.()).toEqual({
      core: { coreId: "k7xq2", fingerprint: reach.fingerprint },
      token: "the-guests-token",
      locator: "the-guests-locator",
    });
    const [live] = sockets;
    live?.onOpen();
    expect(answers).toEqual([]);
    live?.onMessage(welcomeText);
    expect(answers).toEqual([{ type: "ready", deviceId: 9 }]);
    live?.onMessage(welcomeText);
    expect(answers).toHaveLength(1);
    live?.onMessage(receiveText(1));
    await vi.waitFor(() => {
      expect(asked).toContain("POST /api/v1/me/casts/1/accept Bearer the-guests-token ");
    });
    // The sender stops the cast; the core removes the guest's device.
    sockets[1]?.onMessage(JSON.stringify({ type: "ended", reason: "stopped" }));
    stop?.();
    expect(isClosed).toBe(true);
    expect(asked.some((each) => each.includes("/pairings"))).toBe(false);
    expect([...storage.keys()]).toEqual([]);
  });

  it("answers that it failed where the core refuses its live connection before welcoming it", async () => {
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    const answers: GuestAnswer[] = [];
    let isClosed = false;
    await castAsGuest(
      { grant: "a guest grant", reach, showsLog: false },
      {
        reach: () =>
          Promise.resolve({
            fetch: guestCore([]),
            openSocket: (listeners) => {
              sockets.push(listeners);
              return { send: () => undefined, close: () => undefined };
            },
            close: () => {
              isClosed = true;
            },
          }),
        video,
        note: () => undefined,
        answer: (answer) => answers.push(answer),
        onTaken: () => undefined,
        goHome: () => undefined,
      },
    );
    sockets[0]?.onOpen();
    sockets[0]?.onMessage(JSON.stringify({ type: "signedOut" }));
    expect(answers).toHaveLength(1);
    expect(answers[0]?.type).toBe("failed");
    expect(isClosed).toBe(true);
  });

  it("answers why where the trade is refused, and closes its way to the core", async () => {
    const answers: GuestAnswer[] = [];
    let isClosed = false;
    const stop = await castAsGuest(
      { grant: "a traded grant", reach, showsLog: false },
      {
        reach: () =>
          Promise.resolve({
            fetch: () =>
              Promise.resolve(
                new Response(JSON.stringify({ title: "the grant is not valid" }), {
                  status: 401,
                  headers: { "Content-Type": "application/problem+json" },
                }),
              ),
            openSocket: () => ({ send: () => undefined, close: () => undefined }),
            close: () => {
              isClosed = true;
            },
          }),
        video,
        note: () => undefined,
        answer: (answer) => answers.push(answer),
        onTaken: () => undefined,
        goHome: () => undefined,
      },
    );
    expect(stop).toBeUndefined();
    expect(answers).toHaveLength(1);
    expect(answers[0]?.type).toBe("failed");
    expect(isClosed).toBe(true);
  });
});

describe("a Google Cast device taking the part of a Watch Together guest by link (D518)", () => {
  /** The capabilities a Watch Together guest's membership holds (x-orpheum-capability). */
  const membersCapabilities = new Set(["member", "none", "media"]);

  /** Each route a guest's membership may call, as its method and a pattern its path matches. */
  function membersRoutes(): readonly RegExp[] {
    const document: unknown = JSON.parse(
      readFileSync(path.join(import.meta.dirname, "../../../../api/openapi.json"), "utf8"),
    );
    const routes: string[] = [];
    for (const [template, operations] of Object.entries(
      Reflect.get(document as object, "paths") as object,
    )) {
      for (const [method, operation] of Object.entries(operations as object)) {
        const capability: unknown = Reflect.get(operation as object, "x-orpheum-capability");
        if (typeof capability === "string" && membersCapabilities.has(capability)) {
          routes.push(`${method.toUpperCase()} ${template}`);
        }
      }
    }
    return routes.map((route) => new RegExp(`^${route.replaceAll(/\{[^/}]+\}/gu, "[^/]+")}$`, "u"));
  }

  /**
   * A core answering the trade with the guest's membership, refusing every route a guest's
   * membership may not call (each kept in refused), and starting no film.
   */
  function membersCore(
    asked: string[],
    refused: string[],
  ): (request: Request) => Promise<Response> {
    const routes = membersRoutes();
    return async (request) => {
      const apiPath = new URL(request.url).pathname.slice(apiPrefix.length);
      const signedIn = request.headers.get("Authorization") ?? "signed out";
      asked.push(`${request.method} ${apiPath} ${signedIn} ${await request.text()}`);
      if (!routes.some((route) => route.test(`${request.method} ${apiPath}`))) {
        refused.push(`${request.method} ${apiPath}`);
        return new Response("{}", { status: 403 });
      }
      if (apiPath === "/guest-receivers") {
        const part = {
          locator: "the-memberships-locator",
          membershipCredential: "the-guests-membership",
          togetherSessionId: 1,
        };
        return new Response(JSON.stringify(part), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }
      // No film starts: what is tested is the part, not the player.
      return new Response("{}", { status: 503 });
    };
  }

  /** The session's welcome of the Chromecast, device 12 of the guest's membership, a follower. */
  const welcomeOfThePart = {
    v: 3,
    type: "welcome",
    seq: 1,
    sessionId: 1,
    memberId: 2,
    deviceId: 12,
    group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
    roster: [
      { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video", isLeader: true }] },
      {
        id: 2,
        name: "Gran",
        devices: [
          { id: 8, role: "video" },
          { id: 12, role: "video" },
        ],
      },
    ],
  };

  /** The group playing film 7, told the Chromecast as its session tells each follower. */
  function groupPlays(state: "playing" | "paused"): string {
    const group = { state, positionMs: 5000, rate: 1, fileId: 7, event: "leaderPlays" };
    return JSON.stringify({ v: 3, type: "group", seq: 2, group });
  }

  /** A cast under a Watch Together guest's grant: what it asked and answered, and where it went. */
  interface CastPart {
    readonly asked: string[];
    readonly refused: string[];
    readonly answers: GuestAnswer[];
    readonly socket: () => Parameters<OpenTogetherSocket>[0] | undefined;
    readonly traded: () => HeldSession | undefined;
    readonly wentHome: () => number;
    readonly isClosed: () => boolean;
    readonly isTaken: () => boolean;
  }

  /** Casts under a Watch Together guest's grant. */
  async function castAsTheGuestsPart(): Promise<CastPart> {
    const asked: string[] = [];
    const refused: string[] = [];
    const answers: GuestAnswer[] = [];
    const sockets: Parameters<OpenTogetherSocket>[0][] = [];
    let traded: (() => HeldSession | undefined) | undefined;
    let wentHome = 0;
    let isClosed = false;
    let isTaken = false;
    await castAsGuest(
      { grant: "a Watch Together guest's grant", reach, showsLog: false },
      {
        reach: (tradedSession) => {
          traded = tradedSession;
          return Promise.resolve({
            fetch: membersCore(asked, refused),
            openSocket: (listeners) => {
              sockets.push(listeners);
              return { send: () => undefined, close: () => undefined };
            },
            close: () => {
              isClosed = true;
            },
          });
        },
        video,
        note: () => undefined,
        answer: (answer) => answers.push(answer),
        onTaken: () => {
          isTaken = true;
        },
        goHome: () => {
          wentHome += 1;
        },
      },
    );
    return {
      asked,
      refused,
      answers,
      socket: () => sockets[0],
      traded: () => traded?.(),
      wentHome: () => wentHome,
      isClosed: () => isClosed,
      isTaken: () => isTaken,
    };
  }

  it("takes their part by their membership's credential, signed in as no one, ready once welcomed, and asks for the group's film as the guest does", async () => {
    const part = await castAsTheGuestsPart();
    expect(part.answers).toEqual([]);
    // Its later offers are proven by the membership, under the locator the trade answered.
    expect(part.traded()).toEqual({
      core: { coreId: "k7xq2", fingerprint: reach.fingerprint },
      token: "the-guests-membership",
      locator: "the-memberships-locator",
    });
    part.socket()?.onOpen();
    part.socket()?.onMessage(JSON.stringify(welcomeOfThePart));
    expect(part.answers).toEqual([{ type: "ready", deviceId: 12 }]);
    expect(part.isTaken()).toBe(true);
    part.socket()?.onMessage(groupPlays("playing"));
    const formats = JSON.stringify({ playableFormats: receiverFormats() });
    await vi.waitFor(() => {
      expect(part.asked).toContain(
        `POST /together/playback Bearer the-guests-membership ${formats}`,
      );
    });
    expect(part.wentHome()).toBe(0);
  });

  it("goes home, its part let go of, as the guest is removed or the session ends (D518 §2)", async () => {
    for (const reason of ["ejected", "hostEnded"]) {
      const part = await castAsTheGuestsPart();
      part.socket()?.onOpen();
      part.socket()?.onMessage(JSON.stringify(welcomeOfThePart));
      part.socket()?.onMessage(JSON.stringify({ v: 3, type: "ended", reason }));
      expect(part.wentHome()).toBe(1);
      expect(part.isClosed()).toBe(true);
      expect(part.answers).toHaveLength(1);
    }
  });

  it("answers that it failed where its part ends before the session welcomed it", async () => {
    const part = await castAsTheGuestsPart();
    part.socket()?.onOpen();
    part.socket()?.onMessage(JSON.stringify({ v: 3, type: "ended", reason: "ejected" }));
    expect(part.answers).toHaveLength(1);
    expect(part.answers[0]?.type).toBe("failed");
    expect(part.wentHome()).toBe(0);
    expect(part.isClosed()).toBe(true);
  });

  it("asks for no route the core refuses a guest's membership", async () => {
    const part = await castAsTheGuestsPart();
    part.socket()?.onOpen();
    part.socket()?.onMessage(JSON.stringify(welcomeOfThePart));
    part.socket()?.onMessage(groupPlays("paused"));
    await vi.waitFor(() => {
      expect(part.asked.some((each) => each.startsWith("POST /together/playback"))).toBe(true);
    });
    part.socket()?.onMessage(JSON.stringify({ v: 3, type: "ended", reason: "ejected" }));
    expect(part.asked.length).toBeGreaterThan(1);
    expect(part.refused).toEqual([]);
  });
});
