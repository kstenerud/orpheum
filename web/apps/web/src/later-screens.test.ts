// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { createElement } from "react";
import { describe, expect, it } from "vitest";

import * as laterScreens from "./later-screens.tsx";
import { resetLazyScreensForTests } from "./later-screens.tsx";
import { mount } from "./test-dom.tsx";

/** Every screen's exported name, read fresh each time so a snapshot is the reference at that call. */
const screenNames = [
  "Title",
  "Series",
  "Play",
  "TogetherViewer",
  "Lab",
  "LabExtendPlayer",
  "Settings",
  "UiGallery",
] as const;

describe("resetLazyScreensForTests (the shared module state note, test-architecture-2026-10-26.md)", () => {
  it("gives every screen a fresh React.lazy wrapper, not the one an earlier test already resolved", () => {
    const before = screenNames.map((name) => laterScreens[name]);
    resetLazyScreensForTests();
    const after = screenNames.map((name) => laterScreens[name]);
    for (const [index, name] of screenNames.entries()) {
      expect(after[index], `${name} should be a new wrapper`).not.toBe(before[index]);
    }
  });

  it("is called by test-dom's mount(), so a test never passes only because an earlier one in its file warmed the same screen", async () => {
    const before = laterScreens.Title;
    const mounted = await mount(createElement("div"));
    expect(laterScreens.Title).not.toBe(before);
    mounted.unmount();
  });
});
