// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { RtcTransport } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Reached } from "./discovery-start.ts";
import { betterPathFirstMs, betterPathMostMs, keepPath } from "./kept-path.ts";
import { pairsSettleMs, type PathChoice } from "./path-choice.ts";

afterEach(() => {
  vi.useRealTimers();
});

describe("the kept path", () => {
  it("tells the core how its first path was chosen, over it, once the page is signed in (D354 §5)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const asked: { url: string; authorization: string | null; body: string }[] = [];
    const realFetch = globalThis.fetch;
    globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      const url = String(input instanceof Request ? input.url : input);
      asked.push({
        url,
        authorization: new Headers(init?.headers).get("Authorization"),
        body: init?.body instanceof ArrayBuffer ? new TextDecoder().decode(init.body) : "",
      });
      return Promise.resolve(new Response(null, { status: 204 }));
    };
    const choice: PathChoice = {
      browser: "chrome",
      isLanNameUsed: true,
      localNetwork: "granted",
      directOffered: 1,
      lanOffered: 0,
      names: [{ name: away.origin, outcome: "answered" }],
      reached: `direct ${away.origin}`,
    };
    const signedIn: { token?: string } = {};
    const kept = keepPath(
      {
        first: away,
        choice,
        another: () => Promise.resolve(away),
        better: () => Promise.resolve(undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => signedIn.token },
    );
    kept.carried.catch(() => undefined);
    signedIn.token = "a-token";
    await vi.advanceTimersByTimeAsync(pairsSettleMs);
    kept.close();
    globalThis.fetch = realFetch;
    const told = asked.filter((each) => each.url.endsWith("/api/v1/client-paths"));
    expect(told.map((each) => [each.url, each.authorization])).toEqual([
      [`${away.origin}/api/v1/client-paths`, "Bearer a-token"],
    ]);
    expect(JSON.parse(told[0]?.body ?? "{}")).toEqual(choice);
  });

  it("tells each move with the name of the path it went to, for the play report beside it (the owner, 2026-10-17)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    // Every path answers, the request that confirms the better one too; no network is reached.
    const realFetch = globalThis.fetch;
    globalThis.fetch = (): Promise<Response> => Promise.resolve(new Response("{}"));
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => undefined },
    );
    kept.carried.catch(() => undefined);
    const told: string[] = [];
    kept.onPathMove((to) => {
      told.push(to);
    });
    await vi.advanceTimersByTimeAsync(30_000);
    kept.close();
    globalThis.fetch = realFetch;
    expect(told).toEqual([`the core's name, ${home.origin}`]);
  });

  it("looks for a better path half as often after each move away from one that failed, and gives one up after two (2026-10-18, 2026-10-20)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    // A LAN name that answers the look and the request that confirms it, then fails a request.
    const realFetch = globalThis.fetch;
    globalThis.fetch = (input: RequestInfo | URL): Promise<Response> => {
      const url = String(input instanceof Request ? input.url : input);
      return url.startsWith(home.origin) && !url.endsWith("/api/v1/health")
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : Promise.resolve(new Response("{}"));
    };
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => undefined },
    );
    kept.carried.catch(() => undefined);
    const told: string[] = [];
    kept.onPathMove((to) => {
      told.push(to === `the core's name, ${home.origin}` ? "home" : "away");
    });
    const failOnHome = async (): Promise<void> => {
      await kept.fetch(new Request(`${home.origin}/api/v1/me`));
    };
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    await failOnHome(); // moved home, failed, and back
    expect(told).toEqual(["home", "away"]);
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(told).toEqual(["home", "away"]); // not yet: it waits twice as long now
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(told).toEqual(["home", "away", "home"]);
    await failOnHome();
    expect(told).toEqual(["home", "away", "home", "away"]);
    // Failed there twice, the LAN name is moved to no more in this page, however long it waits.
    await vi.advanceTimersByTimeAsync(4 * betterPathMostMs);
    expect(told).toEqual(["home", "away", "home", "away"]);
    kept.close();
    globalThis.fetch = realFetch;
  });

  it("never moves to a better path that fails a request carrying the session, as Firefox's LAN name does under Local Network Access (2026-10-20)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    // The LAN name answers the bare look, but fails any request with the session's header, which
    // needs a preflight; the confirming request carries it.
    const confirming: string[] = [];
    const realFetch = globalThis.fetch;
    globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      const url = String(input instanceof Request ? input.url : input);
      const authorization = new Headers(init?.headers).get("Authorization");
      if (url.startsWith(home.origin)) {
        confirming.push(authorization ?? "none");
        return Promise.reject(new TypeError("NetworkError when attempting to fetch resource."));
      }
      return Promise.resolve(new Response("{}"));
    };
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => "a-session" },
    );
    kept.carried.catch(() => undefined);
    const told: string[] = [];
    kept.onPathMove((to) => {
      told.push(to);
    });
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(told).toEqual([]);
    expect(confirming).toEqual(["Bearer a-session"]);
    // It looks again twice as long after, not at the first wait.
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(confirming).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(confirming).toHaveLength(2);
    expect(told).toEqual([]);
    kept.close();
    globalThis.fetch = realFetch;
  });

  it("looks for a better path at once when the browser's local-network permission is granted (W2c item 1)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    const realFetch = globalThis.fetch;
    globalThis.fetch = (): Promise<Response> => Promise.resolve(new Response("{}"));
    let granted = (): void => undefined;
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
        onLocalNetworkGranted: (listener) => {
          granted = listener;
          return () => undefined;
        },
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => undefined },
    );
    kept.carried.catch(() => undefined);
    const told: string[] = [];
    kept.onPathMove((to) => {
      told.push(to);
    });
    granted();
    await vi.advanceTimersByTimeAsync(0);
    expect(told).toEqual([`the core's name, ${home.origin}`]);
    kept.close();
    globalThis.fetch = realFetch;
  });

  it("closes a WebRTC transport it moved away from when it closes, not only the one in use (staging, 2026-10-25)", async () => {
    vi.useFakeTimers();
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    const peer = closingPeer();
    const webrtc: Reached = { kind: "webrtc", transport: new RtcTransport([peer]) };
    const realFetch = globalThis.fetch;
    globalThis.fetch = (): Promise<Response> => Promise.resolve(new Response("{}"));
    const kept = keepPath(
      {
        first: webrtc,
        another: () => Promise.resolve(webrtc),
        better: (current) => Promise.resolve(current === webrtc ? home : undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => undefined },
    );
    kept.carried.catch(() => undefined);
    const told: string[] = [];
    kept.onPathMove((to) => {
      told.push(to);
    });
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(told).toEqual([`the core's name, ${home.origin}`]);
    expect(peer.isClosed).toBe(false);
    kept.close();
    globalThis.fetch = realFetch;
    expect(peer.isClosed).toBe(true);
  });

  it("closes its WebRTC transport when the page leaves for good, and keeps it for the back button", () => {
    const kept = (peer: ReturnType<typeof closingPeer>): ReturnType<typeof keepPath> => {
      const webrtc: Reached = { kind: "webrtc", transport: new RtcTransport([peer]) };
      const path = keepPath(
        {
          first: webrtc,
          another: () => Promise.resolve(webrtc),
          better: () => Promise.resolve(undefined),
          onLocalNetworkGranted: () => () => undefined,
          onLocalNetworkAsking: () => () => undefined,
        },
        { token: () => undefined },
      );
      path.carried.catch(() => undefined);
      return path;
    };
    const cached = closingPeer();
    const keptForBack = kept(cached);
    window.dispatchEvent(Object.assign(new Event("pagehide"), { persisted: true }));
    expect(cached.isClosed).toBe(false);
    keptForBack.close();
    const left = closingPeer();
    kept(left);
    window.dispatchEvent(Object.assign(new Event("pagehide"), { persisted: false }));
    expect(left.isClosed).toBe(true);
  });
});

/** A peer connection that only says whether it was closed. */
function closingPeer(): {
  isClosed: boolean;
  createDataChannel: () => never;
  close: () => void;
} {
  const peer = {
    isClosed: false,
    createDataChannel: (): never => {
      throw new Error("no channel is opened in this test");
    },
    close: (): void => {
      peer.isClosed = true;
    },
  };
  return peer;
}
