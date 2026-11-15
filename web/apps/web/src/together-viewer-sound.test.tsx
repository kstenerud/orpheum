// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { LiveProvider } from "./live.tsx";
import { createNavigation } from "./navigation.ts";
import { forgetSoundOffsetSetInThisPage, soundOffsetKey } from "./sound-offset.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { GroupFilmPage } from "./together-viewer.tsx";

/** The guest's membership of session 1, as their browser keeps it. */
const viewer = { membership: { sessionId: 1, credential: "viewer-credential" } };

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "playing", positionMs: 70_000, rate: 1, fileId: 5 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video", isLeader: true }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  forgetSoundOffsetSetInThisPage();
});

/**
 * The group's film on a guest's page, signed in on device 7 when deviceId says so; every request it
 * makes in heard.
 */
async function groupFilm(deviceId: number | undefined, heard: Heard[] = []): Promise<Mounted> {
  const services = {
    ...servedBy(
      {
        "POST /together/playback": [200, directDecision({})],
        "GET /me/together": [200, { sessions: [] }],
      },
      heard,
      "token",
    ),
    navigation: createNavigation(fakePlace("/play/5")),
  };
  if (deviceId !== undefined) {
    services.session.signIn("token", { deviceId });
  }
  const sockets = liveAndSession(2); // the page's session opens first, its live connection after
  const page = (
    <LiveProvider>
      <GroupFilmPage {...viewer} />
    </LiveProvider>
  );
  const mounted = await mount(
    withServices(page, { ...services, openTogetherSocket: sockets.openSocket }),
  );
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await sockets.live.accept();
  await sockets.session.acceptEach();
  await sockets.session.say(welcome);
  return mounted;
}

describe("a Watch Together viewer's sound timing (the owner, 2026-11-06)", () => {
  it("starts from the offset this device's browser keeps, asking the core for none (D492 §1)", async () => {
    window.localStorage.setItem(soundOffsetKey, "-200");
    const heard: Heard[] = [];
    const mounted = await groupFilm(7, heard);
    await settleUntil(() =>
      mounted.container.textContent.includes("Sound on this device: 200 ms earlier"),
    );
    expect(buttonWithText(mounted.container, "Sound 50 ms earlier")).toBeDefined();
    const asked = heard.map((each) => each.methodAndPath);
    expect(
      asked.filter((each) => each.startsWith("GET /me") && each !== "GET /me/together"),
    ).toEqual([]);
    mounted.unmount();
  });

  it("starts in step for a viewer with no device of their own, who may still move it", async () => {
    const mounted = await groupFilm(undefined);
    await settleUntil(() =>
      mounted.container.textContent.includes("Sound on this device: in step"),
    );
    await click(buttonWithText(mounted.container, "Sound 50 ms earlier"));
    expect(mounted.container.textContent).toContain("Sound on this device: 50 ms earlier");
    mounted.unmount();
  });
});
