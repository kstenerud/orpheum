// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LabInput } from "./lab-input.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
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
    .filter((request) => request.methodAndPath === "POST /lab/input-events")
    .flatMap((request) => bodyField(request, "events") as Record<string, unknown>[]);
}

/** The sent events of kind. */
function eventsOf(heard: readonly Heard[], kind: string): Record<string, unknown>[] {
  return sentEvents(heard).filter((event) => event["kind"] === kind);
}

/** A key pressed on target, as a remote's or a keyboard's reaches it. */
function press(target: EventTarget, key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

/** The button test, its events sent to a stand-in server that takes them all. */
async function mountTest(heard: Heard[]): Promise<Mounted> {
  const services = servedBy({ "POST /lab/input-events": [204, null] }, heard, "token");
  return await mount(withServices(<LabInput />, services));
}

/** The test's text field. */
function fieldOf(mounted: Mounted): HTMLInputElement {
  const field = mounted.container.querySelector<HTMLInputElement>(".lab__type");
  if (field === null) {
    throw new Error("the test has no text field");
  }
  return field;
}

describe("the button test's text field", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.unstubAllGlobals();
  });

  it("sends focus, input and composition with the value's length, never its text", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const field = fieldOf(mounted);
    act(() => {
      field.focus();
    });
    await enter(field, "secret");
    act(() => {
      field.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
      field.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true }));
      field.blur();
    });
    await settleUntil(() => eventsOf(heard, "fieldOut").length > 0);
    expect(eventsOf(heard, "fieldIn")).toEqual([expect.objectContaining({ valueLength: 0 })]);
    expect(eventsOf(heard, "fieldInput")).toEqual([expect.objectContaining({ valueLength: 6 })]);
    expect(eventsOf(heard, "compositionStart")).toHaveLength(1);
    expect(eventsOf(heard, "compositionEnd")).toEqual([
      expect.objectContaining({ valueLength: 6 }),
    ]);
    expect(eventsOf(heard, "fieldOut")).toEqual([expect.objectContaining({ valueLength: 6 })]);
    expect(JSON.stringify(sentEvents(heard))).not.toContain("secret");
  });

  it("flags its keys as in the field, sends a typed character as Character, and its Backspace as no Back", async () => {
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    const field = fieldOf(mounted);
    act(() => {
      field.focus();
    });
    press(field, "q", 81);
    press(field, "Backspace", 8);
    act(() => {
      field.blur();
    });
    press(document.body, "Enter", 13);
    await settleUntil(() => eventsOf(heard, "key").length === 3);
    const [typed, deleted, outside] = eventsOf(heard, "key");
    expect(typed).toEqual(expect.objectContaining({ key: "Character", inField: true }));
    expect(typed).not.toHaveProperty("code");
    expect(typed).not.toHaveProperty("keyCode");
    expect(deleted).toEqual(
      expect.objectContaining({ key: "Backspace", button: "other", inField: true }),
    );
    expect(outside).toEqual(expect.objectContaining({ key: "Enter", button: "select" }));
    expect(outside).not.toHaveProperty("inField");
  });

  it("leaves the arrows inside the value to the field, and Up out of it to our navigation", async () => {
    mounted = await mountTest([]);
    const field = fieldOf(mounted);
    act(() => {
      field.focus();
    });
    await enter(field, "abc");
    field.setSelectionRange(1, 1);
    expect(press(field, "ArrowLeft", 37).defaultPrevented).toBe(false);
    expect(press(field, "ArrowRight", 39).defaultPrevented).toBe(false);
    field.setSelectionRange(3, 3);
    expect(press(field, "ArrowRight", 39).defaultPrevented).toBe(true);
    field.setSelectionRange(1, 1);
    expect(press(field, "ArrowUp", 38).defaultPrevented).toBe(true);
  });

  it("sends the visual viewport's every new size, as an on-screen keyboard changes it", async () => {
    const viewport = Object.assign(new EventTarget(), { width: 960, height: 540 });
    vi.stubGlobal("visualViewport", viewport);
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    act(() => {
      viewport.height = 270;
      viewport.dispatchEvent(new Event("resize"));
      viewport.dispatchEvent(new Event("resize"));
    });
    await settleUntil(() => eventsOf(heard, "viewport").length > 0);
    expect(eventsOf(heard, "viewport")).toEqual([
      expect.objectContaining({ kind: "viewport", width: 960, height: 270 }),
    ]);
  });
});

