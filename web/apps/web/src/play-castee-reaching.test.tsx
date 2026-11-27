// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import {
  isCastSessionLost,
  tellCastSessionLinked,
  tellCastSessionLost,
} from "./cast-session-lost.ts";
import {
  castingRemote,
  heldUpright,
  receiverLeads,
  sending,
  welcome,
  type RemoteShown,
} from "./test-castee.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";

/** The film playing, a minute in, as the receiver has it. */
const playing = { ...welcome, group: { ...welcome.group, state: "playing" } };

/** The person's devices, as the live connection tells them: this phone, 1, and the receiver, 2. */
const bothConnected = {
  type: "devices",
  deviceIds: [1, 2],
  inFrontDeviceIds: [],
  receivingDeviceIds: [2],
};
const receiverGone = { ...bothConnected, deviceIds: [1], receivingDeviceIds: [] };

/** Longer than any wait the remote once gave its receiver up after (D554 §1), none now (D555 §3). */
const longWaitMs = 30_000;

describe("the cast remote reaches for a receiver the server no longer hears (D550)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
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

  const pill = (): string | undefined =>
    mounted?.container.querySelector(".castee-play__centre .pill")?.textContent ?? undefined;
  const time = (): string | undefined =>
    mounted?.container.querySelector<HTMLInputElement>(".castee-play__page .timeline__range")
      ?.value ?? undefined;
  const isPadDisabled = (): boolean =>
    buttonNamed(mounted!.container, "Pause")?.getAttribute("aria-disabled") === "true";
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

  it("is reaching, its pad disabled and its time standing, once the receiver's device is not among the person's connected devices", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    fakeClock();
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say(bothConnected);
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
    step(3_000);
    const before = time();
    step(3_000);
    expect(time()).not.toBe(before); // heard, the time moves on
    await sockets.live.say(receiverGone);
    expect(pill()).toBe("Reaching Fire TV…");
    expect(isPadDisabled()).toBe(true);
    step(1_500); // the last second's tick
    const standing = time();
    step(10_000);
    expect(time()).toBe(standing);
  });

  it("is reaching, its pad disabled and its time standing, once the leader has left a roster that named one", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    expect(pill()).toBeUndefined();
    fakeClock();
    await sockets.session.say({
      type: "roster",
      roster: [{ id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }] }],
    });
    expect(pill()).toBe("Reaching Fire TV…");
    expect(isPadDisabled()).toBe(true);
    step(1_500);
    const standing = time();
    step(10_000);
    expect(time()).toBe(standing);
  });

  it("is not reaching for a receiver still loading, never heard leading, whose device is connected", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say(bothConnected);
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
  });

  // A Chromecast is a guest receiver, in no devices message (D465 §1): a phone's cast to one was
  // Reaching from its start, the film playing on the television (the owner, 2026-11-25).
  it("hears a guest receiver, among no connected devices, by its session alone", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.live.say({ type: "sending", sending: { ...sending, isGuestReceiver: true } });
    await sockets.live.say(receiverGone);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
    await sockets.session.say({
      type: "roster",
      roster: [{ id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }] }],
    });
    expect(pill()).toBe("Reaching Fire TV…");
  });

  // The owner, 2026-11-25: the phone in airplane mode 26 s lost its Cast session with no Stop, while
  // the Chromecast played on, leading the cast's session (D554 §4).
  it("stays ready, saying nothing and giving nothing up, as a guest receiver's Cast session is lost while the cast's session shows it leading, and forgets the loss (D554 §4)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting({}, { welcome: playing }, heard);
    await sockets.live.say({ type: "sending", sending: { ...sending, isGuestReceiver: true } });
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    fakeClock();
    try {
      act(() => {
        tellCastSessionLost(9);
      });
      expect(pill()).toBeUndefined();
      expect(isPadDisabled()).toBe(false);
      step(longWaitMs);
      expect(mounted!.container.textContent).not.toContain("unreachable");
      expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
      expect(isCastSessionLost(9)).toBe(false); // the leader heard after it: a later loss starts clean
    } finally {
      act(() => {
        tellCastSessionLinked(9);
      });
    }
  });

  it("clears by itself as the receiver comes back", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say(receiverGone);
    expect(pill()).toBe("Reaching Fire TV…");
    await sockets.live.say(bothConnected);
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
  });

  // The castee silent, the server ends its cast (D555 §3); the remote, told, says so.
  it("ends nothing itself as the receiver goes unreached, then, the server ending the cast with its receiver gone, swaps back to the film here, paused, saying nothing of the end, no Try again (D555 §3, §5)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 75_000, isPlaying: false }] },
      { welcome },
      heard,
    );
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    fakeClock();
    await sockets.live.say(bothConnected);
    await sockets.live.say(receiverGone);
    const ends = (): unknown[] =>
      heard
        .filter((each) => each.methodAndPath === "POST /me/casts/9/end")
        .map((each) => bodyField(each, "reason"));
    step(longWaitMs);
    expect(ends()).toEqual([]);
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    vi.useRealTimers();
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(mounted!.container.textContent).not.toContain("Fire TV unreachable");
    expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
    expect(buttonWithText(mounted!.container, "Play here")).toBeUndefined();
    expect(onPlayHere).toHaveBeenCalledOnce();
    expect(onPlayHere.mock.calls[0]?.[1]).toBe(true); // alone, paused (D555 §5)
    expect(ends()).toEqual([]); // the cast over, the swap back asks no end
  });

  // The owner, 2026-11-25: a phone's Wi-Fi gone a moment, the cast should not end; the television
  // plays on, and the remote comes back with the connection (D553 §4).
  it("says it can't reach the server while its own connection reconnects, ending nothing and saying no note, the receiver's wait counting none of it, and takes the cast up again as it returns (D553 §4, D554 §2)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting({}, { welcome: playing }, heard);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say(bothConnected);
    fakeClock();
    const opened = sockets.session.openedCount();
    await sockets.session.drop();
    expect(pill()).toBe("Can't reach the server");
    step(longWaitMs);
    expect(mounted!.container.textContent).not.toContain("Casting ended");
    expect(pill()).toBe("Can't reach the server");
    // The receiver unheard too while the server is unreached: still no time against it (D554 §2).
    await sockets.live.say(receiverGone);
    step(longWaitMs);
    expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
    await sockets.live.say(bothConnected);
    await settleUntil(() => sockets.session.openedCount() > opened);
    await sockets.session.accept();
    await sockets.session.say(playing);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
    expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
    expect(onPlayHere).not.toHaveBeenCalled();
  });
});

