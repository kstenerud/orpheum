// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { listenToPads, padPollCost, padPollEveryMs } from "./input-pad-poll.ts";
import { stickRepeatEachMs, stickRepeatFirstMs } from "./input-pads.ts";

/** A standard pad, its buttons held as the test says. */
interface FakePad {
  readonly index: number;
  readonly id: string;
  readonly mapping: string;
  buttons: Pick<GamepadButton, "pressed">[];
  readonly axes: number[];
}

/** A standard pad at rest, with 17 buttons and four axes. */
function padAtRest(): FakePad {
  return {
    index: 0,
    id: "Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)",
    mapping: "standard",
    buttons: Array.from({ length: 17 }, () => ({ pressed: false })),
    axes: [0, 0, 0, 0],
  };
}

describe("the pads' poll", () => {
  let pads: FakePad[] = [];
  let stops: (() => void)[] = [];
  beforeEach(() => {
    pads = [padAtRest()];
    stops = [];
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "performance"] });
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => pads });
  });
  afterEach(() => {
    for (const stop of stops) {
      stop();
    }
    Reflect.deleteProperty(navigator, "getGamepads");
    vi.useRealTimers();
  });

  /** Listens to the pads as listener, stopped after the test. Returns the stopping. */
  function listen(listener: Parameters<typeof listenToPads>[0]): () => void {
    const stop = listenToPads(listener);
    stops.push(stop);
    return stop;
  }

  // "Poll 15x per second and we'll see if people complain" (the owner, 2026-11-07): each animation
  // frame, it cost a main-thread frame each vsync on every page while a controller was connected.
  it("reads a connected pad 15 times a second, not each frame", () => {
    let reads = 0;
    listen({
      onRead: () => {
        reads += 1;
      },
    });
    vi.advanceTimersByTime(1000);
    expect(reads).toBe(15);
  });

  it("reads the pads no more once its last listener leaves", () => {
    let reads = 0;
    const stopFirst = listen({
      onRead: () => {
        reads += 1;
      },
    });
    const stopSecond = listen({});
    vi.advanceTimersByTime(padPollEveryMs * 3);
    const readsBefore = reads;
    stopSecond();
    vi.advanceTimersByTime(padPollEveryMs * 3);
    expect(reads).toBe(readsBefore + 3); // one listener still listens
    stopFirst();
    const costBefore = padPollCost().polls;
    vi.advanceTimersByTime(1000);
    expect(padPollCost().polls).toBe(costBefore);
  });

  it("reads the pads no more once the last is gone, until one connects", () => {
    listen({});
    vi.advanceTimersByTime(padPollEveryMs * 2);
    pads = [];
    vi.advanceTimersByTime(padPollEveryMs);
    const polls = padPollCost().polls;
    vi.advanceTimersByTime(1000);
    expect(padPollCost().polls).toBe(polls);
    pads = [padAtRest()];
    globalThis.dispatchEvent(new Event("gamepadconnected"));
    vi.advanceTimersByTime(1000);
    expect(padPollCost().polls).toBe(polls + 15);
  });

  it("repeats a held seek button on its schedule though it reads the pad 66 ms apart", () => {
    const repeatsMs: number[] = [];
    listen({
      onButtonRepeat: () => {
        repeatsMs.push(performance.now());
      },
    });
    vi.advanceTimersByTime(padPollEveryMs);
    const pressedAtMs = performance.now() + padPollEveryMs;
    const pad = pads[0];
    if (pad !== undefined) {
      pad.buttons[7] = { pressed: true }; // the right trigger: fast forward
    }
    const heldMs = 2000;
    vi.advanceTimersByTime(heldMs);
    const polledMs = Math.floor(heldMs / padPollEveryMs) * padPollEveryMs;
    // One repeat each interval after the first wait, by the time held, each at the first poll on or
    // after its time: not at every second poll, 133 ms apart, which would make a quarter fewer.
    const expected =
      Math.floor((polledMs - padPollEveryMs - stickRepeatFirstMs) / stickRepeatEachMs) + 1;
    expect(repeatsMs).toHaveLength(expected);
    for (const [at, repeatMs] of repeatsMs.entries()) {
      const dueMs = pressedAtMs + stickRepeatFirstMs + at * stickRepeatEachMs;
      expect(repeatMs).toBeGreaterThanOrEqual(dueMs);
      expect(repeatMs).toBeLessThan(dueMs + padPollEveryMs);
    }
  });

  it("counts its polls and the time spent in them, for the button test to show", () => {
    const before = padPollCost();
    listen({});
    let nowMs = 0;
    const spy = vi.spyOn(performance, "now").mockImplementation(() => {
      nowMs += 0.25;
      return nowMs;
    });
    vi.advanceTimersByTime(padPollEveryMs * 4);
    spy.mockRestore();
    const after = padPollCost();
    expect(after.polls - before.polls).toBe(4);
    expect(after.spentMs - before.spentMs).toBeCloseTo(4 * 0.25);
  });
});
