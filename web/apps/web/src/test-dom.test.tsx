// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { settleUntil } from "./test-dom.tsx";

describe("settleUntil, the wait for what outlasts a page's answers", () => {
  it("returns once its condition holds", async () => {
    let turns = 0;
    await settleUntil(() => {
      turns += 1;
      return turns > 3;
    });
    expect(turns).toBeGreaterThan(3);
  });

  it("fails the test, naming the condition, when it never holds (the audit of 2026-10-25, W11)", async () => {
    const isShown = false;
    await expect(settleUntil(() => isShown)).rejects.toThrow(
      /settleUntil: this never held within about 2 s: .*isShown/,
    );
  });
});
