// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  keyRepeatsWithinMs,
  onCommand,
  onCommandRelease,
  startInput,
  type Input,
} from "./input-commands.ts";
import { answerPlayerCommand } from "./player-commands.ts";
import { seekRestMs, seekStepMs } from "./seek-ahead.ts";

/**
 * A standard pad as navigator.getGamepads gives it, its left stick leaned and its buttons held as
 * the test says.
 */
interface FakePad {
  readonly index: number;
  readonly id: string;
  readonly mapping: string;
  buttons: Pick<GamepadButton, "pressed">[];
  axes: number[];
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

/** How far the test's clock moves for one of the page's frames, in ms. */
const frameMs = 16;

/** Runs the page's frames for ms, in each of which it polls its pads once. */
function frames(ms: number): void {
  vi.advanceTimersByTime(ms);
}

/** Each command offered and each release told, as `<command> <hold>` and `<command> released (<source>)`. */
function heardHolds(): { readonly heard: string[]; readonly stop: () => void } {
  const heard: string[] = [];
  const stopCommands = onCommand(document, (detail) => {
    heard.push(`${detail.command} ${detail.hold ?? "pressed"}`);
  });
  const stopReleases = onCommandRelease(window, (release) => {
    heard.push(`${release.command} released (${release.source})`);
  });
  return {
    heard,
    stop: () => {
      stopCommands();
      stopReleases();
    },
  };
}

/** Sends a key's event of type at the body, as a remote's reaches the page. */
function key(type: "keydown" | "keyup", init: KeyboardEventInit): void {
  document.body.dispatchEvent(
    new KeyboardEvent(type, { ...init, bubbles: true, cancelable: true }),
  );
}

// The owner, 2026-11-04 (navigation-and-widgets-answers-2026-10-27.md, the System and player agents'
// answers, item 4): drivers turn each device's events into one API, pressed and held, and the UI
// knows nothing of any device's quirks.
describe("holds, told alike by every device's driver", () => {
  let pads: FakePad[] = [];
  let input: Input | undefined;
  let holds: ReturnType<typeof heardHolds> | undefined;
  beforeEach(() => {
    pads = [];
    vi.useFakeTimers({
      toFake: [
        "requestAnimationFrame",
        "cancelAnimationFrame",
        "performance",
        "setTimeout",
        "clearTimeout",
      ],
    });
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => pads });
    input = startInput({ goBack: () => false, isNavigating: () => false });
    holds = heardHolds();
  });
  afterEach(() => {
    holds?.stop();
    input?.stop();
    Reflect.deleteProperty(navigator, "getGamepads");
    vi.useRealTimers();
    document.body.replaceChildren();
  });

  /** Connects pad: the page is told, and polls it from the next frame. */
  function connect(pad: FakePad): void {
    pads = [pad];
    globalThis.dispatchEvent(new Event("gamepadconnected"));
    frames(frameMs);
  }

  it("starts a key's hold as it goes down, continues it at each repeat, marked or not, and releases it as it goes up", () => {
    key("keydown", { key: "ArrowRight", keyCode: 39 });
    key("keydown", { key: "ArrowRight", keyCode: 39 }); // Silk's repeat, unmarked (VQ122)
    key("keydown", { key: "ArrowRight", keyCode: 39, repeat: true });
    key("keyup", { key: "ArrowRight", keyCode: 39 });
    key("keydown", { key: "ArrowRight", keyCode: 39 });
    expect(holds?.heard).toEqual([
      "right start",
      "right continue",
      "right continue",
      "right released (key)",
      "right start",
    ]);
  });

  it("starts a key's hold again where its keyup was lost: no press is a repeat for good", () => {
    key("keydown", { key: "Enter", keyCode: 13 });
    frames(keyRepeatsWithinMs);
    key("keydown", { key: "Enter", keyCode: 13 }); // a slow repeat, still held
    frames(keyRepeatsWithinMs + 1);
    key("keydown", { key: "Enter", keyCode: 13 }); // its keyup never came
    expect(holds?.heard).toEqual(["select start", "select continue", "select start"]);
  });

  it("releases every key held when the page loses the keys, whose keyups it will not hear", () => {
    key("keydown", { key: "ArrowLeft", keyCode: 37 });
    globalThis.dispatchEvent(new Event("blur"));
    key("keyup", { key: "ArrowLeft", keyCode: 37 });
    expect(holds?.heard).toEqual(["left start", "left released (key)"]);
  });

  it("starts a controller's stick's hold as it leans, continues it as it repeats, and releases it at its centre", () => {
    const pad = padAtRest();
    connect(pad);
    pad.axes = [1, 0, 0, 0];
    frames(600);
    pad.axes = [0, 0, 0, 0];
    frames(frameMs);
    expect(holds?.heard).toEqual([
      "right start",
      "right continue", // its first repeat, 400 ms on
      "right continue", // and each 100 ms after
      "right released (pad)",
    ]);
  });

