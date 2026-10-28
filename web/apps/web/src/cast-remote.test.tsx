// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Casting } from "./cast-remote.tsx";
import { LiveProvider } from "./live.tsx";
import { Play } from "./play.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
  mount,
  playbackCauses,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, liveAndSession } from "./test-together.ts";

/** The cast this device sends, as GET /me/casts lists it. */
const cast = {
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

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  titleName: "Metropolis",
  durationMs: 600_000,
  isAccepted: true,
};

describe("the casting page (D364 §8, §13)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
  });

  /** The casting page, its live connection saying the cast this device sends, its session joined. */
  async function casting(
    answers: Parameters<typeof servedBy>[0],
    heard: Heard[] = [],
    sent: Readonly<Record<string, unknown>> = sending,
  ): Promise<{
    readonly services: ReturnType<typeof servedBy>;
    readonly sockets: ReturnType<typeof liveAndSession>;
  }> {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [cast] }], "GET /files/4/up-next": [200, {}], ...answers },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Casting />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending: sent });
    await sockets.session.accept();
    await sockets.session.say(welcome);
    return { services, sockets };
  }

  it("holds no player, plays and seeks the receiver, and says where its film stands", async () => {
    const { sockets } = await casting({});
    const server = sockets.session;
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "remote", role: "panel" });
    const page = mounted!.container;
    expect(page.querySelector("video")).toBeNull();
    expect(page.textContent).toContain("Metropolis");
    expect(page.textContent).toContain("Playing on Fire TV");
    expect(page.textContent).toContain("1:01 / 10:00");
    await click(buttonNamed(page, "Play"));
    expect(server.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
    await click(buttonNamed(page, "Forward 10 s"));
    expect(server.heard.at(-1)).toMatchObject({
      type: "command",
      command: "seek",
      positionMs: 71_000,
    });
  });

  it("shows how the receiver plays the film, as its state comes back: delivery, quality and link (D371 §4)", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    expect(page.querySelector(".cast-remote__playback")).toBeNull();
    const receiver = { id: 7, role: "video", isLeader: true, status: "watching" };
    await sockets.session.say({
      type: "roster",
      roster: [
        {
          id: 2,
          name: "owner",
          isHost: true,
          devices: [
            { id: 5, role: "panel" },
            {
              ...receiver,
              playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
            },
          ],
        },
      ],
    });
    expect(page.querySelector(".cast-remote__playback")?.textContent).toBe(
      "Plays it converted · Quality 4.2 Mb/s · Link about 18.0 Mb/s",
    );
    await sockets.session.say({
      type: "roster",
      roster: [
        {
          id: 2,
          name: "owner",
          isHost: true,
          devices: [{ ...receiver, playback: { delivery: "direct" } }],
        },
      ],
    });
    expect(page.querySelector(".cast-remote__playback")?.textContent).toBe(
      "Plays the file as it is",
    );
  });

  it("says the receiver is ready with nothing playing, offering Stop casting alone (D364 §8)", async () => {
    await casting({}, [], { ...sending, fileId: 0, titleName: "" });
    const page = mounted!.container;
    expect(page.textContent).toContain("Ready on Fire TV.");
    expect(buttonNamed(page, "Play")).toBeUndefined();
    expect(buttonWithText(page, "Stop casting")).toBeDefined();
  });

  it("follows what plays on the receiver as the live connection says it (D364 §3)", async () => {
    const { sockets } = await casting({ "GET /files/5/up-next": [200, {}] });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, titleName: "Cat" },
    });
    expect(mounted!.container.querySelector("h2")?.textContent).toBe("Cat");
  });

  it("plays here instead, from where the receiver stood, the receiver waiting ready (D364 §4)", async () => {
    const heard: Heard[] = [];
    const { services } = await casting(
      { "POST /me/casts/9/play": [200, { ...cast, fileId: 0 }] },
      heard,
    );
    await click(buttonWithText(mounted!.container, "Play on this device instead"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/play")?.body).toEqual({});
    expect(services.navigation.route()).toEqual({
      screen: "play",
      fileId: 4,
      atMs: 61_000,
      shouldPlayHere: true,
    });
  });

  it("leaves the controls to Watch Together's page while that is on too, leading there (D364 §11)", async () => {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "host-credential" }),
    );
    await casting({});
    const page = mounted!.container;
    expect(page.textContent).toContain("Watch Together is on");
    expect(buttonNamed(page, "Play")).toBeUndefined();
    expect(page.querySelector('a[href="/watch-together"]')).not.toBeNull();
    expect(buttonWithText(page, "Stop casting")).toBeDefined();
  });

  it("leaves a guest's cast with nothing of its own to their session's page, its receiver following the group (D371 §4)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 3, credential: "viewer-credential" }),
    );
    try {
      await casting({}, [], { ...sending, fileId: 0, titleName: "" });
      const page = mounted!.container;
      expect(page.textContent).toContain("Watch Together is on");
      expect(page.querySelector('a[href="/together"]')).not.toBeNull();
      expect(buttonWithText(page, "Stop casting")).toBeDefined();
    } finally {
      window.localStorage.removeItem("orpheum.together.viewer");
    }
  });

  it("says control was taken back, and offers to play here from where it stood, which it opens at (D364 §9)", async () => {
    const { services, sockets } = await casting({
      "GET /me/devices": [200, { devices: [] }],
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(mounted!.container.textContent).toContain("Fire TV took control back.");
    await click(buttonWithText(mounted!.container, "Play here from 1:01"));
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4, atMs: 61_000 });
  });

  it("stops casting at Stop, offering to play here from where it stood (D364 §10)", async () => {
    const heard: Heard[] = [];
    await casting(
      {
        "POST /me/casts/9/end": [200, { positionMs: 75_000, isPlaying: false }],
        "GET /me/devices": [200, { devices: [] }],
      },
      heard,
    );
    await click(buttonWithText(mounted!.container, "Stop casting"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "stopped",
    });
    expect(mounted!.container.textContent).toContain("Casting stopped.");
    expect(buttonWithText(mounted!.container, "Play here from 1:15")).not.toBeUndefined();
  });

  it("says the receiver was gone for two minutes (D364 §6)", async () => {
    const { sockets } = await casting({ "GET /me/devices": [200, { devices: [] }] });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    expect(mounted!.container.textContent).toContain(
      "Fire TV was gone for two minutes, so casting stopped.",
    );
  });

  it("lists the devices to cast to when this device casts nothing, the film it came from with it (D364 §1, §13)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [] }],
      },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <Casting fileId={4} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "sending" });
    expect(mounted.container.textContent).toContain("Cast to");
    expect(mounted.container.textContent).toContain("None of your other devices");
  });

  it("offers the film's Up next, which moves the cast on (plans/up-next.md)", async () => {
    const heard: Heard[] = [];
    const cat = { ...cast, fileId: 5, titleId: 3, titleName: "Cat", durationMs: 1_200_000 };
    const { sockets } = await casting(
      {
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
        "POST /me/casts/9/next": [200, cat],
      },
      heard,
    );
    await click(buttonWithText(mounted!.container, "Up next: S1E2 · Cat"));
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(true);
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, titleName: "Cat" },
    });
    expect(mounted!.container.querySelector("h2")?.textContent).toBe("Cat");
    expect(buttonWithText(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined();
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
