// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { Reached } from "./discovery-start.ts";
import { betterPathFirstMs, betterPathMostMs, keepPath } from "./kept-path.ts";

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
    // Every path answers, the request that confirms the better one too; no network is reached.
    const realFetch = globalThis.fetch;
    globalThis.fetch = (): Promise<Response> => Promise.resolve(new Response("{}"));
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
});