  it("releases a stick turned the other way before starting the new way, and a pad gone with its stick held", () => {
    const pad = padAtRest();
    connect(pad);
    pad.axes = [1, 0, 0, 0];
    frames(frameMs);
    pad.axes = [-1, 0, 0, 0];
    frames(frameMs);
    pads = [];
    globalThis.dispatchEvent(new Event("gamepaddisconnected"));
    frames(frameMs);
    expect(holds?.heard).toEqual([
      "right start",
      "right released (pad)",
      "left start",
      "left released (pad)",
    ]);
  });

  it("continues a controller's seek button's hold at the stick's delay and interval, and releases it as it goes up", () => {
    const pad = padAtRest();
    connect(pad);
    pad.buttons[7] = { pressed: true }; // the right trigger: fast forward
    frames(600);
    pad.buttons[7] = { pressed: false };
    frames(frameMs);
    expect(holds?.heard).toEqual([
      "seekForward start",
      "seekForward continue", // its first repeat, 400 ms on
      "seekForward continue", // and each 100 ms after
      "seekForward released (pad)",
    ]);
  });

  it("never repeats a controller's OK, Back or Start held, which must not act twice", () => {
    const pad = padAtRest();
    connect(pad);
    pad.buttons[0] = { pressed: true }; // the bottom face button: OK on an Xbox pad
    pad.buttons[1] = { pressed: true }; // the right one: Back
    pad.buttons[9] = { pressed: true }; // Start: play and pause
    frames(1000);
    pad.buttons[0] = { pressed: false };
    pad.buttons[1] = { pressed: false };
    pad.buttons[9] = { pressed: false };
    frames(frameMs);
    expect(holds?.heard.filter((each) => each.endsWith("continue"))).toEqual([]);
    expect(holds?.heard).toHaveLength(6);
  });

  it("releases a seek button held when its pad is gone", () => {
    const pad = padAtRest();
    connect(pad);
    pad.buttons[6] = { pressed: true }; // the left trigger: rewind
    frames(frameMs);
    pads = [];
    globalThis.dispatchEvent(new Event("gamepaddisconnected"));
    frames(frameMs);
    expect(holds?.heard).toEqual(["seekBack start", "seekBack released (pad)"]);
  });

  // The research's §3.1 (starts-chunks-remote-2026-10-29.md): a held stick's arrows came as presses,
  // so the player stepped 10 s at each repeat and never waited for its release.
  it("seeks by a controller's held stick as by a remote's held arrow: by the time held, once, after its release", () => {
    const video = document.createElement("video");
    const seeksS: number[] = [];
    Object.defineProperty(video, "duration", { value: 7200 });
    Object.defineProperty(video, "currentTime", {
      get: () => 0,
      set: (toS: number) => {
        seeksS.push(toS);
      },
    });
    document.body.append(video);
    const stopPlayer = onCommand(document, (detail, event) => {
      if (answerPlayerCommand(video, detail.command, detail)) {
        event.preventDefault();
      }
    });
    const pad = padAtRest();
    connect(pad);
    pad.axes = [1, 0, 0, 0];
    frames(6000);
    pad.axes = [0, 0, 0, 0];
    frames(seekRestMs - frameMs);
    expect(seeksS).toEqual([]); // not before its release has rested
    frames(2 * frameMs);
    // A 2-hour film held 6 s: 2 s at 72 s a second (1%), 2 s at 216 s (3%), 2 s at 576 s (8%), less
    // the time after its last repeat, under 150 ms at 16 ms frames; by its 50-odd repeats, 10 s each,
    // it would be about 500 s.
    const heldS = (seekStepMs + 2 * 72_000 + 2 * 216_000 + 2 * 576_000) / 1000;
    expect(seeksS).toHaveLength(1);
    expect(seeksS[0]).toBeGreaterThan(heldS - 0.15 * 576);
    expect(seeksS[0]).toBeLessThanOrEqual(heldS);
    stopPlayer();
  });

  it("seeks by a controller's held fast-forward trigger as by a held stick: by the time held, once, after its release", () => {
    const video = document.createElement("video");
    const seeksS: number[] = [];
    Object.defineProperty(video, "duration", { value: 7200 });
    Object.defineProperty(video, "currentTime", {
      get: () => 0,
      set: (toS: number) => {
        seeksS.push(toS);
      },
    });
    document.body.append(video);
    const stopPlayer = onCommand(document, (detail, event) => {
      if (answerPlayerCommand(video, detail.command, detail)) {
        event.preventDefault();
      }
    });
    const pad = padAtRest();
    connect(pad);
    pad.buttons[7] = { pressed: true };
    frames(6000);
    pad.buttons[7] = { pressed: false };
    frames(seekRestMs - frameMs);
    expect(seeksS).toEqual([]);
    frames(2 * frameMs);
    // As the stick's, the same film held the same 6 s (see there for the sum).
    const heldS = (seekStepMs + 2 * 72_000 + 2 * 216_000 + 2 * 576_000) / 1000;
    expect(seeksS).toHaveLength(1);
    expect(seeksS[0]).toBeGreaterThan(heldS - 0.15 * 576);
    expect(seeksS[0]).toBeLessThanOrEqual(heldS);
    stopPlayer();
  });
});
