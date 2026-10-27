// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { enter, fire, mount, settleUntil, type Mounted } from "./test-dom.tsx";
import { clockText, Timeline } from "./timeline.tsx";

/** A touch event at clientX, as Safari on an iPhone sends: the test DOM has no Touch to make one with. */
function touchAt(type: "touchstart" | "touchmove", clientX: number): Event {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "touches", { value: [{ clientX }] });
  return event;
}

/** Moves a range to value as a drag does: its value set, then its input event. */
async function dragTo(range: HTMLInputElement, value: number): Promise<void> {
  await enter(range, String(value));
}

describe("a film's timeline (D369 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The timeline drawn, and its slider. */
  async function drawn(element: React.JSX.Element): Promise<HTMLInputElement> {
    mounted = await mount(element);
    const range = mounted.container.querySelector<HTMLInputElement>('input[type="range"]');
    if (range === null) {
      throw new Error("no slider");
    }
    return range;
  }

  it("is a slider in milliseconds, named, saying where the film stands and its length", async () => {
    const range = await drawn(
      <Timeline label="Where" atMs={61_000} durationMs={600_000} onSeek={() => undefined} />,
    );
    expect(range.max).toBe("600000");
    expect(range.value).toBe("61000");
    expect(range.getAttribute("aria-label")).toBe("Where");
    expect(range.getAttribute("aria-valuetext")).toBe("1:01");
    expect(mounted?.container.textContent).toBe("1:01 / 10:00");
    expect(range.hasAttribute("data-focus-arrows-are-commands")).toBe(false);
  });

  it("says where alone while the film's length is not known", async () => {
    await drawn(<Timeline label="Where" atMs={5000} durationMs={0} onSeek={() => undefined} />);
    expect(mounted?.container.textContent).toBe("0:05");
  });

  it("seeks at each move of a drag, unless told to wait for it to settle", async () => {
    const seeks: number[] = [];
    const range = await drawn(
      <Timeline label="Where" atMs={0} durationMs={600_000} onSeek={(ms) => seeks.push(ms)} />,
    );
    await dragTo(range, 120_000);
    await dragTo(range, 180_000);
    expect(seeks).toEqual([120_000, 180_000]);
  });

  it("seeks once a drag has rested for settleMs, showing where it is meanwhile", async () => {
    const seeks: number[] = [];
    const range = await drawn(
      <Timeline
        label="Where"
        atMs={0}
        durationMs={600_000}
        settleMs={50}
        onSeek={(ms) => seeks.push(ms)}
      />,
    );
    await dragTo(range, 120_000);
    await dragTo(range, 180_000);
    expect(seeks).toEqual([]);
    expect(mounted?.container.textContent).toBe("3:00 / 10:00");
    await settleUntil(() => seeks.length > 0);
    expect(seeks).toEqual([180_000]);
  });

  it("moves to a touch anywhere on it, not only on its thumb, and follows the finger", async () => {
    const seeks: number[] = [];
    const range = await drawn(
      <Timeline label="Where" atMs={0} durationMs={600_000} onSeek={(ms) => seeks.push(ms)} />,
    );
    range.getBoundingClientRect = () => new DOMRect(100, 0, 400, 44);
    const start = touchAt("touchstart", 200);
    await fire(range, start);
    await fire(range, touchAt("touchmove", 400));
    expect(seeks).toEqual([150_000, 450_000]);
    expect(start.defaultPrevented).toBe(true); // the page does not scroll under the finger
  });

  it("leaves its Left and Right to the player on a television", async () => {
    const range = await drawn(
      <Timeline
        label="Where"
        atMs={0}
        durationMs={600_000}
        hasArrowsAsCommands
        onSeek={() => undefined}
      />,
    );
    expect(range.hasAttribute("data-focus-arrows-are-commands")).toBe(true);
  });

  it("says a film's times as a player does", () => {
    expect(clockText(0)).toBe("0:00");
    expect(clockText(61_999)).toBe("1:01");
    expect(clockText(3_725_000)).toBe("1:02:05");
  });
});
