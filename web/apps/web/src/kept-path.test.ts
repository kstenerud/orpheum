// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { Reached } from "./discovery-start.ts";
import { betterPathFirstMs, keepPath } from "./kept-path.ts";

afterEach(() => {
  vi.useRealTimers();
});

describe("the kept path", () => {
  it("tells each move with the name of the path it went to, for the play report beside it (the owner, 2026-10-17)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
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
    expect(told).toEqual([`the core's name, ${home.origin}`]);
  });

  it("looks for a better path half as often after each move away from one that failed (the owner's session, 2026-10-18)", async () => {
    vi.useFakeTimers();
    const away: Reached = { kind: "direct", origin: "https://2a01-db8--1.core1.orpheum-core.com" };
    const home: Reached = {
      kind: "direct",
      origin: "https://192-168-1-2.core1.orpheum-core.com:8443",
    };
    // Firefox under the page's service worker: the LAN name answers the look, then fails a request.
    const realFetch = globalThis.fetch;
    globalThis.fetch = (input: RequestInfo | URL): Promise<Response> =>
      String(input instanceof Request ? input.url : input).startsWith(home.origin)
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : Promise.resolve(new Response("{}"));
    const kept = keepPath(
      {
        first: away,
        another: () => Promise.resolve(away),
        better: (current) => Promise.resolve(current === away ? home : undefined),
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
    await vi.advanceTimersByTimeAsync(3 * betterPathFirstMs);
    expect(told).toEqual(["home", "away", "home", "away"]); // four times as long
    await vi.advanceTimersByTimeAsync(betterPathFirstMs);
    expect(told).toEqual(["home", "away", "home", "away", "home"]);
    kept.close();
    globalThis.fetch = realFetch;
  });
});
