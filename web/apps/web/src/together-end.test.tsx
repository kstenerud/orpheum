// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { PlaysAlong } from "./cast-receiver.tsx";
import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { GroupFilmPage } from "./together-viewer.tsx";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

/** A video that says it has ended once told to end, pausing first as a browser's does. */
function endingVideo(): HTMLVideoElement & { end: () => void } {
  const video = document.createElement("video");
  let hasEnded = false;
  Object.defineProperty(video, "ended", { get: () => hasEnded });
  return Object.assign(video, {
    end: (): void => {
      hasEnded = true;
      act(() => {
        video.dispatchEvent(new Event("pause"));
        video.dispatchEvent(new Event("ended"));
      });
    },
  });
}

describe("a Watch Together film played to its end with nothing next (the owner, 2026-11-17)", () => {
  /** The host's player of file 5 in session 3, welcomed; what its session heard. */
  async function hostAtTheEnd(hasNext: boolean): Promise<readonly unknown[]> {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "host-credential" }),
    );
    const server = fakeTogetherServer();
    const video = endingVideo();
    const mounted = await mount(
      withServices(
        <PlaysAlong
          receiving={undefined}
          fileId={5}
          video={video}
          startPositionMs={0}
          cannotKeepUp={undefined}
          hasNext={hasNext}
        />,
        { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "playing", positionMs: 0, rate: 1, fileId: 5 },
      roster: [{ id: 1, name: "Karl", isHost: true, devices: [{ id: 9, role: "video" }] }],
    });
    video.end();
    mounted.unmount();
    return server.heard;
  }

  it("tells the group the host's video ended, by which the server stops a film with nothing next (D516)", async () => {
    const heard = await hostAtTheEnd(false);
    expect(heard).toContainEqual(expect.objectContaining({ type: "state", isAtTheEnd: true }));
  });

  it("sends no stop of its own at the end, where an episode plays next or none does: the server finds which (D516 §2)", async () => {
    for (const hasNext of [true, false]) {
      const heard = await hostAtTheEnd(hasNext);
      expect(heard).not.toContainEqual(expect.objectContaining({ command: "stop" }));
    }
  });

  it("takes a guest's Play page to the page they had before the host played anything", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy({ "POST /together/playback": [200, directDecision({})] }, [], "token"),
      navigation: createNavigation(fakePlace("/play/5")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(
      withServices(
        <GroupFilmPage membership={{ sessionId: 1, credential: "viewer-credential" }} />,
        services,
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      sessionId: 1,
      memberId: 2,
      deviceId: 8,
      group: { state: "playing", positionMs: 0, rate: 1, fileId: 5 },
      roster: [
        { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
        { id: 2, name: "Gran", devices: [{ id: 8, role: "panel" }] },
      ],
    });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
    });
    expect(services.navigation.route()).toEqual({ screen: "together" });
    mounted.unmount();
  });
});