/** Gives every element a requestFullscreen, which happy-dom has not, answered by answer. */
function stubRequestFullscreen(answer: (element: Element) => Promise<void>): void {
  Object.defineProperty(Element.prototype, "requestFullscreen", {
    configurable: true,
    value: function requestFullscreen(this: Element) {
      return answer(this);
    },
  });
}

/** Puts element full screen as a browser does: the document says so, and tells the change. */
function enterFullscreen(element: Element | null): void {
  act(() => {
    Object.defineProperty(document, "fullscreenElement", { value: element, configurable: true });
    document.dispatchEvent(new Event("fullscreenchange"));
  });
}

describe("the button test's full screen", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    // happy-dom's canvas stream has no tracks to stop; a browser's stops its one.
    const stop = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "captureStream").mockReturnValue(
      Object.assign(new MediaStream(), { getTracks: () => [{ stop }] }),
    );
  });
  afterEach(() => {
    vi.restoreAllMocks();
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(document, "fullscreenElement");
    Reflect.deleteProperty(document, "exitFullscreen");
    Reflect.deleteProperty(Element.prototype, "requestFullscreen");
  });

  it("puts the video's box full screen, flags the keys heard in it, and leaves at Back", async () => {
    const asked: Element[] = [];
    stubRequestFullscreen(async (element) => {
      asked.push(element);
      await Promise.resolve();
    });
    const exit = vi.fn(async () => {
      await Promise.resolve();
    });
    Object.defineProperty(document, "exitFullscreen", { value: exit, configurable: true });
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    await click(buttonWithText(mounted.container, "Full-screen test"));
    const box = mounted.container.querySelector(".lab__screen");
    expect(asked).toEqual([box]);
    enterFullscreen(box);
    press(document.body, "Enter", 13);
    act(() => {
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(exit).toHaveBeenCalledTimes(1);
    enterFullscreen(null);
    await settleUntil(() => eventsOf(heard, "fullscreenExit").length > 0);
    expect(eventsOf(heard, "fullscreenEnter")).toEqual([
      expect.objectContaining({ fullscreenTarget: "container" }),
    ]);
    expect(eventsOf(heard, "key")).toEqual([expect.objectContaining({ inFullscreen: true })]);
    expect(eventsOf(heard, "back")).toEqual([expect.objectContaining({ inFullscreen: true })]);
    expect(eventsOf(heard, "fullscreenExitAsked")).toEqual([
      expect.objectContaining({ button: "back" }),
    ]);
    expect(eventsOf(heard, "fullscreenExit")).toEqual([
      expect.objectContaining({ fullscreenTarget: "container" }),
    ]);
  });

  it("puts the bare video full screen by its own button, and sends a refusal with its error's name", async () => {
    const asked: Element[] = [];
    stubRequestFullscreen(async (element) => {
      asked.push(element);
      await Promise.resolve();
      throw new DOMException("no", "NotAllowedError");
    });
    const heard: Heard[] = [];
    mounted = await mountTest(heard);
    await click(buttonWithText(mounted.container, "Full screen: video only"));
    expect(asked).toEqual([mounted.container.querySelector("video")]);
    await settleUntil(() => eventsOf(heard, "fullscreenRefused").length > 0);
    expect(eventsOf(heard, "fullscreenRefused")).toEqual([
      expect.objectContaining({ fullscreenTarget: "video", errorName: "NotAllowedError" }),
    ]);
  });
});
