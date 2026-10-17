// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { Reached } from "./discovery-start.ts";
import { keepPath } from "./kept-path.ts";

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
});
