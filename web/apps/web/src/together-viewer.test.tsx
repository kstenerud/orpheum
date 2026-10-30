// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { stallAfterMs } from "@orpheum/player";
import { playedMutedEvent } from "@orpheum/sdk";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createNavigation } from "./navigation.ts";
import {
  buttonWithText,
  click,
  enter,
  fire,
  mount,
  servedBy,
  submit,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import type { Services } from "./services.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { hideTheDocument, playingVideo } from "./test-hidden.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const joined = {
  sessionId: 1,
  memberId: 2,
  membershipCredential: "viewer-credential",
  locator: "viewer-locator",
};

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

/** The viewer's page at address, served by answers and a fake Watch Together server. */
function viewerServices(
  address: string,
  answers: Answers,
  token?: string,
): {
  services: ReturnType<typeof servedBy>;
  heard: Heard[];
  server: FakeTogetherServer;
  place: ReturnType<typeof fakePlace>;
} {
  const heard: Heard[] = [];
  const server = fakeTogetherServer();
  const place = fakePlace(address);
  return {
    services: {
      ...servedBy(answers, heard, token),
      navigation: createNavigation(place),
      openTogetherSocket: server.openSocket,
    },
    heard,
    server,
    place,
  };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a Watch Together viewer's page (D246 §3, §6)", () => {
  it("joins by the link with a name, then shows black until a film is on, asking nothing of its speakers (D336 §4)", async () => {
    const { services, heard, server } = viewerServices("/together#link-secret", {
      "GET /health": [200, { status: "ready", reasons: [] }],
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // The product's wordmark heads the join card, and then the session's page.
    const wordmark = (): Element | null =>
      mounted.container.querySelector('img[alt="Orpheum"][src$="orpheum-logo.svg"]');
    expect(wordmark()).not.toBeNull();
    await enter(mounted.container.querySelector("input"), "Gran");
    // A click, as a person's tap: it is the one that lets the film and the voices play with sound.
    await click(buttonWithText(mounted.container, "Join"));
    expect(wordmark()).not.toBeNull();
    // The Join asks the server first, so an out-of-date page reloads before its person types.
    expect(heard).toEqual([
      { methodAndPath: "GET /health", body: undefined },
      { methodAndPath: "POST /together/join", body: { secret: "link-secret", name: "Gran" } },
    ]);
    // The secret is spent: it is in neither the address nor the history.
    expect(services.navigation.fragment()).toBe("");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
    // With its locator, so a page on a discovery's origin finds the server again without the link.
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-locator");
    expect(mounted.container.textContent).not.toContain("How does this device play sound?");
    await server.accept();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "viewer-credential",
      role: "video",
    });
    await server.say(welcome);
    const stage = mounted.container.querySelector('section[aria-label="The film"]');
    expect(stage).not.toBeNull();
    expect(stage?.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Here: Karl and Gran");
    expect(mounted.container.textContent).not.toContain("Sign out");
    mounted.unmount();
  });

  it("tells the session its page waits on nothing once it has had its tap: there are no speakers to choose (D336 §4)", async () => {
    const { services, server } = viewerServices("/together#link-secret", {
      "GET /health": [200, { status: "ready", reasons: [] }],
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await enter(mounted.container.querySelector("input"), "Gran");
    await click(buttonWithText(mounted.container, "Join"));
    await server.accept();
    await server.say(welcome);
    const standings = (): unknown[] =>
      server.heard.filter((each) => each["type"] === "standing").map((each) => each["standing"]);
    expect(standings()).toEqual([]);
    mounted.unmount();
  });

  it("shows a viewer no choice of how this browser makes sound under the film (D336 §4)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5 },
    });
    expect(mounted.container.querySelector("video")).not.toBeNull();
    expect(mounted.container.textContent).not.toContain("Sound:");
    expect(buttonWithText(mounted.container, "Change")).toBeUndefined();
    mounted.unmount();
  });

  it("pauses at once as its group pauses though its page is hidden, as a computer's hidden page plays on (the owner, 2026-10-28)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    const playing = { state: "Playing", positionMs: 70_000, rate: 1, fileId: 5 };
    await server.say({ type: "group", seq: 2, group: playing });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no film shown");
    }
    const isPaused = playingVideo(video, 70);
    const show = hideTheDocument();
    try {
      const pausedGroup = { ...playing, state: "Paused", event: "paused", byMemberId: 1 };
      await server.say({ type: "group", seq: 3, group: pausedGroup });
      expect(isPaused()).toBe(true);
    } finally {
      show();
      mounted.unmount();
    }
  });

  it("plays the film the host puts on, asked for by the membership, and names who did it", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    // Nothing is asked of how it makes sound (D336 §4).
    expect(mounted.container.textContent).not.toContain("How does this device play sound?");
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: {
        state: "Paused",
        positionMs: 2_472_000,
        rate: 1,
        fileId: 5,
        event: "paused",
        byMemberId: 1,
      },
    });
    expect(heard).toEqual([
      {
        methodAndPath: "POST /together/playback",
        body: { playableFormats: services.playableFormats() },
      },
    ]);
    expect(mounted.container.querySelector("video")?.getAttribute("src")).toBe(
      "/api/v1/bytes/media/direct/abc",
    );
    expect(mounted.container.textContent).toContain("Karl paused at 41:12");
    // The film's name is above the picture, outside the stage, where it hides none of the film.
    const title = mounted.container.querySelector(".together-viewer__title");
    expect(title?.textContent).toBe("Metropolis");
    expect(title?.closest(".film-player__stage")).toBeNull();
    expect(title?.nextElementSibling?.querySelector("video")).not.toBeNull();
    mounted.unmount();
  });

  it("gives a viewer the player's controls their link allows, and full screen however it is (D271 §4)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const shown: string[] = [];
    for (const rights of [
      { canPlayPause: false, canSeek: false },
      { canPlayPause: true, canSeek: false },
    ]) {
      const { services, server } = viewerServices("/together", {
        "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
      });
      const mounted = await mount(withServices(<TogetherViewer />, services));
      // A guest back in the session taps through its first page (TapToContinue).
      await click(buttonWithText(mounted.container, "Tap to continue"));
      await server.accept();
      await server.say({ ...welcome, rights });
      await server.say({
        type: "group",
        seq: 2,
        group: {
          state: "Paused",
          positionMs: 0,
          rate: 1,
          fileId: 5,
          event: "paused",
          byMemberId: 1,
        },
      });
      const video = mounted.container.querySelector("video");
      const fullscreen = buttonWithText(mounted.container, "Full screen");
      shown.push(
        `controls: ${String(video?.controls)}, full screen: ${String(fullscreen !== undefined)}`,
      );
      if (video !== null && fullscreen !== undefined) {
        const requestFullscreen = vi.fn(() => Promise.resolve());
        video.requestFullscreen = requestFullscreen;
        await click(fullscreen);
        expect(requestFullscreen).toHaveBeenCalled();
      }
      mounted.unmount();
    }
    // Watching only, the player's own controls would offer what the group undoes: full screen alone.
    expect(shown).toEqual([
      "controls: false, full screen: true",
      "controls: true, full screen: false",
    ]);
  });

  it("offers the sound at a tap when the browser would only play the film muted", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5, event: "paused", byMemberId: 1 },
    });
    const video = mounted.container.querySelector("video");
    expect(buttonWithText(mounted.container, "Tap to turn the sound back on")).toBeUndefined();
    // The follower found the browser refusing the film's sound, and plays it muted.
    if (video !== null) {
      video.muted = true;
    }
    await fire(video, new Event(playedMutedEvent));
    await click(buttonWithText(mounted.container, "Tap to turn the sound back on"));
    expect(video?.muted).toBe(false);
    expect(buttonWithText(mounted.container, "Tap to turn the sound back on")).toBeUndefined();
    mounted.unmount();
  });

  it("turns the film's sound on at a tap anywhere on the page, as the voices' tap is too (the owner, 2026-10-13)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5, event: "paused", byMemberId: 1 },
    });
    const video = mounted.container.querySelector("video");
    if (video !== null) {
      video.muted = true;
    }
    await fire(video, new Event(playedMutedEvent));
    await click(mounted.container.querySelector(".together-viewer__heading"));
    expect(video?.muted).toBe(false);
    expect(buttonWithText(mounted.container, "Tap to turn the sound back on")).toBeUndefined();
    mounted.unmount();
  });

  it("covers the page of a guest back in their session with who invited them until a tap, and plays the film in the video that tap readied (the owner, 2026-10-13)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const viewer = viewerServices("/together", {
      "POST /together/playback": [200, directDecision({ titleName: "Metropolis" })],
    });
    const { server } = viewer;
    const sockets: number[] = [];
    const services: Services = {
      ...viewer.services,
      openTogetherSocket: (...opening) => {
        sockets.push(sockets.length);
        return viewer.services.openTogetherSocket(...opening);
      },
    };
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    await server.say(welcome);
    const overlay = (): Element | null =>
      mounted.container.querySelector(".together-viewer__continue");
    expect(overlay()?.textContent).toContain("Karl invited you to watch together.");
    // It says how it came to be shown, so a guest asked straight after their Join says why.
    expect(server.heard).toContainEqual(
      expect.objectContaining({
        type: "note",
        note: {
          kind: "continueAsked",
          why: "navigate",
          page: expect.stringMatching(/^loadedMsAgo=\d+ joinedHere=false$/u) as unknown,
        },
      }),
    );
    // As a page of its own: no film, no voices, until the tap.
    const film = (): Element | null =>
      mounted.container.querySelector('section[aria-label="The film"]');
    expect(film()).toBeNull();
    expect(sockets).toHaveLength(1);
    const tappedVideos: HTMLMediaElement[] = [];
    const muting = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "muted");
    Object.defineProperty(HTMLMediaElement.prototype, "muted", {
      ...muting,
      set(this: HTMLMediaElement, isMuted: boolean) {
        tappedVideos.push(this);
        muting?.set?.call(this, isMuted);
      },
    });
    try {
      await click(buttonWithText(mounted.container, "Tap to continue"));
    } finally {
      Object.defineProperty(HTMLMediaElement.prototype, "muted", muting ?? {});
    }
    expect(overlay()).toBeNull();
    expect(film()).not.toBeNull();
    expect(sockets).toHaveLength(2);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5, event: "paused", byMemberId: 1 },
    });
    const video = mounted.container.querySelector("video");
    expect(video).not.toBeNull();
    expect(tappedVideos.length).toBeGreaterThan(0);
    expect(tappedVideos.every((tapped) => tapped === video)).toBe(true);
    mounted.unmount();
  });

  it("takes the Join's tap as the one that lets the film play with sound, and asks for no other", async () => {
    const { services, server } = viewerServices("/together#link-secret", {
      "GET /health": [200, { status: "ready", reasons: [] }],
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await enter(mounted.container.querySelector("input"), "Gran");
    await click(buttonWithText(mounted.container, "Join"));
    await server.accept();
    await server.say(welcome);
    expect(mounted.container.querySelector(".together-viewer__continue")).toBeNull();
    mounted.unmount();
  });

  it("takes the Join's submitting as the tap, as an iPhone's keyboard Go submits it, and asks for no other", async () => {
    const { services, server } = viewerServices("/together#link-secret", {
      "GET /health": [200, { status: "ready", reasons: [] }],
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    await server.accept();
    await server.say(welcome);
    expect(mounted.container.querySelector(".together-viewer__continue")).toBeNull();
    mounted.unmount();
  });

  it("asks for a tap over the black stage while the voices wait for one, as after a phone call took the sound", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const viewer = viewerServices("/together", {});
    const devices = viewer.services.voiceDevices();
    const services = {
      ...viewer.services,
      voiceDevices: () => ({ ...devices, isWaitingForATap: () => true }),
    };
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await viewer.server.accept();
    await viewer.server.say(welcome);
    const stage = mounted.container.querySelector(".film-player__stage--empty");
    const ask = buttonWithText(mounted.container, "Tap to turn the sound back on");
    expect(ask === undefined ? null : stage?.contains(ask)).toBe(true);
    // Asked over the stage alone, not again among the voice's controls.
    expect(buttonWithText(mounted.container, "Tap to hear everyone's voices")).toBeUndefined();
    mounted.unmount();
  });

  it("reports a viewer's stall by the membership, and moves to the lower quality offered (D247 §2)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const offer = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_242_000,
      qualityKbps: 700,
      height: 432,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 2_400,
      linkSharePercent: 70,
      steadiness: 0,
    };
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision()],
      "POST /playback/t/report": [200, { lowerQuality: offer }],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Playing", positionMs: 60_000, rate: 1, fileId: 5 },
    });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    // Playing along, it runs out of data.
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
    const report = heard.find((each) => each.methodAndPath === "POST /playback/t/report");
    expect(report?.body).toMatchObject({ stalls: [{ positionMs: 0 }] });
    expect(mounted.container.textContent).toContain(
      "Your connection could not keep up, so this moves to a lower quality: 432p, 0.7 Mb/s.",
    );
    mounted.unmount();
  });

  it("tells a viewer the player's own words when the film fails, asking nothing of the host's file", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision()],
      "GET /files/5/availability": [200, {}],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5 },
    });
    await fire(mounted.container.querySelector("video"), new Event("error"));
    // Nothing of the host's file.
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /together/playback"]);
    mounted.unmount();
  });
});
