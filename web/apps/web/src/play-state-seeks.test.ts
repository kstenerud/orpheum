// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { standsStillReportAfterMs } from "@orpheum/player";
import { playerLoadedChunkEvent, playerMovedItselfEvent } from "@orpheum/sdk";
import { act, createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayView } from "./play.tsx";
import { seekAskedEvent } from "./seek-ahead.ts";
import { directDecision } from "./test-fixtures.ts";
import { stepsOf } from "./steps-of.ts";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { createPathMoves } from "./services.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("play state", () => {
  it("tells how long each of the person's seeks took to play again, from its last seeking (the owner, 2026-10-12)", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "sought" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/sought/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/sought/interest": [204, null],
            "DELETE /playback/sought": [204, null],
          },
          heard,
        ),
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 4 });
    Object.defineProperty(video, "paused", { value: false, configurable: true });
    let isSeeking = false;
    Object.defineProperty(video, "seeking", { get: () => isSeeking, configurable: true });
    await act(async () => {
      video.currentTime = 30;
      video.dispatchEvent(new Event("timeupdate"));
      // Firefox moves the position, and says so, before its seeking event.
      isSeeking = true;
      video.currentTime = 600;
      video.dispatchEvent(new Event("timeupdate"));
      video.dispatchEvent(new Event("seeking"));
      isSeeking = false;
      await new Promise((resolve) => setTimeout(resolve, 5));
      const chunk = { loadMs: 900, loadedKbps: 30_000, qualityKbps: 36_208, filmMs: 10_200 };
      video.dispatchEvent(
        new CustomEvent(playerLoadedChunkEvent, { detail: { track: "video", ...chunk } }),
      );
      await new Promise((resolve) => setTimeout(resolve, 15));
      video.dispatchEvent(
        new CustomEvent(playerLoadedChunkEvent, { detail: { track: "audio", ...chunk } }),
      );
      video.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const seeks = heard
      .filter((request) => request.methodAndPath === "POST /playback/sought/report")
      .flatMap((request) => (request.body as { seeks?: Record<string, number>[] }).seeks ?? []);
    expect(seeks).toHaveLength(1);
    expect(seeks[0]).toMatchObject({ fromPositionMs: 30_000, toPositionMs: 600_000 });
    expect(seeks[0]?.["readyAfterMs"]).toBeGreaterThanOrEqual(15);
    // Which it waited on: the picture's first chunk came, then the sound's (the owner, 2026-10-12).
    expect(seeks[0]?.["pictureAfterMs"]).toBeGreaterThanOrEqual(3);
    expect(seeks[0]?.["soundAfterMs"]).toBeGreaterThanOrEqual(
      (seeks[0]?.["pictureAfterMs"] ?? 0) + 10,
    );
  });

  it("sends the player's last steps with a seek, its quality's choices among them, and nothing when it kept none (2026-10-20)", () => {
    const video = document.createElement("video");
    const steps = "ceiling 4 to 0: link 900 kb/s, 0.0 s ahead; level 0";
    expect(stepsOf(video, () => ({ steps }))).toEqual({ steps });
    expect(stepsOf(video, () => ({ steps: "" }))).toEqual({});
    expect(stepsOf(video, () => undefined)).toEqual({});
  });

  it("sends the link's last samples with a seek apart from its steps, and nothing when it kept none (D352 §5)", () => {
    const video = document.createElement("video");
    const linkSamples = "samples 4 812345B 1203ms fb340ms";
    expect(stepsOf(video, () => ({ steps: "level 0", linkSamples }))).toEqual({
      steps: "level 0",
      linkSamples,
    });
    expect(stepsOf(video, () => ({ linkSamples }))).toEqual({ linkSamples });
    expect(stepsOf(video, () => ({ steps: "level 0", linkSamples: "" }))).toEqual({
      steps: "level 0",
    });
  });

  it("tells where each pause and hls.js's own skip came from, and each path the page moved to (the owner, 2026-10-17)", async () => {
    const heard: Heard[] = [];
    const pathMoves = createPathMoves();
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "hiccup" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(playView, {
        ...servedBy(
          {
            "POST /playback/hiccup/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/hiccup/interest": [204, null],
            "DELETE /playback/hiccup": [204, null],
          },
          heard,
        ),
        pathMoves,
      }),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 4 });
    await act(async () => {
      // Each report is let go before the next event: the reporter sends one at a time.
      const sent = async (): Promise<void> => {
        await new Promise((resolve) => setTimeout(resolve, 5));
      };
      video.dispatchEvent(new Event("playing"));
      await sent();
      video.currentTime = 30;
      video.dispatchEvent(new Event("timeupdate"));
      pathMoves.moved("WebRTC");
      video.dispatchEvent(new Event("pause"));
      await sent();
      // hls.js steps over a hole: it says so, then the browser seeks.
      video.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "hole" }));
      video.currentTime = 30.25;
      video.dispatchEvent(new Event("seeking"));
      await sent();
      // A step back the media session asked, then a seek nothing on the page asked.
      video.dispatchEvent(new CustomEvent(seekAskedEvent, { detail: "seekBack-media" }));
      video.currentTime = 20.25;
      video.dispatchEvent(new Event("seeking"));
      await sent();
      video.currentTime = 50;
      video.dispatchEvent(new Event("seeking"));
      await sent();
    });
    const bodies = heard
      .filter((request) => request.methodAndPath === "POST /playback/hiccup/report")
      .map(
        (request) =>
          request.body as {
            events: { kind: string; context?: Record<string, unknown> }[];
            pathMoves?: { to: string }[];
          },
      );
    const events = bodies.flatMap((body) => body.events);
    expect(events.find((event) => event.kind === "paused")?.context).toMatchObject({
      readyState: 4,
    });
    const seeked = events.filter((event) => event.kind === "seeked");
    expect(seeked[0]?.context).toMatchObject({ skippedBy: "hole" });
    expect(seeked[0]?.context?.["soughtBy"]).toBeUndefined();
    // What asked a seek goes with it, and with no other (staging, 2026-10-29, file 5680).
    expect(seeked[1]?.context).toMatchObject({ soughtBy: "seekBack-media" });
    expect(seeked[2]?.context?.["soughtBy"]).toBeUndefined();
    expect(bodies.flatMap((body) => body.pathMoves ?? []).map((move) => move.to)).toEqual([
      "WebRTC",
    ]);
  });

  it("tells what each start played on from, its play or a wait for data, and how long before (a Fire TV, 2026-10-28)", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "twice" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/twice/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/twice/interest": [204, null],
            "DELETE /playback/twice": [204, null],
          },
          heard,
        ),
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 4 });
    await act(async () => {
      const sent = async (): Promise<void> => {
        await new Promise((resolve) => setTimeout(resolve, 5));
      };
      video.dispatchEvent(new Event("play"));
      video.dispatchEvent(new Event("playing"));
      await sent();
      // The Fire TV's second start, 0.6 s after its first, with nothing between but a wait.
      video.dispatchEvent(new Event("waiting"));
      video.dispatchEvent(new Event("playing"));
      await sent();
    });
    const started = heard
      .filter((request) => request.methodAndPath === "POST /playback/twice/report")
      .flatMap(
        (request) =>
          (request.body as { events: { kind: string; context?: Record<string, unknown> }[] })
            .events,
      )
      .filter((event) => event.kind === "started");
    expect(started.map((event) => event.context?.["startedAfter"])).toEqual(["play", "waiting"]);
    expect(started[1]?.context).toMatchObject({ readyState: 4 });
    expect(typeof started[1]?.context?.["stoodMs"]).toBe("number");
  });

  it("tells where a video asked to play keeps its position, and what it showed (the owner's friend, 2026-10-12)", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    try {
      const heard: Heard[] = [];
      const playView = createElement(PlayView, {
        fileId: 2,
        decision: directDecision({ mediaToken: "still" }),
        onDecideAgain: () => undefined,
      });
      mounted = await mount(
        withServices(
          playView,
          servedBy(
            {
              "POST /playback/still/report": [204, undefined],
              "GET /files/2/waits": [200, {}],
              "GET /files/2/up-next": [200, {}],
              "DELETE /playback/still": [204, null],
            },
            heard,
          ),
        ),
      );
      const video = mounted.container.querySelector("video");
      if (video === null) {
        throw new Error("no video");
      }
      Object.defineProperty(video, "readyState", { value: 4 });
      Object.defineProperty(video, "paused", { value: false, configurable: true });
      await act(async () => {
        vi.advanceTimersByTime(standsStillReportAfterMs + 2000);
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      const standstills = heard
        .filter((request) => request.methodAndPath === "POST /playback/still/report")
        .flatMap((request) => (request.body as { standstills?: unknown[] }).standstills ?? []);
      expect(standstills).toEqual([
        expect.objectContaining({
          positionMs: 0,
          readyState: 4,
          isMuted: false,
          isPageHidden: false,
        }),
      ]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("starts on the ladder its decision names, and says the quality follows the connection (D260)", async () => {
    const onLadder = {
      url: "/api/v1/bytes/media/chunks/remembered/master.m3u8",
      startAtBps: 1_896_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: false,
      filmPictureKbps: 4000,
      linkSharePercent: 70,
      steadiness: 0,
    };
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "remembered", onLadder }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/availability": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "POST /playback/remembered/report": [200, {}],
          "DELETE /playback/remembered": [204, null],
        }),
      ),
    );
    expect(mounted.container.textContent).toContain(
      "Starting at 480p, 1.2 Mb/s. The quality follows your connection.",
    );
    expect(mounted.container.textContent).not.toContain("Playing the original file");
  });
});
