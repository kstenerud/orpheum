// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { beforeEach, describe, expect, it } from "vitest";

import { CastDialog } from "./cast-dialog.tsx";
import { DebugToolsContext } from "./debug-tools.tsx";
import { LiveProvider } from "./live.tsx";
import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy, settleUntil, type Heard } from "./test-dom.tsx";
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
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video", isLeader: true }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a signed-in guest's Play page of the group's film while their device casts (D371 §4, D384 §2)", () => {
  it("leaves the film to the device it casts to, which follows the group in its place, holding the group's controls (D371 §4)", async () => {
    const services = {
      ...servedBy(
        {
          "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
          "GET /me/together": [200, { sessions: [] }],
        },
        [],
        "token",
      ),
      navigation: createNavigation(fakePlace("/play/5")),
    };
    const sockets = liveAndSession(2); // the page's session opens first, its live connection after
    const mounted = await mount(
      withServices(
        <LiveProvider>
          <GroupFilmPage {...viewer} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const playing = { state: "playing", positionMs: 70_000, rate: 1, fileId: 5 };
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
    // Its session is told it plays no film, so it never leads in its receiver's place (D374 §1).
    const states = sockets.session.heard.filter((each) => each["type"] === "state");
    expect(states.at(-1)).toMatchObject({ fileId: 0, isPlaying: false });
    expect(states.some((each) => each["fileId"] === 5)).toBe(true);
    mounted.unmount();
  });

  it("carries Watch Together on the cast's controls, its panel in the button's dialog with Leave and no volume, not on the page (D440 §5, §6, §8)", async () => {
    const services = servedBy(
      {
        "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
        "GET /me/together": [200, { sessions: [] }],
      },
      [],
      "token",
    );
    const sockets = liveAndSession(2);
    const mounted = await mount(
      withServices(
        <LiveProvider>
          <GroupFilmPage {...viewer} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const playing = { state: "playing", positionMs: 70_000, rate: 1, fileId: 5 };
    await sockets.session.say({ type: "group", seq: 2, group: playing });
    const panel = mounted.container.querySelector(".together-panel");
    expect(panel?.hasAttribute("hidden")).toBe(false);
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
    const stage = mounted.container.querySelector(".together-viewer__viewing");
    const button = stage?.querySelector<HTMLButtonElement>(
      'button[aria-label="Watch Together is on: its controls"]',
    );
    if (button === null || button === undefined) {
      throw new Error("no Watch Together button on the cast's controls");
    }
    // The panel's Leave and volume no longer show on the page: the button holds them.
    expect(mounted.container.querySelector(".together-panel")?.hasAttribute("hidden")).toBe(true);
    await click(button);
    const leave = (): HTMLButtonElement | undefined => {
      const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
      return dialog === null ? undefined : buttonWithText(dialog, "Leave");
    };
    await settleUntil(() => leave() !== undefined);
    const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
    // The film plays on the receiver, whose volume is the remote's: the page's own has no part.
    expect(dialog?.querySelector("input[type=range]")).toBeNull();
    mounted.unmount();
  });
});

describe("a cast from over a guest's Play page of the group's film (D371 §4, VQ193)", () => {
  it("carries no film of its own, and says the guest's membership, so its receiver follows the group in the guest's part", async () => {
    window.localStorage.setItem("orpheum.together.viewer", JSON.stringify(viewer.membership));
    const heard: Heard[] = [];
    const devices = [
      { id: 1, name: "Phone", isThisDevice: true, isSignedIn: true, lastUsedAt: 0 },
      { id: 2, name: "Fire TV", isThisDevice: false, isSignedIn: true, lastUsedAt: 0 },
    ];
    const services = {
      ...servedBy(
        {
          "POST /together/playback": [200, directDecision({})],
          "GET /me/devices": [200, { devices }],
          "POST /me/casts": [201, { castId: 9, isSender: true, membershipCredential: "sender" }],
        },
        heard,
        "token",
      ),
      navigation: createNavigation(fakePlace("/play/5")),
    };
    const sockets = liveAndSession(2); // the page's session opens first, its live connection after
    const mounted = await mount(
      withServices(
        <LiveProvider>
          <GroupFilmPage {...viewer} />
          <CastDialog onClose={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.live.say({ type: "sending" });
    await sockets.live.say({ type: "devices", deviceIds: [1, 2] });
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "playing", positionMs: 70_000, rate: 1, fileId: 5 },
    });
    await settleUntil(() => buttonWithText(mounted.container, "Fire TV") !== undefined);
    await click(buttonWithText(mounted.container, "Fire TV"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toEqual({
      toDeviceId: 2,
      togetherCredential: "viewer-credential",
    });
    mounted.unmount();
  });
});

describe("a signed-in guest's leader debug view (the owner, 2026-10-29)", () => {
  it("marks the device leading and this one on the guest's page, by their own Show debug tools", async () => {
    for (const showsDebugTools of [false, true]) {
      const services = {
        ...servedBy({}, [], "token"),
        navigation: createNavigation(fakePlace("/play/5")),
      };
      const sockets = liveAndSession(2); // the page's session opens first
      const mounted = await mount(
        withServices(
          <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
            <LiveProvider>
              <GroupFilmPage {...viewer} />
            </LiveProvider>
          </DebugToolsContext>,
          { ...services, openTogetherSocket: sockets.openSocket },
        ),
      );
      await click(buttonWithText(mounted.container, "Tap to continue"));
      await sockets.live.accept();
      await sockets.session.acceptEach();
      await sockets.session.say(welcome);
      const lines = [
        ...mounted.container.querySelectorAll(".together-debug li, .together-debug p"),
      ];
      expect(lines.map((line) => line.textContent)).toEqual(
        showsDebugTools
          ? [
              "Karl: device 3 (video) · ▶ leading",
              "Gran: device 8 (video) · this device",
              "idle · leader device 3 (Karl) · why none · this device 8",
            ]
          : [],
      );
      mounted.unmount();
    }
  });
});
