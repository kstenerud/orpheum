// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, fire, mount, servedBy } from "./test-dom.tsx";
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

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

/**
 * What the guest's page sends, and whether its player is paused after, when its own player is played
 * from outside the page while the group is paused, the guest's link giving rights.
 */
async function playedFromOutside(rights: {
  canPlayPause: boolean;
  canSeek: boolean;
}): Promise<{ commands: unknown[]; isPausedAfter: boolean }> {
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
  await fire(video, new Event("seeked"));
  // The AirPlay device's remote, the lock screen or Safari's own controls: a play the page did not make.
  await video.play();
  await fire(video, new Event("play"));
  const commands = server.heard.filter((message) => message["type"] === "command");
  const isPausedAfter = isPaused();
  mounted.unmount();
  return { commands, isPausedAfter };
}

// AirPlay is offered to a guest by link whatever their link's controls (D520 §2): a press on the
// AirPlay device, the phone's lock screen or Safari's own controls reaches the guest's page as its own
// player's move, which it cannot tell from any other.
describe("a Watch Together guest's page, its player moved from outside it (D520 §2)", () => {
  it("sends no command and pauses again where the group is when the guest's link gives no controls", async () => {
    expect(await playedFromOutside({ canPlayPause: false, canSeek: false })).toEqual({
      commands: [],
      isPausedAfter: true,
    });
  });

  it("sends the play as the group's when the guest's link gives play and pause, as before", async () => {
    expect(await playedFromOutside({ canPlayPause: true, canSeek: false })).toEqual({
      commands: [expect.objectContaining({ type: "command", command: "play" })],
      isPausedAfter: false,
    });
  });
});