describe("the cast remote says the receiver waits for a press on its own remote (D564 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  const pill = (): string | undefined =>
    mounted?.container.querySelector(".castee-play__centre .pill")?.textContent ?? undefined;

  /** The cast's roster, this remote (5) and the receiver's page (6), saying castState. */
  const rosterSaying = (castState: string): Readonly<Record<string, unknown>> => ({
    type: "roster",
    roster: [
      {
        id: 2,
        name: "owner",
        isHost: true,
        devices: [
          { id: 5, role: "panel" },
          { id: 6, role: "video", castState },
        ],
      },
    ],
  });

  it("says it waits for someone to press OK on the television while the receiver holds the cast for its press, and no more once it runs", async () => {
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
    mounted = drawn.mounted;
    const { sockets } = drawn;
    await sockets.live.say(bothConnected);
    await sockets.session.say(rosterSaying("waitingForOK"));
    expect(pill()).toBe("Waiting for someone to press OK on Fire TV");
    expect(mounted.container.querySelector(".cast-remote__spinner")).toBeNull();
    await sockets.session.say(rosterSaying("running"));
    expect(pill()).toBeUndefined();
  });

  it("says nothing of another member's receiver waiting for its press", async () => {
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
    mounted = drawn.mounted;
    const { sockets } = drawn;
    await sockets.session.say({
      type: "roster",
      roster: [
        { id: 2, name: "owner", isHost: true, devices: [{ id: 5, role: "panel" }] },
        {
          id: 3,
          name: "guest",
          isHost: false,
          devices: [{ id: 7, role: "video", castState: "waitingForOK" }],
        },
      ],
    });
    expect(pill()).toBeUndefined();
  });
});
