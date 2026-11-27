// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { startGateOf } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { mount, servedBy } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";

beforeEach(() => {
  window.sessionStorage.clear();
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
});

/** The start gates a test left a play waiting at, each of which looks again on a timer until given up. */
const gatesLeftWaiting: ReturnType<typeof startGateOf>[] = [];

// A gate left waiting looked again after its file's document was gone, and its telling of its wait's
// end threw there, uncaught, failing whichever run it fell into (revert-red, 2026-11-27).
afterEach(() => {
  for (const gate of gatesLeftWaiting.splice(0)) {
    gate.cancel();
  }
});

/**
 * The host's player on film 5, its video paused or playing, or paused only until its start gate
 * plays it (isGettingReady: film-player.tsx's useStartsWithEnoughAhead), welcomed to a session with
 * nothing on.
 */
async function startedWith(
  isPaused: boolean,
  isGettingReady = false,
): Promise<ReturnType<typeof fakeTogetherServer>> {
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { value: isPaused, configurable: true });
  Object.defineProperty(video, "error", { value: null, configurable: true });
  if (isGettingReady) {
    const gate = startGateOf(video);
    gate.play(() => undefined);
    gatesLeftWaiting.push(gate);
  }
  const server = fakeTogetherServer();
  const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
  await mount(
    withServices(
      <HostPlaysTogether
        fileId={5}
        video={video}
        startPositionMs={61_000}
        cannotKeepUp={undefined}
      />,
      services,
    ),
  );
  await server.accept();
  await server.say({
    type: "welcome",
    seq: 1,
    memberId: 1,
    isHost: true,
    group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
    roster: [{ id: 1, name: "Karl", isHost: true, devices: [{ id: 9, role: "video" }] }],
  });
  return server;
}

describe("a Watch Together session started over a paused film (the owner, 2026-11-23)", () => {
  it("puts the film on and pauses it, as the host's video stands paused", async () => {
    const server = await startedWith(true);
    const commands = server.heard.filter((message) => message["type"] === "command");
    expect(commands.map((message) => message["command"])).toEqual(["changeItem", "pause"]);
  });

  // Every session's first film waited paused, and no guest ever played (the browser tier, 2026-11-25):
  // a film just opened stands paused until its start gate plays it, which is not a person's pause.
  it("puts a film still getting ready to play on playing, its pause its start gate's, not a person's", async () => {
    const server = await startedWith(true, true);
    const commands = server.heard.filter((message) => message["type"] === "command");
    expect(commands.map((message) => message["command"])).toEqual(["changeItem"]);
  });

  it("puts a playing film on and leaves it playing", async () => {
    const server = await startedWith(false);
    const commands = server.heard.filter((message) => message["type"] === "command");
    expect(commands.map((message) => message["command"])).toEqual(["changeItem"]);
  });
});

describe("a film a guest puts on for the group (the owner, 2026-11-26; D471 §2)", () => {
  const playing = { state: "playing", positionMs: 0, rate: 1 };
  const idle = { state: "idle", positionMs: 0, rate: 1, fileId: 0 };

  /** The host's page of film 5, welcomed to a session with nothing on, and the route its tab shows. */
  async function leadingFilmFive(): Promise<{
    readonly server: ReturnType<typeof fakeTogetherServer>;
    readonly shown: () => unknown;
  }> {
    const video = document.createElement("video");
    Object.defineProperty(video, "error", { value: null, configurable: true });
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    await mount(
      withServices(
        <HostPlaysTogether fileId={5} video={video} startPositionMs={0} cannotKeepUp={undefined} />,
        services,
      ),
    );
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: idle,
      roster: [{ id: 1, name: "Karl", isHost: true, devices: [{ id: 9, role: "video" }] }],
    });
    return { server, shown: () => services.navigation.route() };
  }

  it("takes the host's page to the film the group plays after its own", async () => {
    const { server, shown } = await leadingFilmFive();
    await server.say({ type: "group", seq: 2, group: { ...playing, fileId: 5 } });
    expect(shown()).not.toMatchObject({ screen: "play" });
    await server.say({ type: "group", seq: 3, group: { ...playing, fileId: 6, itemId: 60 } });
    expect(shown()).toMatchObject({ screen: "play", fileId: 6, itemId: 60 });
  });

  it("leaves the host's page where it is while the group still plays the film before its own", async () => {
    const { server, shown } = await leadingFilmFive();
    // The page was opened on film 5 while the group played film 4: it puts its own on.
    await server.say({ type: "group", seq: 2, group: { ...playing, fileId: 4 } });
    expect(shown()).not.toMatchObject({ screen: "play" });
  });

  it("leaves the host's page where it is as the group stops", async () => {
    const { server, shown } = await leadingFilmFive();
    await server.say({ type: "group", seq: 2, group: { ...playing, fileId: 5 } });
    await server.say({ type: "group", seq: 3, group: idle });
    expect(shown()).not.toMatchObject({ screen: "play" });
  });
});
