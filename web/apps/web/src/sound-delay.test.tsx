// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  forgetSoundOffsetSetInThisTab,
  SoundOffsetControl,
  soundOffsetKeptAfterMs,
} from "./sound-delay.tsx";
import { fire, mount, refusal, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetSoundOffsetSetInThisTab();
  vi.useRealTimers();
});

/**
 * The test's clock from now on: no time passes but as the test advances it, so however slow the
 * machine, a press is never kept before the test lets the wait after it pass.
 */
function holdTheClock(): void {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
}

/** Presses the control's button whose text is text, on the test's held clock. */
function press(text: string): void {
  const button = [...(mounted?.container.querySelectorAll("button") ?? [])].find(
    (each) => each.textContent === text,
  );
  if (button === undefined) {
    throw new Error(`no button ${text}`);
  }
  act(() => {
    button.click();
  });
}

/** Lets byMs pass on the held clock, and what it sends be answered. */
async function letPass(byMs: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(byMs);
  });
}

describe("this device's sound timing (the owner's TCL TV, 2026-10-24)", () => {
  it("starts as the playback gives it, and keeps each step pressed on the device", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] }, heard);
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={550} />, services),
    );
    holdTheClock();
    expect(mounted.container.textContent).toContain("Sound on this device: 550 ms later");
    press("Sound 50 ms later");
    expect(mounted.container.textContent).toContain("Sound on this device: 600 ms later");
    await letPass(soundOffsetKeptAfterMs);
    expect(heard).toEqual([{ methodAndPath: "PUT /me/devices/5", body: { soundOffsetMs: 600 } }]);
  });

  it("keeps a run of presses as one write, of where it ended (the audit of 2026-10-25, W9)", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] }, heard);
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={0} />, services),
    );
    holdTheClock();
    press("Sound 50 ms later");
    press("Sound 50 ms later");
    press("Sound 50 ms later");
    await letPass(soundOffsetKeptAfterMs - 1);
    // A press within the wait starts it again.
    press("Sound 50 ms earlier");
    await letPass(soundOffsetKeptAfterMs - 1);
    expect(heard).toEqual([]);
    await letPass(1);
    expect(heard).toEqual([{ methodAndPath: "PUT /me/devices/5", body: { soundOffsetMs: 100 } }]);
  });

  it("keeps a press still waiting as the control goes", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] }, heard);
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={0} />, services),
    );
    holdTheClock();
    press("Sound 50 ms earlier");
    mounted.unmount();
    mounted = undefined;
    // No time passes: the wait after the press never ends, and the press is kept all the same.
    await letPass(0);
    expect(heard).toEqual([{ methodAndPath: "PUT /me/devices/5", body: { soundOffsetMs: -50 } }]);
  });

  it("says, and reports, a timing the device could not keep", async () => {
    const reported = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const services = servedBy({
      "PUT /me/devices/5": [refusal(503, "migrating"), [204, undefined]],
    });
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={0} />, services),
    );
    holdTheClock();
    press("Sound 50 ms later");
    await letPass(soundOffsetKeptAfterMs);
    expect(mounted.container.textContent).toContain("This device could not keep its sound timing");
    expect(reported).toHaveBeenCalled();
    // The next press kept, it no longer says so.
    press("Sound 50 ms later");
    await letPass(soundOffsetKeptAfterMs);
    expect(mounted.container.textContent).not.toContain("could not keep");
  });

  it("starts at what this tab last set, which a playback asked for earlier did not hear of (2026-10-24)", async () => {
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] });
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={50} />, services),
    );
    const less = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Sound 50 ms earlier",
    );
    await fire(less ?? null, new MouseEvent("click", { bubbles: true }));
    mounted.unmount();
    // The next film's decision was asked for before the change, and still says 50.
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={50} />, services),
    );
    expect(mounted.container.textContent).toContain(
      "Sound on this device: in step with the picture",
    );
  });

  it("goes earlier as well as later, and says a film playing its own file cannot play its sound earlier", async () => {
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] });
    services.session.signIn("token", { deviceId: 5 });
    const videoRef = createRef<HTMLVideoElement>();
    (videoRef as { current: HTMLVideoElement | null }).current = document.createElement("video");
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={videoRef} initialMs={0} />, services),
    );
    const earlier = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Sound 50 ms earlier",
    );
    await fire(earlier ?? null, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.textContent).toContain("Sound on this device: 50 ms earlier");
    // No chunked playback plays this video: its sound cannot be moved earlier, and the page says so.
    expect(mounted.container.textContent).toContain("whose sound cannot be played earlier here");
  });

  it("is not offered to a viewer with no device to keep it", async () => {
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={0} />, servedBy({})),
    );
    expect(mounted.container.textContent).not.toContain("Sound on this device");
  });
});
