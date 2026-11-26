// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a Watch Together guest by link's top bar (D555 §9)", () => {
  it("puts a guest by link's Leave in its own top bar under the session's name, and in the session's panel in full screen (D555 §9)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const server = fakeTogetherServer();
    const services = {
      ...servedBy(
        { "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })] },
        [],
      ),
      navigation: createNavigation(fakePlace("/together")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "paused", positionMs: 0, rate: 1, fileId: 5, event: "paused", byMemberId: 1 },
    });
    /** Where Leave is: the top bar, the session's panel, or nowhere. */
    const leaveIn = (): string[] =>
      [...mounted.container.querySelectorAll("button")]
        .filter((each) => each.textContent === "Leave")
        .map((each) => {
          if (each.closest("header.together-viewer__bar") !== null) {
            return "bar";
          }
          return each.closest(".together-panel") === null ? "elsewhere" : "panel";
        });
    const bar = mounted.container.querySelector("header.together-viewer__bar");
    expect(bar?.querySelector("h1")?.textContent).toBe("Watching together");
    expect(bar?.querySelector("img")?.getAttribute("alt")).toBe("Orpheum");
    expect(leaveIn()).toEqual(["bar"]);
    const stage = mounted.container.querySelector(".film-player__stage");
    Object.defineProperty(document, "fullscreenElement", { configurable: true, get: () => stage });
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    try {
      // The top bar hides with the page full screen: its Leave is the panel's, which the sign over
      // the film opens.
      expect(leaveIn()).toEqual(["panel"]);
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      act(() => {
        document.dispatchEvent(new Event("fullscreenchange"));
      });
    }
    expect(leaveIn()).toEqual(["bar"]);
    mounted.unmount();
  });
});
