// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  forgetSteps,
  noteStep,
  pageLoadedAgain,
  stepsKey,
  stepsMost,
  stepsSoFar,
} from "./steps.ts";

/** A tab's session storage, as the browser's keeps it. */
function tabStorage(): Pick<Storage, "getItem" | "setItem" | "removeItem"> {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

beforeEach(() => {
  vi.stubGlobal("sessionStorage", tabStorage());
});

afterEach(() => {
  forgetSteps();
  vi.resetModules();
  vi.unstubAllGlobals();
});

describe("the steps a page took to reach its core", () => {
  it("are kept in order, each with its time, and only the newest stepsMost", () => {
    noteStep("found the server");
    noteStep("connected over WebRTC");
    expect(stepsSoFar()).toHaveLength(2);
    expect(stepsSoFar()[0]).toMatch(/^\d+ ms found the server$/);
    expect(stepsSoFar()[1]).toMatch(/^\d+ ms connected over WebRTC$/);
    for (let each = 0; each < stepsMost * 2; each += 1) {
      noteStep(`step ${String(each)}`);
    }
    expect(stepsSoFar()).toHaveLength(stepsMost);
    expect(stepsSoFar().at(-1)).toMatch(new RegExp(`step ${String(stepsMost * 2 - 1)}$`));
  });

  it("outlive a reload of the tab, marked where the page loaded again", async () => {
    noteStep("the service worker did not take the page over");
    expect(sessionStorage.getItem(stepsKey)).toContain("did not take the page over");
    vi.resetModules();
    const reloaded = await import("./steps.ts");
    reloaded.noteStep("found the server");
    const steps = reloaded.stepsSoFar();
    expect(steps[0]).toMatch(/did not take the page over$/);
    expect(steps[1]).toBe(pageLoadedAgain);
    expect(steps[2]).toMatch(/found the server$/);
  });
});
