// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pathMovesHeardFrom } from "./services.tsx";

describe("the path moves a loader tells of", () => {
  it("counts each, keeps the name of the path it went to, and tells the screens (the owner, 2026-10-17)", () => {
    let tell: ((to?: string) => void) | undefined;
    const pathMoves = pathMovesHeardFrom((listener) => {
      tell = listener;
      return () => undefined;
    });
    let heard = 0;
    pathMoves.subscribe(() => {
      heard += 1;
    });
    expect([pathMoves.count(), pathMoves.to()]).toEqual([0, undefined]);
    tell?.("WebRTC");
    expect([pathMoves.count(), pathMoves.to(), heard]).toEqual([1, "WebRTC", 1]);
    // An older loader names no path.
    tell?.();
    expect([pathMoves.count(), pathMoves.to(), heard]).toEqual([2, undefined, 2]);
  });
});
