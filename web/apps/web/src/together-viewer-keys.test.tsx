// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createNavigation } from "./navigation.ts";
import { seekRestMs, seekStepMs } from "./seek-ahead.ts";
import { buttonWithText, click, mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { playingVideo } from "./test-hidden.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

/** Presses key on what has focus, or on the page where nothing has, as a keyboard does. */
function press(key: string): void {
  const target = document.activeElement ?? document.body;
  act(() => {
    target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
    target.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
  });
}

/** Waits past the rest after which a step's seek lands (seek-ahead.ts). */
async function pastTheSeeksRest(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, seekRestMs + 100));
  });
}

let mounted: Mounted | undefined;

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  (document.activeElement as HTMLElement | null)?.blur();
});

/**
 * A guest by link's page with the group's film paused at its start, the guest's link giving rights;
 * its video, standing at 0 s, and whether it is paused.
 */
async function guestsFilm(rights: {
  canPlayPause: boolean;
  canSeek: boolean;
}): Promise<{ video: HTMLVideoElement; isPaused: () => boolean }> {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
  const server = fakeTogetherServer();
  const services = {
    ...servedBy(
      { "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })] },
      [],
    ),
    navigation: createNavigation(fakePlace("/together")),
    openTogetherSocket: server.openSocket,
  };
  mounted = await mount(withServices(<TogetherViewer />, services));
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await server.accept();
  await server.say({ ...welcome, rights });
  await server.say({
    type: "group",
    seq: 2,
    group: { state: "paused", positionMs: 0, rate: 1, fileId: 5, event: "paused", byMemberId: 1 },
  });
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("the guest's page drew no player");
  }
  const isPaused = playingVideo(video, 0);
  video.pause();
  (document.activeElement as HTMLElement | null)?.blur();
  return { video, isPaused };
}

describe("a Watch Together guest's keys, as its buttons (D557 §6)", () => {
  it("plays at Space and steps at Right where the guest's link gives play and seeks", async () => {
    const { video, isPaused } = await guestsFilm({ canPlayPause: true, canSeek: true });
    press(" ");
    expect(isPaused()).toBe(false);
    press("ArrowRight");
    await settleUntil(() => video.currentTime !== 0);
    expect(video.currentTime).toBe(seekStepMs / 1000);
  });

  it("neither plays nor steps at a key where the guest's link gives neither, as neither button is drawn", async () => {
    const { video, isPaused } = await guestsFilm({ canPlayPause: false, canSeek: false });
    press(" ");
    press("ArrowRight");
    await pastTheSeeksRest();
    expect(isPaused()).toBe(true);
    expect(video.currentTime).toBe(0);
  });

  it("moves the guest's own volume at − and mutes at M, and puts the film full screen at F, whatever the link gives", async () => {
    const requestFullscreen = vi.fn(() => Promise.resolve());
    Object.defineProperty(document.documentElement, "requestFullscreen", {
      configurable: true,
      value: requestFullscreen,
    });
    try {
      const { video } = await guestsFilm({ canPlayPause: false, canSeek: false });
      const volume = video.volume;
      press("-");
      expect(video.volume).toBeLessThan(volume);
      press("m");
      expect(video.muted).toBe(true);
      press("f");
      expect(requestFullscreen).toHaveBeenCalledOnce();
    } finally {
      Reflect.deleteProperty(document.documentElement, "requestFullscreen");
    }
  });
});
