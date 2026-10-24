// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { forgetSoundOffsetSetInThisTab, SoundOffsetControl } from "./sound-delay.tsx";
import { fire, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetSoundOffsetSetInThisTab();
});

describe("this device's sound timing (the owner's TCL TV, 2026-10-24)", () => {
  it("starts as the playback gives it, and keeps each step pressed on the device", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/devices/5": [204, undefined] }, heard);
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(
      withServices(<SoundOffsetControl videoRef={createRef()} initialMs={550} />, services),
    );
    expect(mounted.container.textContent).toContain("Sound on this device: 550 ms later");
    const more = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Sound 50 ms later",
    );
    await fire(more ?? null, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.textContent).toContain("Sound on this device: 600 ms later");
    expect(heard).toEqual([{ methodAndPath: "PUT /me/devices/5", body: { soundOffsetMs: 600 } }]);
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
