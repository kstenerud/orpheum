// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LabSound } from "./lab-sound.tsx";
import { BackgroundChangesContext, type Ask, type Poller } from "./polling.ts";
import { routeHref } from "./router.ts";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The clip as the server answers it: being made, or made as file 77. */
function clip(isReady: boolean): object {
  return {
    purpose: "sound-timing",
    codec: "h264-aac",
    fileName: "sound-timing.h264-aac.mp4",
    isReady,
    fileId: isReady ? 77 : 0,
    sizeBytes: isReady ? 4096 : 0,
  };
}

describe("the sound timing test (D435 §6)", () => {
  it("asks for the clip by what it is and plays the file it is given on the Play page", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /system-videos/sound-timing/h264-aac": [
          [200, clip(false)],
          [200, clip(true)],
        ],
      },
      heard,
    );
    services.session.signIn("token");
    const asks: Ask[] = [];
    const byHand: Poller = {
      start: (ask) => {
        asks.push(ask);
        return () => undefined;
      },
    };
    mounted = await mount(
      withServices(
        <BackgroundChangesContext value={byHand}>
          <LabSound />
        </BackgroundChangesContext>,
        services,
      ),
    );
    const page = mounted;
    await settleUntil(() => page.container.querySelector("output") !== null);
    expect(page.container.querySelector("a.lab__button")).toBeNull();
    await act(async () => {
      for (const ask of asks) {
        ask(() => undefined);
      }
      await Promise.resolve();
    });
    await settleUntil(() => page.container.querySelector("a.lab__button") !== null);
    expect(page.container.querySelector("a.lab__button")?.getAttribute("href")).toBe(
      routeHref({ screen: "play", fileId: 77 }),
    );
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "GET /system-videos/sound-timing/h264-aac",
      "GET /system-videos/sound-timing/h264-aac",
    ]);
  });
});
