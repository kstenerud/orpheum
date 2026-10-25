// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { reachedFrom } from "./test-walk.tsx";

describe("a walk with a remote's keys", () => {
  it("presses nothing once its page is gone, as when its test timed out (2026-10-25)", async () => {
    const pressed: string[] = [];
    const press = async (keys: string): Promise<void> => {
      pressed.push(keys);
      await Promise.resolve();
    };
    const gone = document.createElement("button");
    expect(await reachedFrom(gone, press)).toEqual(new Set([gone]));
    expect(pressed).toEqual([]);
  });

  it("presses each arrow from where it starts while its page is there", async () => {
    const pressed: string[] = [];
    const press = async (keys: string): Promise<void> => {
      pressed.push(keys);
      await Promise.resolve();
    };
    const here = document.createElement("button");
    document.body.append(here);
    await reachedFrom(here, press);
    here.remove();
    expect(pressed).toEqual(["{ArrowUp}", "{ArrowDown}", "{ArrowLeft}", "{ArrowRight}"]);
  });
});
