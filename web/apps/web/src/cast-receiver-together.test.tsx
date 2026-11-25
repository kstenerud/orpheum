// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { castReportEveryMs } from "@orpheum/sdk";
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
  itemName: "Metropolis",
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
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [],
};

describe("a cast's receiver says where it is each 5 s, its silence ending the cast (D555 §3)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
    vi.useRealTimers();
  });

  it("reports its state to the cast's session each castReportEveryMs", async () => {
    const services = servedBy({}, [], "token");
    const servers: FakeTogetherServer[] = [];
    const video = document.createElement("video");
    document.body.append(video);
    playingVideo(video, 1_200);
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"], shouldAdvanceTime: true });
    mounted = await mount(
      withServices(
        <CastReceiver
          receiving={{ cast, onLeave: vi.fn() }}
          fileId={4}
          video={video}
          startPositionMs={0}
        />,
        {
          ...services,
          openTogetherSocket: (listeners) => {
            const server = fakeTogetherServer();
            servers.push(server);
            return server.openSocket(listeners);
          },
        },
      ),
    );
    const castServer = servers[0];
    await castServer?.accept();
    await castServer?.say({
      ...welcome,
      group: { state: "playing", positionMs: 1_200, rate: 1, fileId: 4 },
    });
    const states = (): number =>
      castServer?.heard.filter((each) => each["type"] === "state").length ?? 0;
    const before = states();
    for (let report = 1; report <= 3; report += 1) {
      act(() => {
        vi.advanceTimersByTime(castReportEveryMs);
      });
      expect(states() - before).toBeGreaterThanOrEqual(report);
      await castServer?.say({ type: "pong", tMono: 0 }); // alive
    }
  });
});

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
      group: { state: "paused", positionMs: 1_200_000, rate: 1, fileId: 4 },
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
