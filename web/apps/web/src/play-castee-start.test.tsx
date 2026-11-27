// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import { useNotesPlayingHere } from "./playing-here.ts";
import { castingRemote, welcome, type RemoteShown } from "./test-castee.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
  mount,
  settleAnswers,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";

/** The film playing, a minute in, as the session has it. */
const playing = { ...welcome, group: { ...welcome.group, state: "playing" } };

/** The person's devices, as the live connection tells them: this phone, 1, and the receiver, 2. */
const bothConnected = {
  type: "devices",
  deviceIds: [1, 2],
  inFrontDeviceIds: [],
  receivingDeviceIds: [2],
};
const receiverGone = { ...bothConnected, deviceIds: [1], receivingDeviceIds: [] };

/** The cast's roster: this remote (5) and the receiver's video (6), leading it, standing as status says. */
const castee = (status: string, castState?: string): Readonly<Record<string, unknown>> => ({
  type: "roster",
  roster: [
    {
      id: 2,
      name: "owner",
      isHost: true,
      devices: [
        { id: 5, role: "panel" },
        {
          id: 6,
          role: "video",
          isLeader: true,
          status,
          ...(castState === undefined ? {} : { castState }),
          playback: {
            state: "paused",
            positionMs: 61_000,
            rate: 1,
            fileId: 4,
            sound: {
              isLevellingOn: false,
              audioTracks: [],
              channels: [{ channel: "film", level: 0.5 }],
            },
          },
        },
      ],
    },
  ],
});

