// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { stallAfterMs } from "@orpheum/player";
import { playedMutedEvent } from "@orpheum/sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PlayView } from "./play.tsx";
import { buttonWithText, click, mount, servedBy } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import { fakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";

const karl = { id: 1, name: "Karl", isHost: true };

beforeEach(() => {
  window.sessionStorage.clear();
});

/** This tab hosting session 3. */
function hostSession(): void {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
}

describe("the host's player in a session (D246 §5)", () => {
  it("puts the film on for everyone from where it starts, and leaves it playing on leaving (D364 §15)", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    const mounted = await mount(
      withServices(
        <HostPlaysTogether
          fileId={5}
          video={null}
          startPositionMs={61_000}
          cannotKeepUp={undefined}
        />,
        services,
      ),
    );
    await server.accept();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "host-credential",
      sessionToken: "token",
    });
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
      roster: [{ ...karl, devices: [{ id: 9, role: "video" }] }],
    });
    expect(server.heard).toContainEqual({
      v: 2,
      type: "command",
      command: "changeItem",
      fileId: 5,
      positionMs: 61_000,
      basedOnSeq: 1,
    });
    expect(mounted.container.textContent).toContain("Watch Together is on");
    // Drawn as the session's panel, which the sign over the film moves over it (D423).
    expect(mounted.container.querySelector(".together-panel")?.textContent).toContain(
      "Watch Together is on",
    );
    // The film's volume beside each person's voice, on the host's player too (the owner, 2026-10-12).
    expect(mounted.container.querySelector(".film-volume input")).not.toBeNull();
    mounted.unmount();
    // Leaving the player stops nothing for the group: Stop, or another film, does (D246 §5 withdrawn).
    expect(server.heard.filter((message) => message["command"] === "stop")).toEqual([]);
  });

  it("goes home, told, as the session ends while its film plays here, the film played on alone no more (D389 §1)", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    services.navigation.go({ screen: "play", fileId: 5 });
    const mounted = await mount(
      withServices(
        <HostPlaysTogether fileId={5} video={null} startPositionMs={0} cannotKeepUp={undefined} />,
        services,
      ),
    );
    await server.accept();
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    const notice = currentNotice();
    expect(notice?.text).toBe("The Watch Together session was ended on another of your devices.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
    mounted.unmount();
  });

  it("offers the host a tap for sound when the browser plays the film muted, as after a reload", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    const video = document.createElement("video");
    const mounted = await mount(
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
    expect(buttonWithText(mounted.container, "Tap for sound")).toBeUndefined();
    video.muted = true;
    await act(async () => {
      video.dispatchEvent(new Event(playedMutedEvent));
      await Promise.resolve();
    });
    await click(buttonWithText(mounted.container, "Tap for sound"));
    expect(video.muted).toBe(false);
    expect(buttonWithText(mounted.container, "Tap for sound")).toBeUndefined();
    mounted.unmount();
  });

  it("follows the player's new video when it moves to a lower quality, stopping nothing", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    let showVideo: (video: HTMLVideoElement) => void = () => undefined;
    function Moving(): JSX.Element {
      const [video, setVideo] = useState(() => document.createElement("video"));
      showVideo = setVideo;
      return (
        <HostPlaysTogether
          fileId={5}
          video={video}
          startPositionMs={61_000}
          cannotKeepUp={undefined}
        />
      );
    }
    const mounted = await mount(withServices(<Moving />, services));
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
      roster: [{ ...karl, devices: [{ id: 9, role: "video" }] }],
    });
    await act(async () => {
      showVideo(document.createElement("video"));
      await Promise.resolve();
    });
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Playing", positionMs: 61_000, rate: 1, fileId: 5 },
    });
    const commands = server.heard.filter((message) => message["type"] === "command");
    expect(commands.map((command) => command["command"])).toEqual(["changeItem"]);
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("keeps the session through the play page's move to a lower quality, stopping nothing", async () => {
    window.localStorage.setItem("orpheum.audioOutput", "speakers");
    hostSession();
    const server = fakeTogetherServer();
    const offer = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_242_000,
      qualityKbps: 700,
      height: 432,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 2_400,
    };
    const services = {
      ...servedBy(
        {
          "POST /playback/t/report": [200, { lowerQuality: offer }],
          "GET /files/5/waits": [200, {}],
          "GET /files/5/up-next": [200, {}],
          "GET /files/5/availability": [200, {}],
          "PUT /playback/t/wants": [204, null],
          "DELETE /playback/t": [204, null],
        },
        [],
        "token",
      ),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(
      withServices(
        <PlayView fileId={5} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
      roster: [{ ...karl, devices: [{ id: 9, role: "video" }] }],
    });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    // Playing, it runs out of data, and the report's answer moves it to the lower quality.
    Object.defineProperty(video, "paused", { value: false });
    await act(async () => {
      // The stall's wait on fake time, not the real clock's (rule 16; the test audit, 2026-10-08).
      vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
      try {
        video.dispatchEvent(new Event("playing"));
        video.dispatchEvent(new Event("waiting"));
        await vi.advanceTimersByTimeAsync(stallAfterMs + 50);
      } finally {
        vi.useRealTimers();
      }
    });
    expect(mounted.container.textContent).toContain("moves to a lower quality");
    expect(server.heard.filter((message) => message["type"] === "command")).toEqual([]);
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("is what the play page does while this tab hosts a session, and nothing without one", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy(
        {
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "DELETE /playback/t": [204, null],
        },
        [],
        "token",
      ),
      openTogetherSocket: server.openSocket,
    };
    const view = (
      <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />
    );
    const alone = await mount(withServices(view, services));
    expect(server.openedCount()).toBe(0);
    alone.unmount();
    hostSession();
    const hosting = await mount(withServices(view, services));
    await act(async () => {
      await Promise.resolve();
    });
    expect(server.openedCount()).toBe(1);
    hosting.unmount();
  });
});
