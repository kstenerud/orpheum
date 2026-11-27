// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { endCast } from "./cast.ts";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, catReads, sending, welcome, type RemoteShown } from "./test-castee.tsx";
import { settleUntil, type Mounted } from "./test-dom.tsx";

/** A session that holds no position yet: the receiver led nothing, and no place was said. */
const welcomeWithNoPlace = { ...welcome, group: undefined };

describe("the cast remote's Stop, where the film resumes (D549)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function casting(
    answers: Parameters<typeof castingRemote>[0],
    shown: RemoteShown = {},
  ): Promise<Awaited<ReturnType<typeof castingRemote>> & { onPlayHere: ReturnType<typeof vi.fn> }> {
    const onPlayHere = vi.fn();
    const drawn = await castingRemote(answers, [], { ...shown, remote: CasteePlay, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  it("plays the film here from where the server's answer to this tab's Stop says it stood, playing as it played there, the person alone, before where the session had it (D555 §5, D564 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({
      "POST /me/casts/9/end": [200, { positionMs: 90_000, isPlaying: true }],
    });
    await endCast(services.api, 9, "stopped");
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(90_000, false);
  });

  it("brings the film back here paused where the server's answer to this tab's Stop says the castee had it paused, the person alone (D564 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({
      "POST /me/casts/9/end": [200, { positionMs: 90_000, isPlaying: false }],
    });
    const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
    await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
    await endCast(services.api, 9, "stopped");
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(90_000, true);
  });

  it("brings the film back here paused where the cast was stopped elsewhere as it played, the person alone (D555 §5, D564 §1)", async () => {
    const { sockets, onPlayHere } = await casting({});
    const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
    await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
    await sockets.session.say({ type: "ended", reason: "stopped" }); // the receiver's own Stop casting here
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledOnce();
    expect(onPlayHere.mock.calls[0]?.[1]).toBe(true);
  });

  it("plays the film here from the server's answer even where no leader was heard and the session holds no place", async () => {
    const { services, sockets, onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 42_000, isPlaying: false }] },
      { welcome: welcomeWithNoPlace },
    );
    await endCast(services.api, 9, "stopped");
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(42_000, true);
  });

  it("asks the page's film for no position, not 0, as the cast ends with the receiver gone, no leader heard and nothing answered (D552 §1, D555 §5)", async () => {
    const { sockets, onPlayHere } = await casting({}, { welcome: welcomeWithNoPlace });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(undefined, true);
  });

  it("opens another film's Play page with no position, not 0, where none is known, as the cast ends (D555 §5)", async () => {
    const { services, sockets } = await casting(
      { "GET /files/5/up-next": [200, {}], "GET /files/5/trickplay": [404, {}], ...catReads },
      { welcome: welcomeWithNoPlace },
    );
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await settleUntil(() => services.navigation.route().screen === "play");
    expect(services.navigation.route()).toEqual({
      screen: "play",
      fileId: 5,
      shouldPlayHere: true,
      isPaused: true,
    });
  });

  it("goes home, asking nothing of the page, where the cast had nothing on, whatever the server answered", async () => {
    const { services, sockets, onPlayHere } = await casting({
      "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }],
    });
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0 } });
    await endCast(services.api, 9, "stopped");
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(onPlayHere).not.toHaveBeenCalled();
  });
});