describe("the cast's page from the choice of a TV until its first picture (D569)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    vi.useRealTimers();
    mounted?.unmount();
    mounted = undefined;
  });

  async function casting(
    answers: Parameters<typeof castingRemote>[0] = {},
    shown: RemoteShown = {},
    heard: Heard[] = [],
  ): Promise<Awaited<ReturnType<typeof castingRemote>> & { onPlayHere: ReturnType<typeof vi.fn> }> {
    const onPlayHere = vi.fn();
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CasteePlay, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  const page = (): HTMLElement => mounted!.container;
  const pill = (): string | undefined =>
    page().querySelector(".castee-play__centre .pill")?.textContent ?? undefined;
  const cancel = (): HTMLElement | undefined => buttonWithText(page(), "Cancel");
  const isDisabled = (name: string): boolean =>
    buttonNamed(page(), name)?.getAttribute("aria-disabled") === "true";
  const step = (ms: number): void => {
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  };
  const fakeClock = (): void => {
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "performance"],
      shouldAdvanceTime: true,
    });
  };

  it("says it connects to the TV, then loads on it once reached, Cancel under it, play/pause, seeking and the menus waiting and the volume not, until its first picture (§1, §2, §6)", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say(receiverGone); // the TV's app not yet open
    expect(pill()).toBe("Connecting to Fire TV…");
    expect(cancel()).toBeDefined();
    expect(isDisabled("Pause")).toBe(true);
    await sockets.live.say(bothConnected); // the TV's app opens
    await sockets.session.say(castee("loading"));
    expect(pill()).toBe("Loading on Fire TV…");
    expect(cancel()).toBeDefined();
    expect(isDisabled("Pause")).toBe(true);
    expect(isDisabled("Back 10 s")).toBe(true);
    expect(page().querySelector(".cast-remote__buttons")?.getAttribute("aria-disabled")).toBe(
      "true",
    );
    expect(isDisabled("Volume up")).toBe(false);
    await sockets.session.say(castee("watching"));
    expect(pill()).toBeUndefined();
    expect(cancel()).toBeUndefined();
    expect(isDisabled("Pause")).toBe(false);
  });

  it("keeps the wait for a press on the TV's own remote first, in its own words, Cancel under it (D564 §2)", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say(bothConnected);
    await sockets.session.say(castee("loading", "waitingForOK"));
    expect(pill()).toBe("Waiting for someone to press OK on Fire TV");
    expect(cancel()).toBeDefined();
  });

  it("says Paused on ‹TV› as the TV is first ready paused, Play focused, until it plays (§3)", async () => {
    const { sockets } = await casting();
    await sockets.session.say(castee("loading"));
    expect(pill()).toBe("Loading on Fire TV…");
    await sockets.session.say(castee("watching"));
    expect(pill()).toBe("Paused on Fire TV");
    expect(document.activeElement).toBe(buttonNamed(page(), "Play"));
    expect(isDisabled("Play")).toBe(false);
    expect(cancel()).toBeUndefined();
    const played = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4, event: "played" };
    await sockets.session.say({ type: "group", seq: 4, group: played });
    expect(pill()).toBeUndefined();
  });

  it("says the TV is slow to start after about 20 s with no picture, the same Cancel under it, and not while it waits for OK (§4)", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    fakeClock();
    await sockets.session.say(castee("loading"));
    step(19_000);
    expect(pill()).toBe("Loading on Fire TV…");
    step(2_000);
    expect(pill()).toBe("Fire TV is slow to start");
    expect(cancel()).toBeDefined();
    await sockets.session.say(castee("loading", "waitingForOK"));
    expect(pill()).toBe("Waiting for someone to press OK on Fire TV");
    step(30_000);
    expect(pill()).toBe("Waiting for someone to press OK on Fire TV");
  });

  it("ends the cast with Cancel as Stop casting does, the film then playing here where it stood (§5)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 75_000, isPlaying: true }] },
      { welcome: playing },
      heard,
    );
    await sockets.session.say(castee("loading"));
    await click(cancel());
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts/9/end"));
    const ends = heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end");
    expect(ends.map((each) => bodyField(each, "reason"))).toEqual(["stopped"]);
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(75_000, false);
  });

  it("offers no Cancel and says nothing of a slow start for a TV lost after its start, connecting to it again (D550)", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say(bothConnected);
    await sockets.session.say(castee("watching"));
    expect(pill()).toBeUndefined();
    fakeClock();
    await sockets.live.say(receiverGone);
    await sockets.session.say({
      type: "roster",
      roster: [{ id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }] }],
    });
    expect(pill()).toBe("Connecting to Fire TV…");
    expect(cancel()).toBeUndefined();
    step(30_000);
    expect(pill()).toBe("Connecting to Fire TV…");
  });

  // As the server opens a cast's session: its group plays nothing until the TV puts the film on
  // (the core's together/cast.go openAlone), and it tells the film put on in the group before the
  // roster saying how the TV stands with it (group.go's announce).
  const noFilmYet = { ...welcome, group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 } };
  /** The cast's roster, this remote (5) and the TV's page (6), leading it, saying fields. */
  const tv = (fields: Readonly<Record<string, unknown>>): Readonly<Record<string, unknown>> => ({
    type: "roster",
    roster: [
      {
        id: 2,
        name: "owner",
        isHost: true,
        devices: [
          { id: 5, role: "panel" },
          {
            id: 6,
            role: "video",
            isLeader: true,
            playback: {
              state: "paused",
              positionMs: 0,
              rate: 1,
              fileId: 0,
              sound: {
                isLevellingOn: false,
                audioTracks: [],
                channels: [{ channel: "film", level: 0.5 }],
              },
            },
            ...fields,
          },
        ],
      },
    ],
  });
  const filmPutOn = (state: string, positionMs: number): Readonly<Record<string, unknown>> => ({
    type: "group",
    seq: 4,
    group: { state, positionMs, rate: 1, fileId: 4, event: "changedItem" },
  });

  // The lead's walk, staging, 2026-11-27, a Fire TV's browser: Cancel went and the volume waited
  // part-way through "Loading…", the group having the film before the roster said the TV loads it.
  it("keeps Cancel and the volume through the whole start as the server tells it: the TV's press, then the film put on before the roster says the TV loads it (§2, §5)", async () => {
    const { sockets } = await casting({}, { welcome: noFilmYet });
    expect(cancel()).toBeDefined();
    await sockets.live.say(bothConnected);
    await sockets.session.say(tv({ castState: "waitingForOK" }));
    expect(pill()).toBe("Waiting for someone to press OK on Fire TV");
    expect(cancel()).toBeDefined();
    await sockets.session.say(tv({ castState: "running" }));
    expect(pill()).toBe("Loading on Fire TV…");
    expect(cancel()).toBeDefined();
    expect(isDisabled("Volume up")).toBe(false);
    await sockets.session.say(filmPutOn("playing", 141_000));
    expect(pill()).toBe("Loading on Fire TV…");
    expect(cancel()).toBeDefined();
    expect(isDisabled("Volume up")).toBe(false);
    expect(isDisabled("Pause")).toBe(true);
    await sockets.session.say(tv({ castState: "running", status: "loading" }));
    expect(cancel()).toBeDefined();
    expect(isDisabled("Volume up")).toBe(false);
    await sockets.session.say(tv({ castState: "running", status: "watching" }));
    expect(pill()).toBeUndefined();
    expect(cancel()).toBeUndefined();
    expect(isDisabled("Pause")).toBe(false);
  });

  it("takes no play, pause or seek at the keys while the TV loads, as its buttons wait (§2)", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say(bothConnected);
    await sockets.session.say(castee("loading"));
    expect(pill()).toBe("Loading on Fire TV…");
    const before = sockets.session.heard.length;
    for (const key of [" ", "ArrowRight", "ArrowLeft"]) {
      act(() => {
        document.body.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
        document.body.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
      });
    }
    await settleAnswers();
    expect(
      sockets.session.heard.slice(before).filter((each) => each["type"] === "command"),
    ).toEqual([]);
  });

  // The lead's walk, staging, 2026-11-27: a second cast to a TV already open said "Loading…" with no
  // Cancel, then stood paused with no "Paused on" and nothing focused.
  it("offers Cancel to a TV already open from the start, and says Paused on ‹TV›, Play focused, as it is ready paused (§3, §5)", async () => {
    const { sockets } = await casting({}, { welcome: noFilmYet });
    await sockets.live.say(bothConnected);
    await sockets.session.say(tv({}));
    expect(pill()).toBe("Loading on Fire TV…");
    expect(cancel()).toBeDefined();
    await sockets.session.say(filmPutOn("paused", 11_000));
    expect(pill()).toBe("Loading on Fire TV…");
    expect(cancel()).toBeDefined();
    await sockets.session.say(tv({ status: "watching" }));
    expect(pill()).toBe("Paused on Fire TV");
    expect(document.activeElement).toBe(buttonNamed(page(), "Play"));
    expect(cancel()).toBeUndefined();
  });

  // The lead's walk, staging, 2026-11-27: Cancel while it loaded left the film here black, paused at
  // 0:00, the server's answer read from the cast's session before the TV had put the film on.
  it("plays the film here again where it stood here, as the person had it, as Cancel ends the start before the TV put the film on (§5)", async () => {
    function PlayedHere(): null {
      useNotesPlayingHere(true, 4, { positionMs: () => 141_000, isPaused: () => false });
      return null;
    }
    const here = await mount(<PlayedHere />);
    here.unmount(); // the film here left for the cast, at 141 s, playing
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }] },
      { welcome: noFilmYet },
      heard,
    );
    await sockets.live.say(bothConnected);
    await sockets.session.say(tv({ castState: "running" }));
    await click(cancel());
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts/9/end"));
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(141_000, false);
  });
});
