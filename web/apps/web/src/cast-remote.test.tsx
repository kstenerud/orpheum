// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CastRemote } from "./cast-remote.tsx";
import { LiveProvider } from "./live.tsx";
import { Play } from "./play.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  choose,
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

describe("a Play page's video area as the cast's remote (D364 §8; D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
  });

  /**
   * The remote on film 4's Play page, its live connection saying the cast this device sends, its
   * session joined; what it was told of playing the film here again.
   */
  async function casting(
    answers: Parameters<typeof servedBy>[0],
    heard: Heard[] = [],
    sent: Readonly<Record<string, unknown>> = sending,
  ): Promise<{
    readonly services: ReturnType<typeof servedBy>;
    readonly sockets: ReturnType<typeof liveAndSession>;
    readonly onPlayHere: ReturnType<typeof vi.fn>;
  }> {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [cast] }], "GET /files/4/up-next": [200, {}], ...answers },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    const onPlayHere = vi.fn();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastRemote castId={9} fileId={4} onPlayHere={onPlayHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending: sent });
    await sockets.session.accept();
    await sockets.session.say(welcome);
    return { services, sockets, onPlayHere };
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
    expect(page.textContent).toContain("1:11 / 10:00"); // shown at once, sent once it rests
    await settleUntil(() => server.heard.at(-1)?.["command"] === "seek");
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

  // A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
  // after taking control back (the owner, VQ116's run).
  it("shows the receiver's sound as it says it, and sends the person's choices of it to the receiver", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    const labelled = (text: string): HTMLInputElement | null | undefined =>
      [...page.querySelectorAll("label")]
        .find((label) => label.textContent === text)
        ?.querySelector("input");
    expect(page.querySelector(".cast-remote__sound")).toBeNull();
    const sound = {
      isBoostOn: false,
      isLevellingOn: true,
      audioStreamId: 1,
      audioTracks: [
        { streamId: 1, language: "en", kind: "plain" },
        { streamId: 2, language: "en", containerTitle: "Director", kind: "commentary" },
      ],
    };
    const roster = (said: Readonly<Record<string, unknown>>): Record<string, unknown> => ({
      type: "roster",
      roster: [
        {
          id: 2,
          name: "owner",
          isHost: true,
          devices: [
            { id: 5, role: "panel" },
            {
              id: 7,
              role: "video",
              isLeader: true,
              playback: { delivery: "converted", sound: said },
            },
          ],
        },
      ],
    });
    await server.say(roster(sound));
    expect(labelled("Dialogue boost")?.checked).toBe(false);
    expect(labelled("Level loudness")?.checked).toBe(true);
    const select = page.querySelector<HTMLSelectElement>(".cast-remote__sound select");
    expect([...(select?.options ?? [])].map((option) => option.textContent)).toEqual([
      "English",
      "English · Director · commentary",
    ]);
    expect(select?.value).toBe("1");

    await click(labelled("Dialogue boost"));
    await click(labelled("Level loudness"));
    await choose(select, "2");
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 2, type: "sound", sound: { kind: "boost", isOn: true } },
      { v: 2, type: "sound", sound: { kind: "levelling", isOn: false } },
      { v: 2, type: "sound", sound: { kind: "audioTrack", streamId: 2 } },
    ]);

    // The receiver says how it sounds after; a film with no boosted mix offers no boost.
    await server.say(
      roster({ ...sound, isBoostOn: undefined, isLevellingOn: false, audioStreamId: 2 }),
    );
    expect(labelled("Dialogue boost")).toBeUndefined();
    expect(labelled("Level loudness")?.checked).toBe(false);
    expect(page.querySelector<HTMLSelectElement>(".cast-remote__sound select")?.value).toBe("2");
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

  it("plays here instead, on the page it shows on, from where the receiver stood, the receiver waiting ready (D364 §4, D384 §2)", async () => {
    const heard: Heard[] = [];
    const { services, onPlayHere } = await casting(
      { "POST /me/casts/9/play": [200, { ...cast, fileId: 0 }] },
      heard,
    );
    await click(buttonWithText(mounted!.container, "Play on this device instead"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/play")?.body).toEqual({});
    expect(onPlayHere).toHaveBeenCalledWith(61_000);
    expect(services.navigation.route()).toEqual({ screen: "films" });
  });

  it("works the session this tab hosts while Watch Together is on too, its receiver the host's player (D364 §11)", async () => {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "host-credential" }),
    );
    const live = fakeTogetherServer();
    const hosted = fakeTogetherServer();
    const castSession = fakeTogetherServer();
    let opened = 0;
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [cast] }], "GET /files/4/up-next": [200, {}] },
      [],
      "token",
    );
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastRemote castId={9} fileId={4} onPlayHere={vi.fn()} />
        </LiveProvider>,
        {
          ...services,
          openTogetherSocket: (listeners) => {
            opened += 1;
            const server = [live, hosted][opened - 1] ?? castSession;
            return server.openSocket(listeners);
          },
        },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    await hosted.accept();
    await hosted.say({ ...welcome, sessionId: 3 });
    await castSession.accept();
    await castSession.say(welcome);
    expect(hosted.heard[0]).toMatchObject({ credential: "host-credential", role: "panel" });
    await click(buttonNamed(mounted.container, "Play"));
    expect(hosted.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
    expect(castSession.heard.some((each) => each["type"] === "command")).toBe(false);
  });

  it("says control was taken back, and offers to play here from where it stood, on the page it shows on (D364 §9)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(mounted!.container.textContent).toContain("Fire TV took control back.");
    expect(onPlayHere).not.toHaveBeenCalled();
    await click(buttonWithText(mounted!.container, "Play here from 1:01"));
    expect(onPlayHere).toHaveBeenCalledWith(61_000);
  });

  it("plays the film of another page on that page, in this one's place, where the cast moved on to it", async () => {
    const { services, sockets } = await casting({ "GET /files/5/up-next": [200, {}] });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, titleName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await click(buttonWithText(mounted!.container, "Play here from 1:01"));
    expect(services.navigation.route()).toEqual({
      screen: "play",
      fileId: 5,
      atMs: 61_000,
      shouldPlayHere: true,
    });
  });

  it("stops casting at Stop, the film playing here again at once from where the receiver stood (D384 §2)", async () => {
    const heard: Heard[] = [];
    const { onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 75_000, isPlaying: false }] },
      heard,
    );
    await click(buttonWithText(mounted!.container, "Stop casting"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "stopped",
    });
    expect(onPlayHere).toHaveBeenCalledWith(75_000);
  });

  it("plays the film here again as the cast is stopped elsewhere, as from Cast's dialog (D384 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledWith(61_000);
  });

  it("says the receiver was gone for two minutes (D364 §6)", async () => {
    const { sockets } = await casting({});
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    expect(mounted!.container.textContent).toContain(
      "Fire TV was gone for two minutes, so casting stopped.",
    );
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
