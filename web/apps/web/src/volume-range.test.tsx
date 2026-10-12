// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { fire, mount } from "./test-dom.tsx";
import { VolumeRange, volumeAt } from "./volume-range.tsx";

/** A touch event at clientX, as Safari on an iPhone sends: the test DOM has no Touch to make one with. */
function touchAt(type: "touchstart" | "touchmove", clientX: number): Event {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "touches", { value: [{ clientX }] });
  return event;
}

describe("a volume slider (the owner's iPhone, 2026-10-12)", () => {
  it("reads a touch as the volume where the finger is along it, in hundredths", () => {
    const rect = { left: 100, width: 200 };
    expect(volumeAt(150, rect, 4)).toBe(1);
    expect(volumeAt(100, rect, 1)).toBe(0);
    expect(volumeAt(50, rect, 1)).toBe(0);
    expect(volumeAt(400, rect, 1)).toBe(1);
    expect(volumeAt(133, rect, 1)).toBe(0.17);
    expect(volumeAt(133, { left: 0, width: 0 }, 1)).toBe(0);
  });

  it("moves to a touch anywhere on it, not only on its thumb, and follows the finger", async () => {
    const set: number[] = [];
    const { container, unmount } = await mount(
      <VolumeRange value={1} most={4} onChange={(value) => set.push(value)} />,
    );
    const slider = container.querySelector("input");
    slider!.getBoundingClientRect = () => new DOMRect(0, 0, 400, 44);
    const start = touchAt("touchstart", 50);
    await fire(slider, start);
    await fire(slider, touchAt("touchmove", 300));
    expect(set).toEqual([0.5, 3]);
    expect(start.defaultPrevented).toBe(true); // the page does not scroll under the finger
    unmount();
  });
});
