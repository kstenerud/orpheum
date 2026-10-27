// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { chooseOk } from "./controller-choice.ts";
import { LabInput } from "./lab-input.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** Every event the page sent, in order. */
function sentEvents(heard: readonly Heard[]): Record<string, unknown>[] {
  return heard
    .filter((request) => request.methodAndPath === "POST /lab-input-events")
    .flatMap((request) => bodyField(request, "events") as Record<string, unknown>[]);
}

/** The sent events of kind. */
function eventsOf(heard: readonly Heard[], kind: string): Record<string, unknown>[] {
  return sentEvents(heard).filter((event) => event["kind"] === kind);
}

/** A pad as navigator.getGamepads gives it, with what a person does to it. */
interface FakePad {
  readonly index: number;
  readonly id: string;
  readonly mapping: string;
  readonly buttons: readonly Pick<GamepadButton, "pressed">[];
  readonly axes: readonly number[];
  /** Holds button down, or lets every button go. */
  readonly hold: (button: number | undefined) => void;
  /** Leans the axes to axes. */
  readonly lean: (axes: readonly number[]) => void;
}

/** A pad of id and mapping, at rest, with 17 buttons and four axes. */
function fakePad(id: string, mapping = "standard"): FakePad {
  let held: number | undefined;
  let leaning: readonly number[] = [0, 0, 0, 0];
  return {
    index: 0,
    id,
    mapping,
    get buttons() {
      return Array.from({ length: 17 }, (_, index) => ({ pressed: index === held }));
    },
    get axes() {
      return leaning;
    },
    hold: (button) => {
      held = button;
    },
    lean: (axes) => {
      leaning = axes;
    },
  };
}

/** The pads navigator.getGamepads gives: a browser with the Gamepad API, none connected yet. */
let connected: FakePad[] = [];

/** Connects pad: navigator.getGamepads gives it from now on, and the page is told. */
function connect(pad: FakePad): void {
  connected = [pad];
  act(() => {
    globalThis.dispatchEvent(new Event("gamepadconnected"));
  });
}

/** Lets the page poll its pads a few frames. */
async function aFewFrames(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 60));
  });
}

/** Presses button on pad, held until the page has read it, then lets it go. */
async function pressPad(pad: FakePad, button: number, isRead: () => boolean): Promise<void> {
  pad.hold(button);
  await settleUntil(isRead);
  pad.hold(undefined);
  await aFewFrames();
}

/** The button test, its events sent to a stand-in server that takes them all. */
async function mountTest(heard: Heard[]): Promise<Mounted> {
  const services = servedBy({ "POST /lab-input-events": [204, null] }, heard, "token");
  return await mount(withServices(<LabInput />, services));
}

