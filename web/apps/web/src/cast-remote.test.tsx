// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellNextChunkLoadMs } from "@orpheum/sdk";
import { act, type JSX, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Screen } from "./app.tsx";
import { CastRemote } from "./cast-remote.tsx";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { LiveProvider } from "./live.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import { firstAskOf, Play } from "./play.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
  enter,
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

/** Opens the remote's menu whose opener's name starts with name, as over the film. */
async function openedMenu(page: Element, name: string): Promise<void> {
  const opener = [...page.querySelectorAll("button[aria-haspopup]")].find(
    (button) => button.getAttribute("aria-label")?.startsWith(name) === true,
  );
  await click(opener);
}

/** The cast's session saying its receiver, leading it, plays as playback says. */
async function receiverSays(
  server: ReturnType<typeof liveAndSession>["session"],
  playback: Readonly<Record<string, unknown>>,
): Promise<void> {
  const receiver = { id: 7, role: "video", isLeader: true, status: "watching", playback };
  await server.say({
    type: "roster",
    roster: [{ id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }, receiver] }],
  });
}

/** The cast this device sends, as GET /me/casts lists it. */
const cast = {
  castId: 9,
  isSender: true,
  fileId: 4,
  itemId: 2,
  itemName: "Metropolis",
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

/** The cast's roster, its receiver's video leading it. */
const receiverLeads = [
  { id: 2, name: "owner", isHost: true, devices: [{ id: 6, role: "video", isLeader: true }] },
];

/** The cast this device sends, as the live connection says it, its receiver having said Use. */
const sending = {
  castId: 9,
  receiverDeviceId: 2,
  receiverName: "Fire TV",
  fileId: 4,
  itemName: "Metropolis",
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
    shown: {
      readonly sent?: Readonly<Record<string, unknown>>;
      readonly castSign?: () => ReactNode;
    } = {},
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
        <CastSignOverTheFilmContext value={shown.castSign ?? (() => null)}>
          <LiveProvider>
            <CastRemote castId={9} fileId={4} onPlayHere={onPlayHere} />
          </LiveProvider>
        </CastSignOverTheFilmContext>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending: shown.sent ?? sending });
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

  it("shows how the receiver plays the film in ⁝'s How it plays, as its state comes back: delivery, quality and link (D371 §4, D444)", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    await openedMenu(page, "Settings");
    expect(page.querySelector(".how-it-plays")).toBeNull();
    await receiverSays(sockets.session, {
      delivery: "converted",
      qualityKbps: 4200,
      linkKbps: 18_000,
    });
    expect(page.querySelector(".how-it-plays")?.textContent).toBe(
      "Plays it converted · Quality 4.2 Mb/s · Link about 18.0 Mb/s",
    );
    await receiverSays(sockets.session, { delivery: "direct" });
    expect(page.querySelector(".how-it-plays")?.textContent).toBe("Plays the file as it is");
  });

  it("lays out the full-screen overlay's rows, its bar and menus, the film's name a sign, and leaves out the bare player and Start over (D444)", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    await receiverSays(sockets.session, {
      delivery: "direct",
      sound: { isLevellingOn: false, audioTracks: [], channels: [{ channel: "film", level: 1 }] },
      picture: { subtitles: [{ key: "s1", label: "English" }], canPlayOriginal: true },
    });
    const overlay = page.querySelector(".cast-remote .player-overlay");
    expect(overlay?.querySelector(".player-overlay__top .cast-remote__sign h2")?.textContent).toBe(
      "Metropolis",
    );
    expect(overlay?.querySelector(".player-overlay__scrub input[type=range]")).not.toBeNull();
    const openers = [...(overlay?.querySelectorAll(".player-overlay__buttons button") ?? [])].map(
      (button) => button.getAttribute("aria-label")?.split(":")[0],
    );
    expect(openers).toEqual(expect.arrayContaining(["Play", "Volume", "Subtitles", "Settings"]));
    await openedMenu(page, "Settings");
    const places = [...page.querySelectorAll("dialog .list-box li")].map((li) => li.textContent);
    expect(places).toContain("Level loudness");
    expect(places.some((place) => place.includes("Start over"))).toBe(false);
    expect(places.some((place) => place.includes("bare"))).toBe(false);
  });

  it("holds the tray's Cast sign in its top row, whose dialog holds Stop casting, and no Stop of its own (D444)", async () => {
    await casting({}, [], { castSign: () => <button type="button">Cast sign</button> });
    const page = mounted!.container;
    expect(buttonWithText(page, "Stop casting")).toBeUndefined();
    const top = page.querySelector<HTMLElement>(".player-overlay__top");
    expect(top === null ? undefined : buttonWithText(top, "Cast sign")).toBeDefined();
  });

  // A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
  // after taking control back (the owner, VQ116's run).
  it("shows the receiver's sound in ⁝ and Sound track as it says it, and sends the person's choices of it to the receiver", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    const labelled = (text: string): HTMLInputElement | null | undefined =>
      [...page.querySelectorAll("label")]
        .find((label) => label.textContent === text)
        ?.querySelector("input");
    const sound = {
      isBoostOn: false,
      isLevellingOn: true,
      audioStreamId: 1,
      audioTracks: [
        { streamId: 1, language: "en", kind: "plain" },
        { streamId: 2, language: "en", containerTitle: "Director", kind: "commentary" },
      ],
    };
    await receiverSays(server, { delivery: "converted", sound });
    await openedMenu(page, "Settings");
    expect(labelled("Dialogue boost")?.checked).toBe(false);
    expect(labelled("Level loudness")?.checked).toBe(true);
    await click(labelled("Dialogue boost"));
    await click(labelled("Level loudness"));
    await openedMenu(page, "Settings"); // closes ⁝ again
    await openedMenu(page, "Sound track");
    const tracks = [...page.querySelectorAll("dialog .list-box button")];
    expect(tracks.map((each) => each.textContent)).toEqual([
      "English",
      "English · Director · commentary",
    ]);
    await click(tracks[1]);
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 2, type: "sound", sound: { kind: "boost", isOn: true } },
      { v: 2, type: "sound", sound: { kind: "levelling", isOn: false } },
      { v: 2, type: "sound", sound: { kind: "audioTrack", streamId: 2 } },
    ]);

    // The receiver says how it sounds after; a film with no boosted mix offers no boost.
    await receiverSays(server, {
      delivery: "converted",
      sound: { ...sound, isBoostOn: undefined, isLevellingOn: false, audioStreamId: 2 },
    });
    await openedMenu(page, "Settings");
    expect(labelled("Dialogue boost")).toBeUndefined();
    expect(labelled("Level loudness")?.checked).toBe(false);
  });

  it("shows the receiver's sound timing in ⁝, and moves it a step from there for a TV that plays its sound late (the owner, 2026-11-06)", async () => {
    // "Put a control in ... cast so that I can adjust the sound timing".
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    const said = (soundOffsetMs?: number): Promise<void> =>
      receiverSays(server, {
        delivery: "direct",
        sound: { isLevellingOn: false, audioTracks: [], soundOffsetMs },
      });
    await said();
    await openedMenu(page, "Settings");
    expect(page.textContent).not.toContain("Sound there");
    await said(-200);
    expect(page.textContent).toContain("Sound there: 200 ms earlier");
    await click(buttonWithText(page, "Sound 50 ms earlier"));
    await said(0);
    expect(page.textContent).toContain("Sound there: in step with the picture");
    await click(buttonWithText(page, "Sound 50 ms later"));
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 2, type: "sound", sound: { kind: "soundOffset", ms: -250 } },
      { v: 2, type: "sound", sound: { kind: "soundOffset", ms: 50 } },
    ]);
  });

  it("shows the film's level and mute on the receiver as 🔈's film slider, and sends the person's moves of them (D440 §2, §5; D444)", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    const said = (film: Readonly<Record<string, unknown>> | undefined): Promise<void> =>
      receiverSays(server, {
        delivery: "direct",
        sound: {
          isLevellingOn: false,
          audioTracks: [],
          channels: film === undefined ? undefined : [{ channel: "film", ...film }],
        },
      });
    const slider = (): HTMLInputElement | null =>
      page.querySelector<HTMLInputElement>("dialog .volume-row__film input[type=range]");
    await said(undefined);
    expect(buttonNamed(page, "Volume")).toBeUndefined();
    await said({ level: 0.6 });
    await openedMenu(page, "Volume");
    // The film's slider leads 🔈's row, as over the film (D440 §2), its mark saying what it sets.
    expect(page.querySelector("dialog .volume-row > .volume-row__film")).not.toBeNull();
    expect(page.querySelector(".volume-row__film .volume-row__mark")?.getAttribute("title")).toBe(
      "The film's volume there, under the TV's own",
    );
    expect(slider()?.value).toBe("60");

    await enter(slider(), "25");
    expect(slider()?.value).toBe("25"); // as moved here, until the receiver says
    await click(buttonNamed(page, "Mute"));
    expect(slider()?.value).toBe("0");
    // The receiver says it is muted; a drag turns its sound on with the level.
    await said({ level: 0.25, isMuted: true });
    expect(buttonNamed(page, "Sound on")).toBeDefined();
    await enter(slider(), "40");
    const channel = (level: number, isMuted: boolean): Record<string, unknown> => ({
      v: 2,
      type: "sound",
      sound: { kind: "channel", channel: "film", level, isMuted },
    });
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      channel(0.25, false),
      channel(0.25, true),
      channel(0.4, true),
      channel(0.4, false),
    ]);
  });

  it("leaves full screen as it shows, a cast started from the film full screen showing its controls on the page (D440 §5)", async () => {
    // The Play page's player put the whole page full screen, which outlives the player.
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => document.documentElement,
    });
    const exitFullscreen = vi.fn(() => Promise.resolve());
    Object.defineProperty(document, "exitFullscreen", {
      configurable: true,
      value: exitFullscreen,
    });
    try {
      await casting({});
      expect(exitFullscreen).toHaveBeenCalledOnce();
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
  });

  it("says the receiver is ready with nothing playing, as a sign alone, Stop casting in Cast's dialog (D364 §8, D444)", async () => {
    await casting({}, [], { sent: { ...sending, fileId: 0, itemName: "" } });
    const page = mounted!.container;
    expect(page.querySelector(".player-overlay__top .cast-remote__sign")?.textContent).toContain(
      "Ready on Fire TV.",
    );
    expect(buttonNamed(page, "Play")).toBeUndefined();
    expect(buttonWithText(page, "Stop casting")).toBeUndefined();
  });

  it("follows what plays on the receiver as the live connection says it (D364 §3)", async () => {
    const { sockets } = await casting({ "GET /files/5/up-next": [200, {}] });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    expect(mounted!.container.querySelector("h2")?.textContent).toBe("Cat");
  });

  it("never plays here while the cast goes on, Stop casting in Cast's dialog the one way back (D388, D444)", async () => {
    const heard: Heard[] = [];
    const { services, onPlayHere } = await casting({}, heard);
    const page = mounted!.container;
    expect(page.textContent).toContain("Playing on Fire TV");
    expect(page.textContent).not.toContain("Play on this device instead");
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /me/casts/9/play");
    expect(onPlayHere).not.toHaveBeenCalled();
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
    // The owner, 2026-11-06: "While I have watch together active, I no longer get any controls for
    // sound offset in the cast dialog", and "It shouldn't be watch together's business at all which
    // TV has what offset or playback characteristics": the receiver, a panel in the cast's session
    // while it leads the hosted one, says its playback there alone, and is worked there.
    await castSession.say({
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
              role: "panel",
              playback: {
                delivery: "converted",
                sound: { isLevellingOn: false, audioTracks: [], soundOffsetMs: -400 },
              },
            },
          ],
        },
      ],
    });
    await openedMenu(mounted.container, "Settings");
    expect(mounted.container.textContent).toContain("Sound there: 400 ms earlier");
    await click(buttonWithText(mounted.container, "Sound 50 ms earlier"));
    expect(castSession.heard.at(-1)).toMatchObject({
      type: "sound",
      sound: { kind: "soundOffset", ms: -450 },
    });
    expect(hosted.heard.some((each) => each["type"] === "sound")).toBe(false);
  });

  // The owner, staging, 2026-10-30: after a Fire TV took control back, the page offered "Play here
  // from ‹time›", its time rising on its own whatever the TV then did.
  it("plays the film here at once from where the receiver stood, paused as it was, as control is taken back there, offering nothing (D385 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    expect(mounted!.container.textContent).not.toContain("took control back");
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
  });

  it("plays the film of another page on that page, in this one's place, where the cast moved on to it, paused as the receiver had it", async () => {
    const { services, sockets } = await casting({ "GET /files/5/up-next": [200, {}] });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(services.navigation.route()).toEqual({
      screen: "play",
      fileId: 5,
      atMs: 61_000,
      shouldPlayHere: true,
      isPaused: true,
    });
  });

  // The owner, 2026-10-30: "We could just have it take over at the last known playback position once
  // it's sure the castee is gone."
  it("plays the film here, playing, from where the receiver last stood once it is gone, not where the clock has moved it to since (D385 §3)", async () => {
    vi.useFakeTimers({ toFake: ["performance", "setInterval", "clearInterval"] });
    try {
      const { sockets, onPlayHere } = await casting({});
      const playing = { state: "Playing", positionMs: 61_000, rate: 1, fileId: 4 };
      await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
      await sockets.session.say({ type: "roster", roster: receiverLeads });
      act(() => {
        vi.advanceTimersByTime(5000);
      });
      // The receiver's connection lost: the server detaches it, and no device leads the cast.
      await sockets.session.say({ type: "roster", roster: [] });
      act(() => {
        // The server sure it is gone: its minutes, shortened, as this stand-in answers no ping.
        vi.advanceTimersByTime(20_000);
      });
      await sockets.live.say({ type: "sending" });
      await sockets.session.say({ type: "ended", reason: "receiverGone" });
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(66_000, false);
      expect(mounted!.container.textContent).not.toContain("was gone");
    } finally {
      vi.useRealTimers();
    }
  });

  it("opens the film here paused, where the receiver had it paused, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledWith(61_000, true);
  });

  it("plays the film here on, where the receiver played it, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    const playing = { state: "Playing", positionMs: 61_000, rate: 1, fileId: 4 };
    await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledOnce();
    expect(onPlayHere.mock.calls[0]?.[1]).toBe(false);
  });

  it("goes home, playing nothing here, as a cast with nothing on is stopped (D446 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0 } });
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
  });

  it("goes home, saying in a pop-up there that the server no longer knows the cast, as its session is gone (D386 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "gone" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  it("goes home with no word as its cast ends with the Watch Together session it took a part in, a normal end (D389 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "sessionEnded" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(currentNotice()).toBeUndefined();
  });

  it("goes home, saying so, where the server lists the cast no more", async () => {
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [] }], "GET /files/4/up-next": [200, {}] },
      [],
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
    await sockets.live.say({ type: "sending", sending });
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  it("offers the film's Up next at its end alone, as over the film, which moves the cast on (plans/up-next.md, D444)", async () => {
    const heard: Heard[] = [];
    const cat = { ...cast, fileId: 5, itemId: 3, itemName: "Cat", durationMs: 1_200_000 };
    const { sockets } = await casting(
      {
        "GET /files/4/up-next": [
          200,
          {
            upNext: {
              episode: {
                itemId: 3,
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
    expect(buttonWithText(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined(); // at 1:01
    const atTheEnd = { state: "Paused", positionMs: 575_000, rate: 1, fileId: 4, event: "seeked" };
    await sockets.session.say({ type: "group", seq: 4, group: atTheEnd });
    await click(buttonWithText(mounted!.container, "Up next: S1E2 · Cat"));
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(true);
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
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

  /** How often the video of film 2's Play page, page, was played once loaded far ahead of it. */
  async function playsOf(page: JSX.Element): Promise<number> {
    const services = servedBy({
      "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
      "PUT /files/2/preferences": [204, undefined],
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "POST /playback/t/report": [204, undefined],
      "PUT /playback/t/wants": [204, null],
      "DELETE /playback/t": [204, null],
    });
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(page, services));
    await settleUntil(() => mounted?.container.querySelector("video") !== null);
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    let plays = 0;
    Object.defineProperty(video, "readyState", { value: 4 });
    Object.defineProperty(video, "networkState", { value: 2 }); // loading
    Object.defineProperty(video, "buffered", {
      get: () => ({ length: 1, start: () => 0, end: () => 7200 }),
    });
    Object.defineProperty(video, "play", {
      value: async (): Promise<void> => {
        plays += 1;
        await Promise.resolve();
      },
    });
    tellNextChunkLoadMs(video, () => ({ loadMs: 100, from: "confident" }));
    for (const event of ["loadstart", "loadedmetadata", "canplay", "progress"]) {
      await act(async () => {
        video.dispatchEvent(new Event(event));
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }
    mounted.unmount();
    mounted = undefined;
    return plays;
  }

  it("waits paused there where the receiver had the film paused as its sender took it over, and plays at once where it played (D385 §2)", async () => {
    expect(await playsOf(<Play fileId={2} atMs={982_680} isPaused />)).toBe(0);
    expect(await playsOf(<Play fileId={2} atMs={982_680} />)).toBeGreaterThan(0);
  });

  it("opens a cast received paused there where its sender had the film paused, and playing where it played (the owner, 2026-11-08)", () => {
    const cast = {
      castId: 9,
      membershipCredential: "receiver",
      fileId: 2,
      itemName: "Metropolis",
      positionMs: 61_000,
      fromDeviceId: 1,
      fromDeviceName: "Pixel",
    };
    const nothingGiven = { atMs: undefined, round: 0 };
    const receiving = (isPaused?: boolean) => ({
      cast: isPaused === undefined ? cast : { ...cast, isPaused },
      onLeave: () => undefined,
    });
    expect(firstAskOf(receiving(true), nothingGiven)).toEqual({
      cause: "playPage",
      atMs: 61_000,
      isPaused: true,
    });
    expect(firstAskOf(receiving(), nothingGiven)).toEqual({
      cause: "playPage",
      atMs: 61_000,
      isPaused: false,
    });
  });

  it("waits paused there from its address, as on another film's page the cast had moved on to (D385 §2)", async () => {
    const route = { screen: "play", fileId: 2, atMs: 982_680, shouldPlayHere: true } as const;
    expect(await playsOf(<Screen route={{ ...route, isPaused: true }} />)).toBe(0);
    expect(await playsOf(<Screen route={route} />)).toBeGreaterThan(0);
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
