// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { everyRight, startingTuning, type TogetherSnapshot } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { createHiddenNeed } from "./hidden-need.ts";
import type { FilmsPage } from "./page-freezes.ts";
import { SessionRemote } from "./session-remote.tsx";
import { mount } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** A page whose one video, paused at first, plays or ends as the test says. */
function pageWithVideo(): {
  readonly page: FilmsPage;
  readonly play: () => void;
  readonly end: () => void;
} {
  const video = { paused: true, ended: false };
  return {
    page: { querySelectorAll: () => [video] },
    play: () => {
      video.paused = false;
    },
    end: () => {
      video.ended = true;
    },
  };
}

const hosting: TogetherSnapshot = {
  status: "connected",
  memberId: 1,
  isHost: true,
  rights: everyRight,
  group: undefined,
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  answersHeard: 0,
  tuning: startingTuning,
};

describe("what a page needs of its WebRTC connection while hidden (D482 §3)", () => {
  it("needs it while a film plays in the page, read as each try is weighed, not once it pauses or ends", () => {
    const video = pageWithVideo();
    const need = createHiddenNeed(video.page);
    expect(need.isNeeded()).toBe(false);
    video.play();
    expect(need.isNeeded()).toBe(true);
    video.end();
    expect(need.isNeeded()).toBe(false);
  });

  it("needs it while any cast's remote is shown, each mark's removal counted once", () => {
    const need = createHiddenNeed(pageWithVideo().page);
    const first = need.remoteShown();
    const second = need.remoteShown();
    first();
    first();
    expect(need.isNeeded()).toBe(true);
    second();
    expect(need.isNeeded()).toBe(false);
  });

  it("is marked by a session's remote for as long as it is up", async () => {
    const hiddenNeed = createHiddenNeed(document);
    const mounted = await mount(
      withServices(<SessionRemote snapshot={hosting} connection={undefined} durationMs={0} />, {
        ...testServices(),
        hiddenNeed,
      }),
    );
    expect(hiddenNeed.isNeeded()).toBe(true);
    mounted.unmount();
    expect(hiddenNeed.isNeeded()).toBe(false);
  });
});
