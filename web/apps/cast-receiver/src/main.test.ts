// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from "vitest";

import mainSource from "./main.ts?raw";

const ownToSorted = Reflect.getOwnPropertyDescriptor(Array.prototype, "toSorted");

afterEach(() => {
  vi.useRealTimers();
  if (ownToSorted !== undefined) {
    Reflect.defineProperty(Array.prototype, "toSorted", ownToSorted);
  }
});

/** The modules a source imports as it loads, in order: its static imports, never `import type`. */
function staticImportsOf(source: string): string[] {
  const imports = source.matchAll(/^import\s+(?!type\s)(?:[^;]*?\sfrom\s+)?"([^"]+)";/gmsu);
  return [...imports].map((each) => each[1] ?? "");
}

describe("the receiver's entry point", () => {
  it("fills what Chromium 90 lacks before anything it imports is loaded", async () => {
    // ES modules are evaluated in the order they are imported, so the fill runs before the rest only
    // as main.ts's first import; the lab's own modules may not be mocked to watch it (web lint).
    expect(staticImportsOf(mainSource)[0]).toBe("./old-engine.ts");
    vi.useFakeTimers();
    // As on the 3rd-generation Chromecast's Chromium 90, which has no toSorted (Chrome 110).
    Reflect.deleteProperty(Array.prototype, "toSorted");
    await import("./main.ts");
    expect("toSorted" in Array.prototype).toBe(true);
    expect([3, 1, 2].toSorted()).toEqual([1, 2, 3]);
  });

  it("makes no report at its start, whose WebRTC check would play a tone through the TV", async () => {
    // The owner, 2026-11-12: a cast started with a trill of some seconds, the report's tone.
    vi.resetModules();
    document.body.replaceChildren();
    await import("./main.ts");
    const log = document.querySelector(".receiver__log");
    expect(log?.textContent).toContain(navigator.userAgent);
    expect(log?.textContent).not.toContain("making the report");
  });
});
