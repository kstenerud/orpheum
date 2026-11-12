// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import type { HeldSession, OpenTogetherSocket } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

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
      },
    );
    expect(stop).toBeUndefined();
    expect(answers).toHaveLength(1);
    expect(answers[0]?.type).toBe("failed");
    expect(isClosed).toBe(true);
  });
});
