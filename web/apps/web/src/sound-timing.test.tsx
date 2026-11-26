// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { SoundTiming } from "./sound-timing.tsx";
import { BackgroundChangesContext, type Ask, type Poller } from "./polling.ts";
import { routeHref } from "./router.ts";
import {
  mount,
  mountToRevisit,
  servedBy,
  settleUntil,
  unanswered,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The clip as the server answers it: being made, or made as file 77. */
function clip(isReady: boolean): object {
  return {
    purpose: "soundTiming",
    codec: "h264-aac",
    fileName: "soundTiming.h264-aac.mp4",
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
        "GET /system-videos/soundTiming/h264-aac": [
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
          <SoundTiming />
        </BackgroundChangesContext>,
        services,
      ),
    );
    const page = mounted;
    await settleUntil(() => page.container.querySelector("output") !== null);
    expect(page.container.querySelector('a[href^="/play/"]')).toBeNull();
    await act(async () => {
      for (const ask of asks) {
        ask(() => undefined);
      }
      await Promise.resolve();
    });
    await settleUntil(() => page.container.querySelector('a[href^="/play/"]') !== null);
    expect(page.container.querySelector('a[href^="/play/"]')?.getAttribute("href")).toBe(
      routeHref({ screen: "play", fileId: 77 }),
    );
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "GET /system-videos/soundTiming/h264-aac",
      "GET /system-videos/soundTiming/h264-aac",
    ]);
  });

  it("plays the clip its own read answers, never one an earlier visit to the page read (D478)", async () => {
    const services = servedBy({
      "GET /system-videos/soundTiming/h264-aac": [[200, clip(true)], unanswered],
    });
    services.session.signIn("token");
    const byHand: Poller = { start: () => () => undefined };
    const page = await mountToRevisit(
      withServices(
        <BackgroundChangesContext value={byHand}>
          <SoundTiming />
        </BackgroundChangesContext>,
        services,
      ),
    );
    mounted = page;
    await settleUntil(() => page.container.querySelector('a[href^="/play/"]') !== null);
    await page.leave();
    await page.comeBack();
    expect(page.container.querySelector("#sound-timing-heading")).not.toBeNull();
    expect(page.container.querySelector('a[href^="/play/"]')).toBeNull();
  });
});
