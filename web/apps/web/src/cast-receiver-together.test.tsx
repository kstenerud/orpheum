// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastReceiver } from "./cast-receiver.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { playingVideo } from "./test-hidden.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** A cast handed to the receiver: cast 9, of film 4, from where the sender stood, 61 s. */
const cast = {
  castId: 9,
  membershipCredential: "receiver",
  fileId: 4,
  titleName: "Metropolis",
  positionMs: 61_000,
  fromDeviceId: 1,
  fromDeviceName: "Pixel",
};

/** A session's welcome to the receiver, nothing on. */
const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 9,
  memberId: 2,
  deviceId: 6,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [],
};

describe("a cast's receiver and its person's Watch Together session (D364 §11, D372 §8)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
  });

  it("leads the Watch Together session its person starts while it plays, putting its film on there as it stands, paused, so the session is one (D364 §11, D372 §8)", async () => {
    const services = servedBy({}, [], "token");
    // Each socket the receiver opens, its own fake server, told apart by the hello it hears.
    const servers: FakeTogetherServer[] = [];
    const video = document.createElement("video");
    document.body.append(video);
    playingVideo(video, 1_200);
    video.pause();
    let handAgain: (handed: typeof cast & { togetherCredential?: string }) => void = () =>
      undefined;
    function Receiver(): JSX.Element {
      const [handed, setHanded] = useState<typeof cast & { togetherCredential?: string }>(cast);
      handAgain = setHanded;
      return (
        <CastReceiver
          receiving={{ cast: handed, onLeave: vi.fn() }}
          fileId={4}
          video={video}
          startPositionMs={0}
        />
      );
    }
    mounted = await mount(
      withServices(<Receiver />, {
        ...services,
        openTogetherSocket: (listeners) => {
          const server = fakeTogetherServer();
          servers.push(server);
          return server.openSocket(listeners);
        },
      }),
    );
    const castServer = servers[0];
    expect(castServer).toBeDefined();
    await castServer?.accept();
    await castServer?.say({
      ...welcome,
      group: { state: "Paused", positionMs: 1_200_000, rate: 1, fileId: 4 },
    });
    await act(async () => {
      handAgain({ ...cast, positionMs: 1_199_000, togetherCredential: "hosted" });
      await Promise.resolve();
    });
    for (const server of servers.slice(1)) {
      await server.accept();
    }
    const hostedServer = servers.find((server) =>
      server.heard.some((each) => each["credential"] === "hosted"),
    );
    expect(hostedServer?.heard[0]).toMatchObject({ credential: "hosted", role: "video" });
    await hostedServer?.say({ ...welcome, sessionId: 7 });
    const commands = hostedServer?.heard.filter((each) => each["type"] === "command");
    expect(commands).toEqual([
      expect.objectContaining({ command: "changeItem", fileId: 4, positionMs: 1_200_000 }),
      expect.objectContaining({ command: "pause" }),
    ]);
  });
});
