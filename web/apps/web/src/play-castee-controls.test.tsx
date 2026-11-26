// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { CasteePlay } from "./play-castee.tsx";
import { startCast } from "./cast.ts";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { createHiddenNeed } from "./hidden-need.ts";
import { LiveProvider } from "./live.tsx";
import {
  heldUpright,
  openedMenu,
  receiverSays,
  cast,
  wayUp,
  signOf,
  welcome,
  sending,
  catNext,
  castOnCat,
  pausedAt,
  upNextCard,
  castingRemote,
  type RemoteShown,
  catReads,
  filmReads,
  receiverLeads,
} from "./test-castee.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleAnswers,
  settleUntil,
  type Heard,
  type Mounted,
  isSwitchOn,
  switchNamed,
} from "./test-dom.tsx";
import { useServices } from "./services.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, liveAndSession } from "./test-together.ts";

/** A cast started from this page, its remote drawn once starting it answered, as Play draws it. */
function StartedHere(): JSX.Element {
  const { api } = useServices();
  const [isStarted, setIsStarted] = useState(false);
  if (isStarted) {
    return <CasteePlay castId={9} fileId={4} onPlayHere={() => undefined} />;
  }
  const start = (): void => {
    void startCast(api, 2, undefined).then(() => {
      setIsStarted(true);
    });
  };
  return (
    <button type="button" onClick={start}>
      Start the cast
    </button>
  );
}

/** Longer than any wait the remote once gave its receiver up after (D554 §1), none now (D555 §3). */
const longWaitMs = 30_000;

