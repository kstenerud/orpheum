// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, welcome, type RemoteShown } from "./test-castee.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
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
});
