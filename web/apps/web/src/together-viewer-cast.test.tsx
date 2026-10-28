// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { LiveProvider } from "./live.tsx";
import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video", isLeader: true }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a signed-in guest's page while their device casts (D371 §4)", () => {
  it("leaves the film to the device it casts to, which follows the group in its place, holding the group's controls (D371 §4)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const services = {
      ...servedBy(
        {
          "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
          "GET /together/mine": [200, { sessions: [] }],
        },
        [],
        "token",
      ),
      navigation: createNavigation(fakePlace("/together")),
    };
    const sockets = liveAndSession(2); // the page's session opens first, its live connection after
    const mounted = await mount(
      withServices(
        <LiveProvider>
          <TogetherViewer />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const playing = { state: "Playing", positionMs: 70_000, rate: 1, fileId: 5 };
    await sockets.session.say({ type: "group", seq: 2, group: playing });
    expect(mounted.container.querySelector("video")).not.toBeNull();
    await sockets.live.say({
      type: "sending",
      sending: {
        castId: 9,
        receiverDeviceId: 2,
        receiverName: "Fire TV",
        fileId: 0,
        isAccepted: true,
      },
    });
    expect(mounted.container.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Playing on Fire TV");
    mounted.unmount();
  });
});
