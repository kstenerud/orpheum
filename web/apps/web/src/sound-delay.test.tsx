// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { browserStorage } from "./browser-storage.ts";
import { keptSoundDelayMs, SoundDelayControl, soundDelayKey } from "./sound-delay.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  browserStorage().removeItem(soundDelayKey);
});

describe("this device's sound delay (the owner's TCL TV, 2026-10-24)", () => {
  it("starts as the browser keeps it, and keeps each step pressed", async () => {
    browserStorage().setItem(soundDelayKey, "550");
    mounted = await mount(<SoundDelayControl videoRef={createRef()} />);
    expect(mounted.container.textContent).toContain("Sound delay on this device: 550 ms");
    const more = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "+50 ms",
    );
    await fire(more ?? null, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.textContent).toContain("Sound delay on this device: 600 ms");
    expect(browserStorage().getItem(soundDelayKey)).toBe("600");
  });

  it("keeps none for what is not a delay", () => {
    for (const kept of ["-50", "abc", "99999", "12.5"]) {
      browserStorage().setItem(soundDelayKey, kept);
      expect(keptSoundDelayMs()).toBe(0);
    }
  });
});
