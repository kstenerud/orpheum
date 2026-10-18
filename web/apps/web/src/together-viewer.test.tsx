// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { stallAfterMs } from "@orpheum/player";
import { playedMutedEvent } from "@orpheum/sdk";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "./app.tsx";
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
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "playback" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "playback" }] },
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
  it("joins by the link with a name, then asks about speakers, then shows black until a film is on", async () => {
    const { services, heard, server } = viewerServices("/together#link-secret", {
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
    // Asked at once, before anything plays, and kept in this browser.
    expect(mounted.container.textContent).toContain("How does this device play sound?");
    await server.accept();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "viewer-credential",
      role: "playback",
    });
    await click(
      [...mounted.container.querySelectorAll("button")].find((button) =>
        button.textContent.startsWith("Headphones"),
      ),
    );
    expect(window.localStorage.getItem("orpheum.together.audioOutput")).toBe("headphones");
    await server.say(welcome);
    const stage = mounted.container.querySelector('section[aria-label="The film"]');
    expect(stage).not.toBeNull();
    expect(stage?.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Here: Karl and Gran");
    expect(mounted.container.textContent).not.toContain("Sign out");
    mounted.unmount();
  });

  it("tells the session what the page waits on, its person's speakers, until they are chosen (the owner's session, 2026-10-18)", async () => {
    const { services, server } = viewerServices("/together#link-secret", {
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await enter(mounted.container.querySelector("input"), "Gran");
    await click(buttonWithText(mounted.container, "Join"));
    await server.accept();
    await server.say(welcome);
    const standings = (): unknown[] =>
      server.heard.filter((each) => each["type"] === "standing").map((each) => each["standing"]);
    expect(standings()).toEqual(["choosingSound"]);
    await click(
      [...mounted.container.querySelectorAll("button")].find((button) =>
        button.textContent.startsWith("Headphones"),
      ),
    );
    expect(standings()).toEqual(["choosingSound", ""]);
    mounted.unmount();
  });

  it("lets a viewer change how this browser makes sound, and asks for the film again with it", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5 },
    });
    expect(mounted.container.textContent).toContain("Sound: Soundbar or receiver");
    await click(buttonWithText(mounted.container, "Change"));
    await click(
      [...mounted.container.querySelectorAll("button")].find((each) =>
        each.textContent.startsWith("Headphones"),
      ),
    );
    expect(window.localStorage.getItem("orpheum.together.audioOutput")).toBe("headphones");
    expect(mounted.container.textContent).toContain("Sound: Headphones");
    expect(heard.map((each) => each.body)).toEqual([
      { playableFormats: services.playableFormats(), audioOutput: "receiver" },
      { playableFormats: services.playableFormats(), audioOutput: "headphones" },
    ]);
    mounted.unmount();
  });

  it("plays the film the host puts on, asked for by the membership, and names who did it", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    // The speaker question was answered in this browser before: it is not asked again.
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
        body: {
          playableFormats: services.playableFormats(),
          audioOutput: "receiver",
        },
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
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

  it("tells a viewer the player's own words when a prepared film fails, asking nothing of the host's file", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const prepared = directDecision({
      delivery: "hls",
      url: "/api/v1/bytes/media/hls/abc/master.m3u8",
      profile: "remux",
      notDirectReasons: ["container"],
    });
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, prepared],
      "GET /files/5/availability": [200, {}],
      "GET /bytes/media/hls/abc/master.m3u8": [200, {}],
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
    // The prepared copy's playlist, waited for, and nothing of the host's file.
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "POST /together/playback",
      "GET /bytes/media/hls/abc/master.m3u8",
    ]);
    mounted.unmount();
  });

  it("comes back as the same member by the link it joined by, after the page was closed (D267 §1)", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential", link: "link-secret" }),
    );
    const { services, heard, server } = viewerServices("/together#link-secret", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "viewer-credential" });
    expect(heard).toEqual([]); // no join: the spent link is not used again
    expect(services.navigation.fragment()).toBe("");
    mounted.unmount();
  });

  it("has back, signed in, the session its person joined on another device (D267 §2)", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    const { services, server } = viewerServices(
      "/together",
      {
        "GET /together/mine": [
          200,
          {
            sessions: [
              {
                sessionId: 1,
                memberId: 2,
                isHost: false,
                membershipCredential: "phone-credential",
              },
            ],
          },
        ],
      },
      "signed-in-token",
    );
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "phone-credential" });
    mounted.unmount();
  });

  it("leaves, and says so, forgetting the membership", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard } = viewerServices("/together", {
      "POST /together/leave": [204, undefined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await click(buttonWithText(mounted.container, "Leave"));
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /together/leave"]);
    expect(mounted.container.textContent).toContain("You left the session.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("tells a viewer the host ended the session", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(mounted.container.textContent).toContain("The host ended the session.");
    mounted.unmount();
  });

  it("tells a viewer sent away for the host's connection why, keeps them a member, and lets them back in", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "uploadFull" });
    expect(mounted.container.textContent).toContain(
      "The host's connection can't carry everyone right now.",
    );
    expect(window.localStorage.getItem("orpheum.together.viewer")).not.toBeNull();
    await click(buttonWithText(mounted.container, "Join again"));
    expect(mounted.container.textContent).not.toContain("can't carry everyone");
    expect(buttonWithText(mounted.container, "Leave")).toBeDefined();
    mounted.unmount();
  });

  it("joins by a new link pasted into a tab still on the page, which does not load again", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server, heard, place } = viewerServices("/together", {
      "POST /together/join": [200, { ...joined, sessionId: 2, membershipCredential: "second" }],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "hostEnded" });
    // Only the part after # changed: the browser says so, and the page stays.
    await act(async () => {
      place.back("/together#second-secret");
      await Promise.resolve();
    });
    const join = buttonWithText(mounted.container, "Join");
    expect(join?.disabled).toBe(false);
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(heard).toEqual([
      { methodAndPath: "GET /health", body: undefined },
      { methodAndPath: "POST /together/join", body: { secret: "second-secret", name: "Gran" } },
    ]);
    mounted.unmount();
  });

  it("fills in the name of someone signed in here, which they may change before joining", async () => {
    const { services, heard } = viewerServices(
      "/together#link-secret",
      {
        "GET /me": [200, { id: 4, name: "Partner", isAdmin: false, canHostWatchTogether: false }],
        "POST /together/join": [200, joined],
      },
      "partner-token",
    );
    const mounted = await mount(withServices(<TogetherViewer />, services));
    const input = mounted.container.querySelector("input");
    expect(input?.value).toBe("Partner");
    await enter(input, "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(heard.find((h) => h.methodAndPath === "POST /together/join")?.body).toEqual({
      secret: "link-secret",
      name: "Gran",
    });
    mounted.unmount();
  });

  it("fills in no name that arrives once its person is in the field (check-pr, 2026-10-15)", async () => {
    const { services } = viewerServices(
      "/together#link-secret",
      { "GET /me": [200, { id: 4, name: "Partner", isAdmin: false, canHostWatchTogether: false }] },
      "partner-token",
    );
    let answerMe = (): void => undefined;
    const meAnswered = new Promise<void>((resolve) => {
      answerMe = resolve;
    });
    services.api.use({
      async onRequest({ request }) {
        if (new URL(request.url).pathname.endsWith("/me")) {
          await meAnswered;
        }
        return undefined;
      },
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    const input = mounted.container.querySelector("input");
    expect(input?.value).toBe("");
    act(() => {
      input?.focus();
    });
    // Their own name arrives once they are in the field: it goes nowhere, as what they type would
    // have gone after it.
    answerMe();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(input?.value).toBe("");
    mounted.unmount();
  });

  it("asks no one not signed in who they are, and leaves the name empty", async () => {
    const { services, heard } = viewerServices("/together#link-secret", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    expect(mounted.container.querySelector("input")?.value).toBe("");
    expect(heard.map((h) => h.methodAndPath)).not.toContain("GET /me");
    mounted.unmount();
  });

  it("with no link in the address and no session, says so and offers nothing to fill in", async () => {
    const { services } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    expect(mounted.container.textContent).toContain("This address has no invitation in it.");
    expect(mounted.container.querySelector("input")).toBeNull();
    expect(mounted.container.querySelector("button")).toBeNull();
    mounted.unmount();
  });

  it("tells a browser that cannot keep in step to update, before anyone joins (D250 §7)", async () => {
    const { services, heard } = viewerServices("/together#link-secret", {});
    const mounted = await mount(
      withServices(<TogetherViewer />, { ...services, canFollowAGroup: () => false }),
    );
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "update to iOS 17.1 or later",
    );
    expect(mounted.container.querySelector("input")).toBeNull();
    expect(mounted.container.querySelector("button")).toBeNull();
    expect(heard).toEqual([]);
    mounted.unmount();
  });

  it("is a page of its own for someone not signed in: no sign-in, no menu", async () => {
    const { services } = viewerServices("/together#link-secret", {});
    const mounted = await mount(withServices(<App />, services));
    expect(mounted.container.textContent).toContain("Join");
    expect(mounted.container.querySelector("nav")).toBeNull();
    expect(mounted.container.textContent).not.toContain("Sign in");
    mounted.unmount();
  });
});