describe("a Play page's video area as the cast's remote (D364 §8; D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
    vi.unstubAllGlobals();
  });

  /** The remote on film 4's Play page (castingRemote), the one this describe unmounts. */
  async function casting(
    answers: Parameters<typeof castingRemote>[0],
    heard?: Heard[],
    shown: RemoteShown = {},
  ): Promise<Awaited<ReturnType<typeof castingRemote>> & { onPlayHere: ReturnType<typeof vi.fn> }> {
    const onPlayHere = vi.fn();
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CasteePlay, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  it("opens on what starting the cast answered, asking for this device's casts no more (D478)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "POST /me/casts": [200, cast], "GET /files/4/up-next": [200, {}], ...filmReads },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    mounted = await mount(
      withServices(
        <CastSignOverTheFilmContext value={() => null}>
          <LiveProvider>
            <StartedHere />
          </LiveProvider>
        </CastSignOverTheFilmContext>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await click(buttonWithText(mounted.container, "Start the cast"));
    await settleUntil(() => buttonWithText(mounted!.container, "Start the cast") === undefined);
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending });
    await settleUntil(
      () => mounted?.container.querySelector(".castee-play") instanceof HTMLElement,
    );
    await settleAnswers();
    const asked = heard.map((each) => each.methodAndPath);
    expect(asked).toContain("POST /me/casts");
    expect(asked).not.toContain("GET /me/casts");
  });

  it("marks its page as needing its connection while hidden for as long as it is up, no film playing here (D482 §3)", async () => {
    const hiddenNeed = createHiddenNeed(document);
    const services = servedBy(
      { "GET /me/casts": [200, { casts: [cast] }], "GET /files/4/up-next": [200, {}] },
      [],
      "token",
    );
    const sockets = liveAndSession();
    expect(hiddenNeed.isNeeded()).toBe(false);
    mounted = await mount(
      withServices(
        <CastSignOverTheFilmContext value={() => null}>
          <LiveProvider>
            <CasteePlay castId={9} fileId={4} onPlayHere={vi.fn()} />
          </LiveProvider>
        </CastSignOverTheFilmContext>,
        { ...services, openTogetherSocket: sockets.openSocket, hiddenNeed },
      ),
    );
    expect(mounted.container.querySelector("video")).toBeNull();
    expect(hiddenNeed.isNeeded()).toBe(true);
    mounted.unmount();
    mounted = undefined;
    expect(hiddenNeed.isNeeded()).toBe(false);
  });

  it("holds no player, plays and seeks the receiver, and says where its film stands", async () => {
    const { sockets } = await casting({});
    const server = sockets.session;
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "remote", role: "panel" });
    const page = mounted!.container;
    expect(page.querySelector("video")).toBeNull();
    // The owner, 2026-11-23: the back links as the player's, under them "On: <device>".
    await settleUntil(() => wayUp(page) !== undefined);
    expect(wayUp(page)).toBe("Metropolis");
    expect(signOf(page)).toBe("On: Fire TV");
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

  // A menu with nothing in it is not drawn (the owner, 2026-11-23): no ⁝ until the receiver says.
  it("shows how the receiver plays the film in ⁝'s How it plays, as its state comes back: delivery, quality and link, and no ⁝ before (D371 §4, D444)", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    expect(page.querySelector("[aria-haspopup][aria-label='Settings']")).toBeNull();
    expect(page.querySelector(".how-it-plays")).toBeNull();
    await receiverSays(sockets.session, {
      delivery: "converted",
      qualityKbps: 4200,
      linkKbps: 18_000,
    });
    await openedMenu(page, "Settings");
    expect(page.querySelector(".how-it-plays")?.textContent).toBe(
      "Playing: Converted 4.2 Mb/s · Connection: 18.0 Mb/s",
    );
    await receiverSays(sockets.session, { delivery: "direct" });
    expect(page.querySelector(".how-it-plays")?.textContent).toBe("Playing: Original");
  });

  it("moves the cast on to the next episode at ⏭, where one can play next", async () => {
    const heard: Heard[] = [];
    await casting(
      {
        "GET /files/4/up-next": [200, catNext],
        "POST /me/casts/9/next": [200, { ...cast, fileId: 5, itemId: 3, itemName: "Cat" }],
      },
      heard,
    );
    await settleUntil(() => buttonNamed(mounted!.container, "Next episode") !== undefined);
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(false);
    await click(buttonNamed(mounted!.container, "Next episode"));
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(true);
  });

  it("offers no ⏭ where nothing can play next", async () => {
    await casting({});
    await settleAnswers();
    expect(buttonNamed(mounted!.container, "Next episode")).toBeUndefined();
  });

  // D538 §7 (the owner, 2026-11-22): the signs end the buttons row, as the overlay's do, only where
  // the top bar is hidden (the owner, 2026-11-23); the remote's page keeps the bar, its Cast sign
  // the one whose dialog holds Stop casting.
  it("holds no Stop of its own, and leaves the tray's Cast sign to the top bar it keeps, as over a film in the page (D444)", async () => {
    await casting({}, [], { castSign: () => <button type="button">Cast sign</button> });
    const page = mounted!.container;
    expect(buttonWithText(page, "Stop casting")).toBeUndefined();
    const buttons = page.querySelector<HTMLElement>(".cast-remote__buttons");
    expect(buttons).not.toBeNull();
    expect(buttons === null ? undefined : buttonWithText(buttons, "Cast sign")).toBeUndefined();
    const top = page.querySelector<HTMLElement>(".castee-play__top");
    expect(top).not.toBeNull();
    expect(top === null ? undefined : buttonWithText(top, "Cast sign")).toBeUndefined();
  });

  // A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
  // after taking control back (the owner, VQ116's run).
  it("shows the receiver's sound in the Sound menu and its Sound track as it says it, and sends the person's choices of it to the receiver", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    const labelled = (text: string): HTMLElement | undefined => switchNamed(page, text);
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
    expect(labelled("Dialogue boost")).toBeUndefined(); // ⁝ holds no sound setting (D538 §8)
    expect(labelled("Level loudness")).toBeUndefined();
    await openedMenu(page, "Settings"); // closes ⁝ again
    await openedMenu(page, "Sound");
    expect(isSwitchOn(labelled("Dialogue boost"))).toBe(false);
    expect(isSwitchOn(labelled("Level loudness"))).toBe(true);
    await click(labelled("Dialogue boost"));
    await click(labelled("Level loudness"));
    const shown = (): HTMLElement[] =>
      [...page.querySelectorAll<HTMLElement>('dialog .list-box [role="button"]')].filter(
        (each) => each.closest("[hidden]") === null,
      );
    await click(shown().find((each) => each.textContent === "Sound track: English›"));
    const tracks = shown().slice(1);
    expect(shown()[0]?.textContent).toBe("‹Sound track");
    expect(tracks.map((each) => each.textContent)).toEqual([
      "English",
      "English · Director · commentary",
    ]);
    await click(tracks[1]);
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 3, type: "sound", sound: { kind: "boost", isOn: true } },
      { v: 3, type: "sound", sound: { kind: "levelling", isOn: false } },
      { v: 3, type: "sound", sound: { kind: "audioTrack", streamId: 2 } },
    ]);

    // The receiver says how it sounds after; a film with no boosted mix offers no boost.
    await receiverSays(server, {
      delivery: "converted",
      sound: { ...sound, isBoostOn: undefined, isLevellingOn: false, audioStreamId: 2 },
    });
    if (page.querySelector("dialog") === null) {
      await openedMenu(page, "Sound");
    }
    expect(labelled("Dialogue boost")).toBeUndefined();
    expect(isSwitchOn(labelled("Level loudness"))).toBe(false);
  });

  // A Chromecast, a guest receiver, is refused the person's routes (D465 §2): its sender keeps what
  // the person chooses on its remote, so the next cast starts with it (VQ407 step 5).
  it("keeps for the person each sound and subtitle choice sent to a guest receiver, as the Play page keeps it", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting(
      { "PUT /me/preferences": [204, {}], "PUT /files/4/preferences": [204, {}] },
      heard,
      { sent: { ...sending, isGuestReceiver: true } },
    );
    const page = mounted!.container;
    const server = sockets.session;
    await receiverSays(server, {
      delivery: "converted",
      sound: {
        isBoostOn: false,
        isLevellingOn: true,
        audioStreamId: 1,
        audioTracks: [
          { streamId: 1, language: "en", kind: "plain" },
          { streamId: 2, language: "de", kind: "plain" },
        ],
      },
    });
    await openedMenu(page, "Sound");
    await click(switchNamed(page, "Dialogue boost"));
    await click(switchNamed(page, "Level loudness"));

    const kept = (): unknown[] =>
      heard
        .filter((each) => each.methodAndPath.startsWith("PUT "))
        .map((each) => [each.methodAndPath, each.body]);
    await settleUntil(() => kept().length === 2);
    expect(kept()).toEqual([
      ["PUT /me/preferences", { isDialogueBoostOn: true }],
      ["PUT /me/preferences", { isLevellingOn: false }],
    ]);
  });

  it("shows the receiver's sound timing as That device under the Sound menu's Sound timing, not in ⁝, and moves it a step from there for a TV that plays its sound late (the owner, 2026-11-06)", async () => {
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
    await openedMenu(page, "Sound");
    expect(page.textContent).not.toContain("Sound there");
    expect(page.querySelector("dialog .sound-menu__heading")).toBeNull();
    await said(-200);
    await openedMenu(page, "Sound"); // closes it
    await openedMenu(page, "Settings");
    expect(page.textContent).not.toContain("Sound there"); // not ⁝'s (D538 §8)
    await openedMenu(page, "Settings"); // closes ⁝ again
    await openedMenu(page, "Sound");
    expect(page.querySelector("dialog .sound-menu__heading")?.textContent).toBe("Sound timing");
    expect(page.querySelector("dialog .stepper")?.textContent).toContain("That device");
    expect(page.textContent).toContain("Sound there: 200 ms earlier");
    await click(buttonNamed(page, "That device earlier"));
    expect(page.querySelector("dialog")).not.toBeNull(); // kept open for the next step
    await said(-250); // the receiver took the step
    await said(0); // and later says it is in step: nothing is outstanding, its word is adopted
    expect(page.textContent).toContain("Sound there: in step with the picture");
    await click(buttonNamed(page, "That device later"));
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 3, type: "sound", sound: { kind: "soundOffset", ms: -250 } },
      { v: 3, type: "sound", sound: { kind: "soundOffset", ms: 50 } },
    ]);
  });

  it("steps the receiver's sound timing from the value last pressed, not the one the receiver last said, for two quick presses", async () => {
    const { sockets } = await casting({});
    const page = mounted!.container;
    const server = sockets.session;
    await receiverSays(server, {
      delivery: "direct",
      sound: { isLevellingOn: false, audioTracks: [], soundOffsetMs: -200 },
    });
    await openedMenu(page, "Sound");
    await click(buttonNamed(page, "That device earlier"));
    await click(buttonNamed(page, "That device earlier"));
    expect(page.textContent).toContain("Sound there: 300 ms earlier");
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      { v: 3, type: "sound", sound: { kind: "soundOffset", ms: -250 } },
      { v: 3, type: "sound", sound: { kind: "soundOffset", ms: -300 } },
    ]);
  });

  it("sets the film's level on the castee from the pad's Volume down, Mute and Volume up, sending each move, and draws neither before the castee says its level (D555 §10)", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
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
    await said(undefined);
    // No level said: no volume's row.
    expect(buttonNamed(page, "Mute")).toBeUndefined();
    expect(buttonNamed(page, "Volume up")).toBeUndefined();
    await said({ level: 0.6 });
    expect(buttonNamed(page, "Mute")?.getAttribute("aria-pressed")).toBe("false");
    await click(buttonNamed(page, "Volume up"));
    await click(buttonNamed(page, "Mute"));
    // The receiver says it is muted; the volume stepped up turns its sound on with the level.
    await said({ level: 0.7, isMuted: true });
    expect(buttonNamed(page, "Mute")?.getAttribute("aria-pressed")).toBe("true");
    await click(buttonNamed(page, "Volume up"));
    const channel = (level: number, isMuted: boolean): Record<string, unknown> => ({
      v: 3,
      type: "sound",
      sound: { kind: "channel", channel: "film", level, isMuted },
    });
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      channel(0.7, false),
      channel(0.7, true),
      channel(0.7, false),
      channel(0.8, false),
    ]);
  });

  it("says it is reaching the receiver while it is not reached, its pad and menus taking no press, and ends nothing itself however long it goes unreached, the server's end alone (D548 §9, D555 §3)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: false }] },
      heard,
    );
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    const page = mounted!.container;
    const pill = (): string | undefined =>
      page.querySelector(".castee-play__centre .pill")?.textContent ?? undefined;
    expect(pill()).toBeUndefined();
    expect(buttonNamed(page, "Play")?.hasAttribute("aria-disabled")).toBe(false);
    // The page's own waits (a turn of the event loop) run as real time does; the 15 s is stepped.
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      // The receiver no longer heard, its leader gone from the cast's roster: not the remote's own
      // connection dropping, which says it can't reach the server (D553 §4).
      await sockets.session.say({ type: "roster", roster: [] });
      expect(pill()).toBe("Reaching Fire TV…");
      expect(buttonNamed(page, "Play")?.getAttribute("aria-disabled")).toBe("true");
      expect(page.querySelector(".cast-remote__buttons")?.getAttribute("aria-disabled")).toBe(
        "true",
      );
      const ends = (): unknown[] =>
        heard
          .filter((each) => each.methodAndPath === "POST /me/casts/9/end")
          .map((each) => bodyField(each, "reason"));
      act(() => {
        vi.advanceTimersByTime(longWaitMs);
      });
      expect(page.textContent).not.toContain("Casting ended");
      expect(ends()).toEqual([]);
      expect(pill()).toBe("Reaching Fire TV…");
    } finally {
      vi.useRealTimers();
    }
  });

  it("leaves full screen as a film shows on the castee, drawing the remote's pad as out of it, and leaves it with nothing playing there: casting is never full screen (the owner, 2026-11-26)", async () => {
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
      const page = mounted!.container;
      expect(exitFullscreen).toHaveBeenCalledOnce();
      expect(page.querySelector(".castee-play__page > .remote-pad")).not.toBeNull();
      expect(page.querySelector(".player-overlay")).toBeNull();
      expect(buttonNamed(page, "Full screen")).toBeUndefined();
      exitFullscreen.mockClear();
      mounted?.unmount();
      mounted = undefined;
      // Ready on, nothing plays there: a page and no picture.
      await casting({}, [], { sent: { ...sending, fileId: 0, itemName: "" } });
      expect(exitFullscreen).toHaveBeenCalledOnce();
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
  });

  it("says the receiver is ready with nothing playing, as a sign alone, Stop casting in Cast's dialog (D364 §8, D444)", async () => {
    await casting({}, [], { sent: { ...sending, fileId: 0, itemName: "" } });
    const page = mounted!.container;
    expect(page.querySelector(".castee-play__top .castee-play__sign")?.textContent).toContain(
      "Ready on Fire TV.",
    );
    expect(buttonNamed(page, "Play")).toBeUndefined();
    expect(buttonWithText(page, "Stop casting")).toBeUndefined();
  });

  it("follows what plays on the receiver as the live connection says it (D364 §3)", async () => {
    const { sockets } = await casting({ "GET /files/5/up-next": [200, {}], ...catReads });
    await settleUntil(() => wayUp(mounted!.container) === "Metropolis");
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await settleUntil(() => wayUp(mounted!.container) !== "Metropolis");
    expect(wayUp(mounted!.container)).toBe("S1E2 Cat");
  });

  it("never plays here while the cast goes on, Stop casting in Cast's dialog the one way back (D388, D444)", async () => {
    const heard: Heard[] = [];
    const { services, onPlayHere } = await casting({}, heard);
    const page = mounted!.container;
    expect(signOf(page)).toBe("On: Fire TV");
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
      {
        "GET /me/casts": [200, { casts: [cast] }],
        "GET /files/4/up-next": [200, {}],
        ...filmReads,
      },
      [],
      "token",
    );
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CasteePlay castId={9} fileId={4} onPlayHere={vi.fn()} />
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
    await openedMenu(mounted.container, "Sound");
    expect(mounted.container.textContent).toContain("Sound there: 400 ms earlier");
    await click(buttonNamed(mounted.container, "That device earlier"));
    expect(castSession.heard.at(-1)).toMatchObject({
      type: "sound",
      sound: { kind: "soundOffset", ms: -450 },
    });
    expect(hosted.heard.some((each) => each["type"] === "sound")).toBe(false);
  });

  it("offers the film's Up next at its end alone, as over the film, which moves the cast on (plans/up-next.md, D444)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting(
      {
        "GET /files/4/up-next": [200, catNext],
        "GET /files/5/up-next": [200, {}],
        "POST /me/casts/9/next": [200, castOnCat],
        ...catReads,
      },
      heard,
    );
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined(); // at 1:01
    await sockets.session.say({ type: "group", seq: 4, group: pausedAt(575_000) });
    await click(upNextCard(mounted!.container, "Up next: S1E2 · Cat"));
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(true);
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await settleUntil(() => wayUp(mounted!.container) === "S1E2 Cat");
    expect(wayUp(mounted!.container)).toBe("S1E2 Cat");
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined();
  });

  // Its ✕ puts it away on this device alone (D548 §3, amending the owner's 2026-11-23 none).
  it("offers its Up next with a ✕, Dismiss, which puts it away here until the film is moved back from its end, offered again as it nears it anew (D538 §1, D548 §3)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting({ "GET /files/4/up-next": [200, catNext] }, heard);
    await sockets.session.say({ type: "group", seq: 4, group: pausedAt(575_000) });
    expect(mounted!.container.textContent).toContain("Play now"); // no count on a remote
    expect(mounted!.container.querySelectorAll(".offer-card button")).toHaveLength(2);
    await click(buttonNamed(mounted!.container, "Dismiss"));
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined();
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/next")).toBe(false);
    await sockets.session.say({ type: "group", seq: 5, group: pausedAt(574_000) });
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined(); // still at its end
    await sockets.session.say({ type: "group", seq: 6, group: pausedAt(100_000) });
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeUndefined();
    await sockets.session.say({ type: "group", seq: 7, group: pausedAt(575_000) });
    expect(upNextCard(mounted!.container, "Up next: S1E2 · Cat")).toBeDefined();
  });

  it("reads what plays around the cast's film once, for its ⏮ and its Up next both (D477 §2)", async () => {
    const heard: Heard[] = [];
    const pilot = { itemId: 2, groupNumber: 1, numberInGroup: 1, fileId: 3, isPartway: false };
    const { sockets } = await casting(
      {
        "GET /files/4/up-next": [200, { ...catNext, previous: pilot }],
      },
      heard,
    );
    await sockets.session.say({ type: "group", seq: 4, group: pausedAt(575_000) });
    await settleUntil(() => upNextCard(mounted!.container, "Up next: S1E2 · Cat") !== undefined);
    expect(buttonNamed(mounted!.container, "Previous episode")).toBeDefined();
    const reads = heard.filter((each) => each.methodAndPath === "GET /files/4/up-next");
    expect(reads).toHaveLength(1);
  });
});
