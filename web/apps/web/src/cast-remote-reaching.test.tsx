// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { notAnsweringAfterMs } from "./cast-remote-offer.tsx";
import { CastRemote } from "./cast-remote.tsx";
import {
  castingRemote,
  receiverLeads,
  sending,
  welcome,
  type RemoteShown,
} from "./test-cast-remote.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
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

describe("the cast remote reaches for a receiver the server no longer hears (D550)", () => {
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
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CastRemote, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  const pill = (): string | undefined =>
    mounted?.container.querySelector(".cast-remote__centre .pill")?.textContent ?? undefined;
  const time = (): string | undefined =>
    mounted?.container.querySelector(".cast-remote__time")?.textContent ?? undefined;
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

  it("clears by itself as the receiver comes back", async () => {
    const { sockets } = await casting({}, { welcome: playing });
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say(receiverGone);
    expect(pill()).toBe("Reaching Fire TV…");
    await sockets.live.say(bothConnected);
    expect(pill()).toBeUndefined();
    expect(isPadDisabled()).toBe(false);
  });

  it("offers Play here once it isn't answering, which ends the cast stopped and plays the film here from where it stood", async () => {
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
    step(notAnsweringAfterMs);
    const note = mounted!.container.querySelector(".cast-remote__note");
    expect(note?.textContent).toContain("Fire TV isn't answering");
    expect(buttonWithText(mounted!.container, "Try again")).toBeDefined();
    await click(buttonWithText(mounted!.container, "Play here"));
    await sockets.session.say({ type: "ended", reason: "stopped" });
    vi.useRealTimers();
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    const ends = heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end");
    expect(ends.map((each) => bodyField(each, "reason"))).toEqual(["stopped"]);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(75_000, true);
  });

  // The owner, 2026-11-25: a phone's Wi-Fi gone a moment, the cast should not end; the television
  // plays on, and the remote comes back with the connection (D553 §4).
  it("says it can't reach the server while its own connection reconnects, ending nothing and saying no note, and takes the cast up again as it returns (D553 §4)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting({}, { welcome: playing }, heard);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say(bothConnected);
    fakeClock();
    const opened = sockets.session.openedCount();
    await sockets.session.drop();
    expect(pill()).toBe("Can't reach the server");
    step(notAnsweringAfterMs + 1_000);
    expect(mounted!.container.querySelector(".cast-remote__note")).toBeNull();
    expect(pill()).toBe("Can't reach the server");
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
