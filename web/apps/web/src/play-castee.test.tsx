// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { browserStorage } from "./browser-storage.ts";
import { CasteePlay } from "./play-castee.tsx";
import {
  cast,
  castingRemote,
  receiverLeads,
  welcome,
  type RemoteShown,
  heldUpright,
} from "./test-castee.tsx";
import { buttonNamed, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { forgetMembership, keepMembership } from "./together-storage.ts";

/** The person's devices, as the live connection tells them: this phone, 1, and the receiver, 2. */
const bothConnected = {
  type: "devices",
  deviceIds: [1, 2],
  inFrontDeviceIds: [],
  receivingDeviceIds: [2],
};

/** The receiver's device gone from the person's connected devices. */
const receiverGone = { ...bothConnected, deviceIds: [1], receivingDeviceIds: [] };

// The rework plan's phase 4 item 3: casting is the same Play page with the other target.
describe("the Play page over the castee (D555 §5, §8)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    forgetMembership(browserStorage(), "viewer");
    vi.unstubAllGlobals();
  });

  async function casting(
    answers: Parameters<typeof castingRemote>[0] = {},
    heard: Heard[] = [],
    shown: RemoteShown = {},
  ): Promise<
    Awaited<ReturnType<typeof castingRemote>> & { readonly onPlayHere: ReturnType<typeof vi.fn> }
  > {
    const onPlayHere = vi.fn();
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CasteePlay, onPlayHere });
    mounted = drawn.mounted;
    await drawn.sockets.session.say({ type: "roster", roster: receiverLeads });
    await drawn.sockets.live.say(bothConnected);
    return { ...drawn, onPlayHere };
  }

  const page = (): HTMLElement => mounted!.container;
  const pill = (): string | undefined =>
    page().querySelector(".castee-play__centre .pill")?.textContent ?? undefined;

  it("draws the remote's pad under the castee, its sign where the film plays, held upright (the owner, 2026-11-26)", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    await casting();
    expect(page().querySelector(".castee-play h2")?.textContent).toBe("On: Fire TV");
    const pad = page().querySelector<HTMLElement>(".castee-play__page > .remote-pad");
    expect(pad).not.toBeNull();
    expect(buttonNamed(pad!, "Play")?.hasAttribute("aria-disabled")).toBe(false);
    expect(page().querySelector(".castee-play__page .timeline__range")).not.toBeNull();
    expect(page().querySelector(".castee-play__stage")).not.toBeNull();
    expect(page().querySelector(".cast-remote")).toBeNull();
    expect(pill()).toBeUndefined();
  });

  it("swaps back to the video here, paused, at the castee's last place as the cast ends with the person alone, saying nothing of the end", async () => {
    const { sockets, onPlayHere } = await casting();
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    expect(page().textContent).not.toContain("Casting ended");
    expect(page().textContent).not.toContain("Cast again");
  });

  it("swaps back to the video here, playing, as the cast ends while the person is in a Watch Together session", async () => {
    keepMembership(browserStorage(), "viewer", {
      sessionId: 7,
      credential: "guest",
      link: "https://orpheum.test/w/7",
      locator: undefined,
    });
    const { sockets, onPlayHere } = await casting();
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, false);
    expect(page().textContent).not.toContain("Casting ended");
  });

  it("takes the cast moved into a Watch Together session for no end, joining it by its new credential", async () => {
    const moved = { ...cast, membershipCredential: "remote in the party" };
    const { sockets, onPlayHere } = await casting({
      "GET /me/casts": [
        [200, { casts: [cast] }],
        [200, { casts: [moved] }],
      ],
    });
    await sockets.session.say({ type: "ended", reason: "castMoved" });
    await settleUntil(() => sockets.session.openedCount() === 2);
    await sockets.session.accept();
    const hellos = sockets.session.heard.filter((each) => each["type"] === "hello");
    expect(hellos.at(-1)).toMatchObject({ credential: "remote in the party" });
    await sockets.session.say(welcome);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(page().querySelector(".castee-play h2")?.textContent).toBe("On: Fire TV");
  });

  it("shows the reconnecting sign over the key frame, the pad taking no press, while the castee is not heard, and leaves the end to the server", async () => {
    const { sockets, onPlayHere } = await casting();
    await sockets.live.say(receiverGone);
    expect(pill()).toBe("Reaching Fire TV…");
    expect(buttonNamed(page(), "Play")?.getAttribute("aria-disabled")).toBe("true");
    expect(onPlayHere).not.toHaveBeenCalled();
    await sockets.live.say(bothConnected);
    expect(pill()).toBeUndefined();
  });
});
