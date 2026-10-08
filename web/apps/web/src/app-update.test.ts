// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createUpdateWatch, ownScriptOf } from "./app-update.ts";

describe("a page out of date", () => {
  it("is one whose own script an answer names otherwise, and only then", () => {
    const watch = createUpdateWatch();
    let told = 0;
    watch.subscribe(() => {
      told += 1;
    });
    watch.noteServed("index-old.js", "index-old.js");
    watch.noteServed("index-old.js", null);
    expect(watch.isOutdated()).toBe(false);
    watch.noteServed("index-old.js", "index-new.js");
    watch.noteServed("index-old.js", "index-newer.js");
    expect(watch.isOutdated()).toBe(true);
    expect(told).toBe(1);
  });

  it("is never a development page, which has no built script", () => {
    const watch = createUpdateWatch();
    watch.noteServed(undefined, "index-new.js");
    expect(watch.isOutdated()).toBe(false);
  });

  it("knows its own script from the page's entry", () => {
    document.head.innerHTML =
      '<script type="module" crossorigin src="/assets/index-DCLi77_b.js"></script>';
    expect(ownScriptOf(document)).toBe("index-DCLi77_b.js");
    document.head.innerHTML = '<script type="module" src="/src/main.tsx"></script>';
    expect(ownScriptOf(document)).toBeUndefined();
  });
});
