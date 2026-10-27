// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Casting } from "./cast-remote.tsx";
import { Play } from "./play.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  playbackCauses,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** The cast this device sends, as GET /me/casts lists it. */
const sent = {
  castId: 9,
  isSender: true,
  fileId: 4,
  titleId: 2,
  titleName: "Metropolis",
  durationMs: 600_000,
  senderDeviceId: 1,
  senderName: "Pixel",
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  membershipCredential: "remote",
};

/** The remote's welcome, the receiver's film paused at a minute. */
const welcome = {
  type: "welcome",
  seq: 3,
  sessionId: 9,
  memberId: 2,
  deviceId: 5,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "Paused", positionMs: 61_000, rate: 1, fileId: 4 },
  roster: [],
};

describe("the remote of a cast (D337 §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("holds no player, plays and seeks the receiver, and says where its film stands", async () => {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [sent] }], "GET /files/4/up-next": [200, {}] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(<Casting />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "remote", role: "panel" });
    await server.say(welcome);
    expect(mounted.container.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Metropolis");
    expect(mounted.container.textContent).toContain("Playing on Fire TV");
    expect(mounted.container.textContent).toContain("1:01 / 10:00");
    await click(buttonWithText(mounted.container, "Play"));
    expect(server.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
    await click(buttonWithText(mounted.container, "Forward 10 s"));
    expect(server.heard.at(-1)).toMatchObject({
      type: "command",
      command: "seek",
      positionMs: 71_000,
    });
  });

  it("says control was taken back, and offers to play here from where it stood, which it opens at (§9 Q5)", async () => {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [sent] }], "GET /files/4/up-next": [200, {}] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(<Casting />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "takenBack" });
    expect(mounted.container.textContent).toContain("Fire TV took control back.");
    await click(buttonWithText(mounted.container, "Play here from 1:01"));
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4, atMs: 61_000 });
  });

  it("stops casting at Stop, the receiver told", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [sent] }],
        "GET /files/4/up-next": [200, {}],
        "POST /me/casts/9/end": [200, { positionMs: 75_000, isPlaying: false }],
      },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(<Casting />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    await server.say(welcome);
    await click(buttonWithText(mounted.container, "Stop casting"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "stopped",
    });
    expect(buttonWithText(mounted.container, "Play here from 1:15")).not.toBeUndefined();
  });

  it("says when this device casts nothing", async () => {
    const services = servedBy({ "GET /me/casts": [200, { casts: [] }] }, [], "token");
    mounted = await mount(withServices(<Casting />, services));
    expect(mounted.container.textContent).toContain("This device is not casting anything.");
  });

  it("offers the film's Up next, which moves the cast on, and follows the receiver to it (plans/up-next.md)", async () => {
    const heard: Heard[] = [];
    const cat = { ...sent, fileId: 5, titleId: 3, titleName: "Cat", durationMs: 1_200_000 };
    const bag = { ...sent, fileId: 6, titleId: 4, titleName: "Bag", durationMs: 1_300_000 };
    const services = servedBy(
      {
        "GET /me/casts": [
          [200, { casts: [sent] }],
          [200, { casts: [bag] }],
        ],
        "GET /files/4/up-next": [
          200,
          {
            upNext: {
              episode: {
                titleId: 3,
                name: "Cat",
                groupNumber: 1,
                numberInGroup: 2,
                fileId: 5,
                isPartway: false,
              },
              ordering: "aired",
            },
          },
        ],
        "GET /files/5/up-next": [200, {}],
        "GET /files/6/up-next": [200, {}],
        "POST /me/casts/9/next": [200, cat],
      },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(<Casting />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    await server.say(welcome);
    await click(buttonWithText(mounted.container, "Up next: S1E2 · Cat"));
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(true);
    expect(mounted.container.querySelector("h1")?.textContent).toBe("Cat");
    expect(buttonWithText(mounted.container, "Up next: S1E2 · Cat")).toBeUndefined();
    // The receiver's own Up next played the one after: its session says another file plays.
    await server.say({ type: "group", seq: 4, group: { ...welcome.group, fileId: 6 } });
    await settleUntil(() => mounted?.container.querySelector("h1")?.textContent === "Bag");
    expect(mounted.container.textContent).toContain("Bag");
  });
});

describe("playing here from where a cast stood (§9 Q5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("starts the film there, not where the server last heard it was, and asks saying so", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
        "PUT /files/2/preferences": [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    // A Play here mid-film played from 0:00 (staging, 2026-10-27).
    mounted = await mount(withServices(<Play fileId={2} atMs={982_680} />, services));
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 1, configurable: true });
    video.dispatchEvent(new Event("loadedmetadata"));
    expect(video.currentTime).toBeCloseTo(982.68);
    const asked = heard.filter((each) => each.methodAndPath === "POST /playback");
    expect(playbackCauses(heard)).toEqual(["castOver"]);
    expect(asked.map((each) => bodyField(each, "startsAtMs"))).toEqual([982_680]);
  });
});
