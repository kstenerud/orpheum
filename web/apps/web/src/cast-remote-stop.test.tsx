// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { endCast } from "./cast.ts";
import { CastRemote } from "./cast-remote.tsx";
import {
  castingRemote,
  catReads,
  sending,
  welcome,
  type RemoteShown,
} from "./test-cast-remote.tsx";
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
    const drawn = await castingRemote(answers, [], { ...shown, remote: CastRemote, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  it("plays the film here from where the server's answer to this tab's Stop says it stood, playing as it played, before where the session had it", async () => {
    const { services, sockets, onPlayHere } = await casting({
      "POST /me/casts/9/end": [200, { positionMs: 90_000, isPlaying: true }],
    });
    await endCast(services.api, 9, "stopped");
    await sockets.session.say({ type: "ended", reason: "stopped" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(90_000, false);
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

  it("asks the page's film for no position, not 0, as the receiver is gone with no leader heard and nothing answered", async () => {
    const { sockets, onPlayHere } = await casting({}, { welcome: welcomeWithNoPlace });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(undefined, false);
  });

  it("opens another film's Play page with no position, not 0, where none is known", async () => {
    const { services, sockets } = await casting(
      { "GET /files/5/up-next": [200, {}], ...catReads },
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
