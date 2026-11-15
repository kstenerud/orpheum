// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { SoundOffsetControl } from "./sound-delay.tsx";
import { forgetSoundOffsetSetInThisPage, soundOffsetKey } from "./sound-offset.ts";
import { newSoundPanel, SoundPanelContext } from "./sound-panel.ts";
import { fire, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetSoundOffsetSetInThisPage();
  window.localStorage.removeItem(soundOffsetKey);
});

/** Presses the control's button whose text is text. */
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

/** What this device's browser keeps as its sound offset. */
function keptByTheBrowser(): unknown {
  return JSON.parse(window.localStorage.getItem(soundOffsetKey) ?? "null");
}

describe("this device's sound timing (the owner's TCL TV, 2026-10-24)", () => {
  it("starts as the device's browser keeps it, and keeps each step pressed there, asking the core nothing (D492 §1)", async () => {
    window.localStorage.setItem(soundOffsetKey, "550");
    const heard: Heard[] = [];
    const services = servedBy({}, heard);
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<SoundOffsetControl videoRef={createRef()} />, services));
    expect(mounted.container.textContent).toContain("Sound on this device: 550 ms later");
    press("Sound 50 ms later");
    expect(mounted.container.textContent).toContain("Sound on this device: 600 ms later");
    expect(keptByTheBrowser()).toBe(600);
    expect(heard).toEqual([]);
  });

  it("starts the next control from what was set before it, as the next film's does", async () => {
    mounted = await mount(withServices(<SoundOffsetControl videoRef={createRef()} />));
    press("Sound 50 ms earlier");
    mounted.unmount();
    forgetSoundOffsetSetInThisPage();
    mounted = await mount(withServices(<SoundOffsetControl videoRef={createRef()} />));
    expect(mounted.container.textContent).toContain("Sound on this device: 50 ms earlier");
  });

  it("goes earlier as well as later, and says a film playing its own file cannot play its sound earlier", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    (videoRef as { current: HTMLVideoElement | null }).current = document.createElement("video");
    mounted = await mount(withServices(<SoundOffsetControl videoRef={videoRef} />));
    const earlier = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Sound 50 ms earlier",
    );
    await fire(earlier ?? null, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.textContent).toContain("Sound on this device: 50 ms earlier");
    // No chunked playback plays this video: its sound cannot be moved earlier, and the page says so.
    expect(mounted.container.textContent).toContain("whose sound cannot be played earlier here");
  });

  it("shows and applies the one value in every control drawn, the hidden one and the ⁝ menu's (D407 §8)", async () => {
    mounted = await mount(
      withServices(
        <>
          <SoundOffsetControl videoRef={createRef()} />
          <SoundOffsetControl videoRef={createRef()} />
        </>,
      ),
    );
    const [first] = mounted.container.querySelectorAll("button");
    act(() => {
      first?.click();
    });
    const shown = [...mounted.container.querySelectorAll(".play__sound-delay span")].map(
      (each) => each.textContent,
    );
    expect(shown).toEqual([
      "Sound on this device: 50 ms earlier",
      "Sound on this device: 50 ms earlier",
    ]);
  });

  it("is offered to a Watch Together viewer with no device, kept by its browser all the same", async () => {
    // The owner, 2026-11-06: "Put a control in watch together ... so that I can adjust the sound
    // timing".
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} />, servedBy({}, heard)),
    );
    expect(mounted.container.textContent).toContain("Sound on this device: in step");
    press("Sound 50 ms earlier");
    expect(mounted.container.textContent).toContain("Sound on this device: 50 ms earlier");
    expect(keptByTheBrowser()).toBe(-50);
    expect(heard).toEqual([]);
  });

  it("tells a cast's remote how it stands, and moves as the remote chooses, kept by the receiver (the owner, 2026-11-06)", async () => {
    // "Put a control in ... cast so that I can adjust the sound timing", from the phone that casts.
    const heard: Heard[] = [];
    const services = servedBy({}, heard);
    services.session.signIn("token", { deviceId: 5 });
    const panel = newSoundPanel();
    mounted = await mount(
      withServices(
        <SoundPanelContext value={panel}>
          <SoundOffsetControl videoRef={createRef()} />
          <SoundOffsetControl videoRef={createRef()} isInAList />
        </SoundPanelContext>,
        services,
      ),
    );
    expect(panel.read()?.soundOffsetMs).toBe(0);
    act(() => {
      panel.choose({ kind: "soundOffset", ms: -200 });
    });
    expect(panel.read()?.soundOffsetMs).toBe(-200);
    const shown = [...mounted.container.querySelectorAll("span")].map((each) => each.textContent);
    expect(shown).toEqual([
      "Sound on this device: 200 ms earlier",
      "Sound on this device: 200 ms earlier",
    ]);
    expect(keptByTheBrowser()).toBe(-200);
    expect(heard).toEqual([]);
  });
});
