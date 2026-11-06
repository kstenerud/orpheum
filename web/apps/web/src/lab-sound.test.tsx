// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LabSound } from "./lab-sound.tsx";
import { forgetSoundOffsetSetInThisTab, soundOffsetKeptAfterMs } from "./sound-delay.tsx";
import { soundLatencyKey } from "./sound-latency.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

/** The page's frames waiting to be drawn, which the test draws as it chooses. */
let frames: FrameRequestCallback[] = [];

/** The audio contexts the page made, each with the elements whose sound it took. */
let contexts: { readonly taken: HTMLMediaElement[] }[] = [];

/** An audio context as the player needs one: it runs once asked, and takes an element's sound. */
function fakeContext(): object {
  const taken: HTMLMediaElement[] = [];
  contexts.push({ taken });
  const node = { connect: () => undefined, disconnect: () => undefined };
  const context = {
    state: "suspended",
    destination: { channelCount: 2, maxChannelCount: 2 },
    resume: async () => {
      context.state = "running";
      await Promise.resolve();
    },
    createMediaElementSource: (element: HTMLMediaElement) => {
      taken.push(element);
      return node;
    },
    createGain: () => ({ ...node, gain: { value: 1 } }),
    createDelay: () => ({ ...node, delayTime: { value: 0 } }),
  };
  return context;
}

beforeEach(() => {
  window.localStorage.clear();
  frames = [];
  contexts = [];
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.push(callback);
    return frames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", () => undefined);
  vi.stubGlobal(
    "AudioContext",
    vi.fn(function made() {
      return fakeContext();
    }),
  );
  // happy-dom's element neither plays nor loads: its promise resolves, and it says so.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(async function played(
    this: HTMLMediaElement,
  ) {
    this.dispatchEvent(new Event("play"));
    await Promise.resolve();
  });
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetSoundOffsetSetInThisTab();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

/** The test's page, its device 5 keeping a sound offset of keptMs, Level loudness as given. */
async function soundTest(
  isLevellingOn: boolean,
  keptMs: number,
  heard: Heard[] = [],
): Promise<Mounted> {
  const services = servedBy(
    {
      "GET /me/preferences": [200, { isDialogueBoostOn: false, isLevellingOn, isAutoplayOn: true }],
      "GET /me/devices": [
        200,
        { devices: [television({ id: 5, isThisDevice: true, soundOffsetMs: keptMs })] },
      ],
      "PUT /me/devices/5": [204, undefined],
      "PUT /me/preferences": [204, undefined],
    },
    heard,
  );
  services.session.signIn("token", { deviceId: 5 });
  const page = await mount(withServices(<LabSound />, services));
  await settleUntil(() => page.container.querySelector(".lab-sound__panel") !== null);
  return page;
}

/** The picture as drawn at soundAtMs into the clip's sound: its number, and whether it flashes. */
function pictureAt(page: Mounted, soundAtMs: number): { beat: string; isFlash: boolean } {
  const video = page.container.querySelector("video");
  const panel = page.container.querySelector<HTMLElement>(".lab-sound__panel");
  if (video === null || panel === null) {
    throw new Error("no clip drawn");
  }
  Object.defineProperty(video, "currentTime", { configurable: true, value: soundAtMs / 1000 });
  const drawn = frames;
  frames = [];
  for (const draw of drawn) {
    draw(0);
  }
  return {
    beat: panel.textContent,
    isFlash: panel.classList.contains("lab-sound__panel--flash"),
  };
}

describe("the sound timing test (the owner, 2026-11-06)", () => {
  it("draws the flash with its beep, from the element's clock", async () => {
    mounted = await soundTest(false, 0);
    expect(mounted.container.querySelector("#lab-sound-heading")?.textContent).toBe("Sound timing");
    expect(pictureAt(mounted, 1020)).toEqual({ beat: "2", isFlash: true });
    expect(pictureAt(mounted, 1500)).toEqual({ beat: "2", isFlash: false });
    expect(pictureAt(mounted, 3050)).toEqual({ beat: "4", isFlash: true });
  });

  it("starts from the device's offset, and keeps each press with the device for every film", async () => {
    const heard: Heard[] = [];
    mounted = await soundTest(false, -100, heard);
    await settleUntil(
      () =>
        mounted?.container.textContent.includes("Sound on this device: 100 ms earlier") ?? false,
    );
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    act(() => {
      buttonWithText(mounted?.container ?? document.body, "Sound 50 ms earlier")?.click();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(soundOffsetKeptAfterMs);
    });
    expect(heard.filter((each) => each.methodAndPath === "PUT /me/devices/5")).toEqual([
      { methodAndPath: "PUT /me/devices/5", body: { soundOffsetMs: -150 } },
    ]);
  });

  it("holds the picture back by the player's move of the sound earlier, which a film's chunks would make", async () => {
    mounted = await soundTest(false, -300);
    await settleUntil(
      () =>
        mounted?.container.textContent.includes("Sound on this device: 300 ms earlier") ?? false,
    );
    // The second beep sounds at 1000 ms, and the picture shows it 300 ms after.
    expect(pictureAt(mounted, 1020)).toEqual({ beat: "1", isFlash: false });
    expect(pictureAt(mounted, 1320)).toEqual({ beat: "2", isFlash: true });
  });

  it("plays its sound through Web Audio with Level loudness on, as a levelled film's, and not with it off", async () => {
    mounted = await soundTest(false, 0);
    await click(buttonWithText(mounted.container, "Start"));
    expect(contexts).toEqual([]);
    expect(mounted.container.textContent).toContain(
      "Played as your films are with Level loudness off.",
    );
    mounted.unmount();
    // A film levelled moves its sound earlier by the latency the device remembers for Web Audio.
    window.localStorage.setItem(soundLatencyKey, "250");
    mounted = await soundTest(true, 0);
    const video = mounted.container.querySelector("video");
    await click(buttonWithText(mounted.container, "Start"));
    await settleUntil(() => contexts[0]?.taken.length === 1);
    expect(contexts[0]?.taken).toEqual([video]);
    expect(mounted.container.textContent).toContain(
      "Played as your films are with Level loudness on.",
    );
    expect(pictureAt(mounted, 1020)).toEqual({ beat: "1", isFlash: false });
    expect(pictureAt(mounted, 1270)).toEqual({ beat: "2", isFlash: true });
  });

  it("starts the clip anew on an element of its own as Level loudness is switched, and keeps the switch for every film", async () => {
    const heard: Heard[] = [];
    mounted = await soundTest(true, 0, heard);
    const before = mounted.container.querySelector("video");
    await click(mounted.container.querySelector('input[type="checkbox"]'));
    expect(mounted.container.querySelector("video")).not.toBe(before);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isLevellingOn: false },
    });
  });
});