describe("the button test's controllers", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    // The OK a test chose is kept in the browser, as the person's is: each test starts with none.
    chooseOk("automatic");
    connected = [];
    Object.defineProperty(navigator, "getGamepads", {
      configurable: true,
      value: () => connected,
    });
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(navigator, "getGamepads");
  });

  it("reads a pad the browser announced before the test opened, as on coming back to it", async () => {
    // A browser fires gamepadconnected once a document; opened again within the app, the test
    // hears of no pad but the one getGamepads gives (the owner, Firefox on a Mac, 2026-10-25).
    connected = [fakePad("045e-02e0-8Bitdo SN30 Pro")];
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    await settleUntil(() => eventsOf(heard, "pad").length > 0);
    expect(eventsOf(heard, "pad")).toEqual([
      expect.objectContaining({ padLayout: "xbox", padMapping: "standard" }),
    ]);
  });

  it("reads a Switch Pro as Nintendo's, whose right button is OK, and names its buttons by place", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const pad = fakePad("Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)");
    connect(pad);
    await settleUntil(() => eventsOf(heard, "pad").length > 0);
    expect(eventsOf(heard, "pad")).toEqual([
      expect.objectContaining({
        padIndex: 0,
        padMapping: "standard",
        padLayout: "nintendo",
        okButton: "right",
      }),
    ]);
    expect(mounted.container.textContent).toContain("Nintendo layout, OK is the right button");
    // A known layout still offers the choice: an 8BitDo in X-input mode reports Xbox's, its labels
    // Nintendo's (the owner, 2026-10-25).
    expect(buttonWithText(mounted.container, "the bottom button")).toBeDefined();
    await pressPad(pad, 1, () => eventsOf(heard, "gamepad").length > 0);
    await pressPad(pad, 0, () => eventsOf(heard, "gamepad").length > 1);
    expect(eventsOf(heard, "gamepad")).toEqual([
      expect.objectContaining({
        button: "select",
        padButton: 1,
        padButtonName: "faceRight",
        padLayout: "nintendo",
      }),
      expect.objectContaining({ button: "back", padButton: 0, padButtonName: "faceBottom" }),
    ]);
  });

  it("offers the choice of OK for a pad of unknown layout, and reads its buttons by the choice", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const pad = fakePad("8Bitdo SF30 Pro (STANDARD GAMEPAD Vendor: 2dc8 Product: 6102)");
    connect(pad);
    await settleUntil(() => eventsOf(heard, "pad").length > 0);
    expect(eventsOf(heard, "pad")).toEqual([
      expect.objectContaining({ padLayout: "unknown", okButton: "bottom" }),
    ]);
    const right = buttonWithText(mounted.container, "the right button");
    expect(right?.getAttribute("aria-pressed")).toBe("false");
    await click(right);
    await settleUntil(() => eventsOf(heard, "okChoice").length > 0);
    expect(eventsOf(heard, "okChoice")).toEqual([expect.objectContaining({ okButton: "right" })]);
    await pressPad(pad, 1, () => eventsOf(heard, "gamepad").length > 0);
    expect(eventsOf(heard, "gamepad")).toEqual([
      expect.objectContaining({ button: "select", padLayout: "unknown" }),
    ]);
    expect(right?.getAttribute("aria-pressed")).toBe("true");
  });

  it("says a pad with no standard mapping has no layout, and sends its buttons as numbers and its axes as they cross half way", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const pad = fakePad("2dc8-6102-8Bitdo SF30 Pro", "");
    connect(pad);
    await settleUntil(() => eventsOf(heard, "pad").length > 0);
    expect(mounted.container.textContent).toContain("This controller has no standard layout");
    await pressPad(pad, 14, () => eventsOf(heard, "gamepad").length > 0);
    expect(eventsOf(heard, "gamepad")).toEqual([
      expect.objectContaining({ button: "other", padButton: 14 }),
    ]);
    expect(eventsOf(heard, "gamepad")[0]).not.toHaveProperty("padButtonName");
    pad.lean([0, 0, 0.4, 0]);
    await aFewFrames();
    pad.lean([0, 0, 0.9, 0]);
    await settleUntil(() => eventsOf(heard, "axis").length > 0);
    pad.lean([0, 0, 0.1, 0]);
    await settleUntil(() => eventsOf(heard, "axis").length > 1);
    expect(eventsOf(heard, "axis")).toEqual([
      expect.objectContaining({ padIndex: 0, axisIndex: 2, axisValue: 0.9 }),
      expect.objectContaining({ axisIndex: 2, axisValue: 0.1 }),
    ]);
  });

  it("sends a standard pad's stick leaning as a direction, and moves our focus by it and by the D-pad", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const pad = fakePad("Xbox 360 Controller (XInput STANDARD GAMEPAD)");
    connect(pad);
    pad.lean([0, 0.8, 0, 0]);
    await settleUntil(() => eventsOf(heard, "stick").length > 0);
    expect(eventsOf(heard, "stick")).toEqual([
      expect.objectContaining({
        button: "down",
        axisIndex: 1,
        axisValue: 0.8,
        padLayout: "xbox",
      }),
    ]);
    // The first arrow starts our navigation at the tiles, as a key's does: the focus moves a frame
    // after the event is heard, which a loaded machine may not give before the check.
    await settleUntil(() => document.activeElement?.classList.contains("lab__tile") === true);
    expect(document.activeElement?.classList.contains("lab__tile")).toBe(true);
    pad.lean([0, 0, 0, 0]);
    await aFewFrames();
    act(() => {
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
    });
    await pressPad(pad, 13, () => eventsOf(heard, "gamepad").length > 0);
    expect(eventsOf(heard, "gamepad")).toEqual([
      expect.objectContaining({ button: "down", padButtonName: "dpadDown" }),
    ]);
    await settleUntil(() => document.activeElement?.classList.contains("lab__tile") === true);
    expect(document.activeElement?.classList.contains("lab__tile")).toBe(true);
  });

  it("presses what has focus by its layout's OK, as Enter does", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const pad = fakePad("Xbox 360 Controller (XInput STANDARD GAMEPAD)");
    connect(pad);
    const browsers = buttonWithText(mounted.container, "The browser's own");
    act(() => {
      browsers?.focus();
    });
    await pressPad(pad, 0, () => eventsOf(heard, "mode").length > 0);
    expect(browsers?.getAttribute("aria-pressed")).toBe("true");
    expect(eventsOf(heard, "mode")).toEqual([expect.objectContaining({ mode: "browser" })]);
  });
});
