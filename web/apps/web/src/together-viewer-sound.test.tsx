// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { LiveProvider } from "./live.tsx";
import { createNavigation } from "./navigation.ts";
import { MeContext } from "./signed-in-me.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision, television } from "./test-fixtures.ts";
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
});

/**
 * Who the shell read as signed in (MeContext), on device 7 at -200 ms; a page drawn without it reads
 * GET /me, which says the same.
 */
const signedInOnDevice7 = {
  id: 1,
  name: "Gran",
  role: "user",
  isAdmin: false,
  canManageAccount: true,
  canHostWatchTogether: false,
  hasNoLibrary: false,
  showsDebugTools: false,
  device: { id: 7, name: "Living room TV", soundOffsetMs: -200 },
};

/**
 * The group's film on a guest's page, signed in on device 7 when deviceId says so, within the shell's
 * reading of who is signed in unless isAlone; every request it makes in heard.
 */
async function groupFilm(
  deviceId: number | undefined,
  heard: Heard[] = [],
  isAlone = false,
): Promise<Mounted> {
  const devices = [television({ id: 7, isThisDevice: true, soundOffsetMs: -200 })];
  const services = {
    ...servedBy(
      {
        "POST /together/playback": [200, directDecision({})],
        "GET /me": [200, signedInOnDevice7],
        "GET /me/devices": [200, { devices }],
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
  const me = { state: "loaded", value: signedInOnDevice7 } as const;
  const mounted = await mount(
    withServices(
      isAlone || deviceId === undefined ? (
        page
      ) : (
        <MeContext value={{ me, readMeAgain: () => undefined }}>{page}</MeContext>
      ),
      { ...services, openTogetherSocket: sockets.openSocket },
    ),
  );
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await sockets.live.accept();
  await sockets.session.acceptEach();
  await sockets.session.say(welcome);
  return mounted;
}

describe("a Watch Together viewer's sound timing (the owner, 2026-11-06)", () => {
  it("starts from the offset kept with the device the viewer is signed in on, as the server gives a guest's playback none", async () => {
    const mounted = await groupFilm(7);
    await settleUntil(() =>
      mounted.container.textContent.includes("Sound on this device: 200 ms earlier"),
    );
    expect(buttonWithText(mounted.container, "Sound 50 ms earlier")).toBeDefined();
    mounted.unmount();
  });

  it("takes that offset from who the shell read as signed in, asking for no list of devices (D480 §5)", async () => {
    const heard: Heard[] = [];
    const mounted = await groupFilm(7, heard);
    await settleUntil(() =>
      mounted.container.textContent.includes("Sound on this device: 200 ms earlier"),
    );
    const asked = heard.map((each) => each.methodAndPath);
    expect(asked).not.toContain("GET /me/devices");
    expect(asked).not.toContain("GET /me");
    mounted.unmount();
  });

  it("drawn alone, takes it from GET /me's device, still asking for no list of devices", async () => {
    const heard: Heard[] = [];
    const mounted = await groupFilm(7, heard, true);
    await settleUntil(() =>
      mounted.container.textContent.includes("Sound on this device: 200 ms earlier"),
    );
    expect(heard.map((each) => each.methodAndPath)).not.toContain("GET /me/devices");
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
