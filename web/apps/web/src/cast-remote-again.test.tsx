// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CastRemote } from "./cast-remote.tsx";
import {
  castingRemote,
  catReads,
  receiverLeads,
  sending,
  type RemoteShown,
} from "./test-cast-remote.tsx";
import { buttonWithText, click, type Heard, type Mounted } from "./test-dom.tsx";

/** The person's devices, as the live connection tells them: this phone, 1, and the receiver, 2. */
const bothConnected = {
  type: "devices",
  deviceIds: [1, 2],
  inFrontDeviceIds: [],
  receivingDeviceIds: [2],
};

/** The new cast Cast again starts, as the server answers it. */
const castAgainAnswer = {
  "POST /me/casts": [201, { castId: 10, isSender: true, membershipCredential: "sender" }],
} as const;

// The owner, 2026-11-25, of a cast its receiver took back or lost: "Play here" and, once the
// television is back, "Cast again", the film cast there anew from where it stood (D553 §2).
describe("the ended cast remote's Cast again (D553 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function ended(
    answers: Parameters<typeof castingRemote>[0],
    heard: Heard[] = [],
    shown: RemoteShown = {},
  ): Promise<Awaited<ReturnType<typeof castingRemote>>> {
    const drawn = await castingRemote(answers, heard, {
      ...shown,
      remote: CastRemote,
      onPlayHere: vi.fn(),
    });
    mounted = drawn.mounted;
    await drawn.sockets.session.say({ type: "roster", roster: receiverLeads });
    return drawn;
  }

  const castAgain = (): HTMLElement | undefined => buttonWithText(mounted!.container, "Cast again");

  it("is offered beside Play here once the receiver's device is back among the person's connected devices, and casts the film there anew, as its title, from where it stood, paused as it was", async () => {
    const heard: Heard[] = [];
    const { sockets } = await ended(castAgainAnswer, heard);
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(buttonWithText(mounted!.container, "Play here")).toBeDefined();
    expect(castAgain()).toBeUndefined(); // the receiver not yet heard among the devices
    await sockets.live.say(bothConnected);
    await click(castAgain());
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts")?.body).toEqual({
      toDeviceId: 2,
      fileId: 4,
      itemId: 2,
      positionMs: 61_000,
      isPaused: true,
    });
  });

  it("is not offered while the receiver's device is not among the person's connected devices", async () => {
    const { sockets } = await ended({});
    await sockets.live.say({ ...bothConnected, deviceIds: [1], receivingDeviceIds: [] });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    expect(buttonWithText(mounted!.container, "Play here")).toBeDefined();
    expect(castAgain()).toBeUndefined();
  });

  // A Chromecast is cast to through Chrome's picker alone, which names no device the page could
  // find again (D553 §2).
  it("is not offered for a guest receiver, a Chromecast", async () => {
    const { sockets } = await ended({}, [], { sent: { ...sending, isGuestReceiver: true } });
    await sockets.live.say(bothConnected);
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    expect(buttonWithText(mounted!.container, "Play here")).toBeDefined();
    expect(castAgain()).toBeUndefined();
  });

  it("goes to the page of the film the cast had moved on to as its receiver takes the new cast", async () => {
    const { services, sockets } = await ended({
      ...castAgainAnswer,
      "GET /files/5/up-next": [200, {}],
      ...catReads,
    });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await sockets.live.say(bothConnected);
    await click(castAgain());
    expect(services.navigation.route().screen).not.toBe("play");
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, castId: 10, fileId: 5, itemName: "Cat" },
    });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
  });
});
