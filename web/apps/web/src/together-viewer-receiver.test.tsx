// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { playingVideo } from "./test-hidden.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 7,
  deviceId: 8,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 7, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
  rights: { canPlayPause: true, canSeek: true },
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

/**
 * Where the guest's player stands, in s, once its group is paused 280 ms ahead of it, its film on an
 * AirPlay receiver where isOnReceiver, as WebKit says (VQ364's hold, staging, 2026-11-18).
 */
async function heldAt(isOnReceiver: boolean): Promise<number> {
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
  const mounted = await mount(withServices(<TogetherViewer />, services));
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await server.accept();
  await server.say(welcome);
  await server.say({
    type: "group",
    seq: 2,
    group: {
      state: "paused",
      positionMs: 10_000,
      rate: 1,
      fileId: 5,
      event: "paused",
      byMemberId: 1,
    },
  });
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("the guest's page drew no player");
  }
  playingVideo(video, 10);
  video.pause();
  Object.defineProperty(video, "webkitCurrentPlaybackTargetIsWireless", {
    value: isOnReceiver,
    configurable: true,
  });
  await server.say({
    type: "group",
    seq: 3,
    group: {
      state: "paused",
      positionMs: 10_280,
      rate: 1,
      fileId: 5,
      event: "sought",
      byMemberId: 1,
    },
  });
  const atS = video.currentTime;
  mounted.unmount();
  return atS;
}

describe("a Watch Together guest's page, its film on an AirPlay receiver (D522 §2)", () => {
  it("leaves the receiver 280 ms off its paused group where it is", async () => {
    expect(await heldAt(true)).toBe(10);
  });

  it("seeks its own player 280 ms off its paused group to the group, as before", async () => {
    expect(await heldAt(false)).toBe(10.28);
  });
});
