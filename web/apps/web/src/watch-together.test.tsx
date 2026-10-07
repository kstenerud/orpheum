// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { stallAfterMs } from "@orpheum/player";
import { beforeEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  type Heard,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";
import { WatchTogether } from "./watch-together.tsx";

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

describe("the host's Watch Together screen (D246 §3, §5)", () => {
  it("starts a session, makes a link with a note, and shows it to pass on", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /together": [
          200,
          { sessionId: 3, membershipCredential: "host-credential", isNew: true },
        ],
        "GET /together/3": [
          [200, { sessionId: 3, links: [], members: [karl] }],
          [200, { sessionId: 3, links: [{ id: 4, hostNote: "Grandma" }], members: [karl] }],
        ],
        "POST /together/3/links": [201, { linkId: 4, secret: "s3cret" }],
      },
      heard,
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(buttonWithText(mounted.container, "Start a session"));
    expect(window.sessionStorage.getItem("orpheum.together.host")).toContain("host-credential");
    expect(mounted.container.textContent).toContain("No one is invited yet.");
    await enter(mounted.container.querySelector("input"), "Grandma");
    await submit(mounted.container.querySelector("form"));
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "POST /together",
      "GET /together/3",
      "POST /together/3/links",
      "GET /together/3",
    ]);
    expect(heard[2]?.body).toEqual({ hostNote: "Grandma" });
    expect(mounted.container.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/together#s3cret",
    );
    expect(
      mounted.container.querySelector('img[alt="The link for Grandma, as a QR code"]'),
    ).not.toBeNull();
    expect(mounted.container.textContent).toContain("while the session lasts");
    expect(mounted.container.textContent).toContain("A link for Grandma, not used yet");
    mounted.unmount();
  });

  it("lists who joined with the host's note, and removes them", async () => {
    hostSession();
    const heard: Heard[] = [];
    const gran = { id: 2, name: "Gran", hostNote: "Grandma", isHost: false };
    const services = servedBy(
      {
        "GET /together/3": [
          [
            200,
            {
              sessionId: 3,
              links: [{ id: 4, hostNote: "Grandma", usedByMemberId: 2 }],
              members: [karl, gran],
            },
          ],
          [200, { sessionId: 3, links: [], members: [karl] }],
        ],
        "POST /together/3/eject/2": [204, undefined],
      },
      heard,
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(mounted.container.textContent).toContain("Gran (Grandma)");
    expect(mounted.container.textContent).not.toContain("not used yet");
    await click(buttonWithText(mounted.container, "Remove"));
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /together/3/eject/2");
    expect(mounted.container.textContent).not.toContain("Gran (Grandma)");
    mounted.unmount();
  });

  it("says when a role may not host", async () => {
    const services = servedBy({ "POST /together": refusal(403, "forbidden") }, [], "token");
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(buttonWithText(mounted.container, "Start a session"));
    expect(mounted.container.textContent).toContain("Your role may not host Watch Together.");
    mounted.unmount();
  });

  it("tells a browser that cannot keep in step that it cannot host, offering no start (D250 §7)", async () => {
    const services = { ...servedBy({}, [], "token"), canFollowAGroup: () => false };
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "update to iOS 17.1 or later",
    );
    expect(mounted.container.querySelector("button")).toBeNull();
    mounted.unmount();
  });
});

describe("the host's player in a session (D246 §5)", () => {
  it("puts the film on for everyone from where it starts, and stops it on leaving", async () => {
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
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
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
    mounted.unmount();
    expect(server.heard.at(-1)).toMatchObject({ type: "command", command: "stop" });
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
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
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
      ...servedBy({ "POST /playback/t/report": [200, { lowerQuality: offer }] }, [], "token"),
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
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
    });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    // Playing, it runs out of data, and the report's answer moves it to the lower quality.
    Object.defineProperty(video, "paused", { value: false });
    await act(async () => {
      video.dispatchEvent(new Event("playing"));
      video.dispatchEvent(new Event("waiting"));
      await new Promise((resolve) => setTimeout(resolve, stallAfterMs + 50));
    });
    expect(mounted.container.textContent).toContain("moves to a lower quality");
    expect(server.heard.filter((message) => message["type"] === "command")).toEqual([]);
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("is what the play page does while this tab hosts a session, and nothing without one", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy({}, [], "token"),
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
