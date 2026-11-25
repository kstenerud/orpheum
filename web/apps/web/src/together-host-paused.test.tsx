// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { startGateOf } from "@orpheum/sdk";
import { beforeEach, describe, expect, it } from "vitest";

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
    startGateOf(video).play(() => undefined);
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
