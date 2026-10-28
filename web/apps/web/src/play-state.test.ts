// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import {
  playerFoundUnplayableEvent,
  ReportRefused,
  standsStillReportAfterMs,
  steadinessNoter,
} from "@orpheum/player";
import { playerLoadedChunkEvent, playerMovedItselfEvent, startGateOf } from "@orpheum/sdk";
import { act, createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Play, PlayView } from "./play.tsx";
import { directDecision } from "./test-fixtures.ts";
import { stepsOf } from "./steps-of.ts";
import {
  decisionTimesKept,
  decisionTimesOf,
  noteDecisionTimes,
  sendReport,
  tellUnplayable,
  whenOwnPictureGivenUp,
} from "./play-state.ts";
import { ApiProblem } from "./problem.ts";
import { reopenWaitsMs } from "./reopen.ts";
import {
  mount,
  refusal,
  servedBy,
  unreachable,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { createPathMoves } from "./services.tsx";
import { withServices } from "./test-services.tsx";

/** One report, as the player sends it. */
const report = {
  reportSeq: 1,
  sentMonoMs: 5000,
  positionMs: 1200,
  positionReadMonoMs: 5000,
  events: [{ kind: "paused" as const, positionMs: 1200, happenedMonoMs: 5000 }],
  spans: [],
};

/** How sending a report fails when the server answers it so, or "sent". */
async function sendingFailsWith(answer: Answers[string]): Promise<unknown> {
  const { api } = servedBy({ "POST /playback/abc/report": answer });
  return await sendReport(api, { mediaToken: "abc" }, report).then(
    () => "sent",
    (cause: unknown) => cause,
  );
}

let mounted: Mounted | undefined;

/** The video a mounted page shows. */
function videoOf(shown: Mounted): HTMLVideoElement {
  const video = shown.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  return video;
}

/**
 * Whether the page plays video at once, as it begins to load: its start gate is asked to play it,
 * and waits, with nothing loaded yet (useStartsWithEnoughAhead).
 */
function isPlayedAtOnce(video: HTMLVideoElement): boolean {
  video.dispatchEvent(new Event("loadstart"));
  return startGateOf(video).isWaiting();
}

/** Lets the requests a test's events started answer, and what they start in turn. */
async function settle(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("play state", () => {
  it("reports on the playback its media token names", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy({ "POST /playback/abc/report": [204, undefined] }, heard);
    await sendReport(api, { mediaToken: "abc" }, report);
    expect(heard).toEqual([{ methodAndPath: "POST /playback/abc/report", body: report }]);
  });

  it("hands back the admin's stop, which the server says in a report's answer (D243 §9)", async () => {
    const { api } = servedBy({
      "POST /playback/abc/report": [
        [200, {}],
        [200, { stop: { message: "Back soon" } }],
      ],
    });
    expect(await sendReport(api, { mediaToken: "abc" }, report)).toEqual({});
    expect(await sendReport(api, { mediaToken: "abc" }, report)).toEqual({
      stop: { message: "Back soon" },
    });
  });

  it("gives up on a playback the server refuses for good, and not on a passing failure", async () => {
    const gone = await sendingFailsWith(refusal(404, "not_found"));
    expect(gone instanceof ReportRefused && gone.cause instanceof ApiProblem).toBe(true);
    const busy = await sendingFailsWith(refusal(503, "migrating"));
    expect(busy instanceof ApiProblem && !(busy instanceof ReportRefused)).toBe(true);
    // A gateway's answer while the server restarts behind it.
    const restarting = await sendingFailsWith(refusal(502, "not_answering"));
    expect(restarting instanceof ApiProblem && !(restarting instanceof ReportRefused)).toBe(true);
    expect(await sendingFailsWith(unreachable)).toBeInstanceOf(TypeError);
  });

  it("tells the person when where they are is no longer kept", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "gone" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/gone/report": refusal(404, "not_found"),
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/gone/interest": [204, null],
            "DELETE /playback/gone": [204, null],
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
      video.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 0));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    // One report, refused for good, and none after; the player's word on what it wants beside it.
    const reports = heard.filter((request) => request.methodAndPath.endsWith("/report"));
    expect(reports.map((request) => request.methodAndPath)).toEqual(["POST /playback/gone/report"]);
    expect(heard.map((request) => request.methodAndPath)).toContain("PUT /playback/gone/interest");
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "Where you are is no longer being kept for next time. That is no longer here",
    );
  });

  it("opens a playback the server lost again where it was, paused as it was, and tells a second loss within a minute (the owner's test, 2026-10-18)", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        createElement(Play, { fileId: 2 }),
        servedBy(
          {
            "POST /playback": [
              [200, directDecision({ mediaToken: "lost", resumePositionMs: 0 })],
              [200, directDecision({ mediaToken: "again", resumePositionMs: 0 })],
            ],
            "POST /playback/lost/report": refusal(404, "playback_gone"),
            "POST /playback/again/report": refusal(404, "playback_gone"),
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/lost/interest": [204, null],
            "DELETE /playback/lost": [204, null],
            "PUT /playback/again/interest": [204, null],
            "DELETE /playback/again": [204, null],
          },
          heard,
        ),
      ),
    );
    const lost = videoOf(mounted);
    expect(isPlayedAtOnce(lost)).toBe(true);
    Object.defineProperty(lost, "readyState", { value: 4 });
    Object.defineProperty(lost, "currentTime", { value: 754.5, writable: true });
    await act(async () => {
      lost.dispatchEvent(new Event("timeupdate"));
      lost.dispatchEvent(new Event("pause")); // reported at once, and refused: the server lost it
      await settle();
    });
    const causes = heard
      .filter((request) => request.methodAndPath === "POST /playback")
      .map((request) => (request.body as { readonly cause: string }).cause);
    expect(causes).toEqual(["playPage", "playbackGone"]);
    const again = videoOf(mounted);
    expect(again).not.toBe(lost);
    expect(isPlayedAtOnce(again)).toBe(false); // the person had paused it
    expect(mounted.container.querySelector('[role="alert"]')).toBe(null);
    Object.defineProperty(again, "readyState", { value: 4 });
    await act(async () => {
      again.dispatchEvent(new Event("loadedmetadata"));
      await settle();
    });
    expect(again.currentTime).toBe(754.5); // where the lost one was
    await act(async () => {
      again.dispatchEvent(new Event("playing"));
      await settle();
    });
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "Reload the page to carry on from where you were",
    );
  });

  it("holds the lost video on screen while the server comes back, and opens the new one where it has got to", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        createElement(Play, { fileId: 2 }),
        servedBy(
          {
            "POST /playback": [
              [200, directDecision({ mediaToken: "lost" })],
              refusal(502, "not_answering"), // the server still starting
              [200, directDecision({ mediaToken: "again" })],
            ],
            "POST /playback/lost/report": refusal(404, "playback_gone"),
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/lost/interest": [204, null],
            "DELETE /playback/lost": [204, null],
            "DELETE /playback/again": [204, null],
          },
          heard,
        ),
      ),
    );
    const lost = videoOf(mounted);
    Object.defineProperty(lost, "readyState", { value: 4 });
    Object.defineProperty(lost, "currentTime", { value: 600, writable: true });
    await act(async () => {
      lost.dispatchEvent(new Event("timeupdate"));
      lost.dispatchEvent(new Event("playing")); // reported, and refused: the server lost it
      await settle();
    });
    expect(videoOf(mounted)).toBe(lost);
    expect(mounted.container.querySelector("output")?.textContent).toContain(
      "Opening the film again where you were",
    );
    lost.currentTime = 612; // it plays on from what it holds
    await act(async () => {
      lost.dispatchEvent(new Event("timeupdate"));
      await new Promise((resolve) => setTimeout(resolve, reopenWaitsMs[0]));
      await settle();
    });
    const again = videoOf(mounted);
    expect(again).not.toBe(lost);
    expect(isPlayedAtOnce(again)).toBe(true);
    expect(mounted.container.querySelector("output")).toBe(null);
    Object.defineProperty(again, "readyState", { value: 4 });
    await act(async () => {
      again.dispatchEvent(new Event("loadedmetadata"));
      await settle();
    });
    expect(again.currentTime).toBe(612);
  });

  it("reports at once when the player fails, so a playback lost before it played is learned of", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "failed" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/failed/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "GET /files/2/availability": [200, {}],
            "DELETE /playback/failed": [204, null],
          },
          heard,
        ),
      ),
    );
    const video = videoOf(mounted);
    await act(async () => {
      video.dispatchEvent(new Event("error"));
      await settle();
    });
    expect(heard.map((request) => request.methodAndPath)).toContain("POST /playback/failed/report");
  });

  it("moves to the lower quality a report's answer offers, and says which (D247 §2)", async () => {
    const offer = {
      url: "/api/v1/bytes/media/chunks/slow/master.m3u8",
      startAtBps: 1_992_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 2_400,
      linkSharePercent: 70,
      steadiness: 0,
    };
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "slow" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy({
          "POST /playback/slow/report": [200, { lowerQuality: offer }],
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "GET /files/2/availability": [200, {}],
          "PUT /playback/slow/interest": [204, null],
          "DELETE /playback/slow": [204, null],
        }),
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 4 });
    await act(async () => {
      video.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 0));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(mounted.container.textContent).toContain(
      "Your connection could not keep up, so this moves to a lower quality: 480p, 1.2 Mb/s.",
    );
    expect(mounted.container.textContent).not.toContain("Playing the original file");
  });

  it("tells, at the first playing, how long the playback took to start from asking how to play", async () => {
    const heard: Heard[] = [];
    const askedMs = performance.now() - 3000;
    noteDecisionTimes("timed", { askedMs, decidedMs: askedMs + 200 });
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "timed" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/timed/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/timed/interest": [204, null],
            "DELETE /playback/timed": [204, null],
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
      video.dispatchEvent(new Event("loadeddata"));
      video.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const sent = heard.find((request) => request.methodAndPath === "POST /playback/timed/report");
    expect(sent?.body).toMatchObject({ startup: { decidedAfterMs: 200 } });
    // How its sound reaches the speakers: a film played as it is, not through Web Audio, nothing
    // buffered by hls.js (2026-10-24).
    expect(sent?.body).toMatchObject({ startup: { sound: {} } });
    const { startup } = sent?.body as {
      startup: { firstDataAfterMs: number; playingAfterMs: number };
    };
    expect(startup.firstDataAfterMs).toBeGreaterThanOrEqual(3000);
    expect(startup.playingAfterMs).toBeGreaterThanOrEqual(startup.firstDataAfterMs);
  });

  it("tells, with each report, the steadiness level its chunked player stands at (D352 Q2)", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "steady" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/steady/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/steady/interest": [204, null],
            "DELETE /playback/steady": [204, null],
          },
          heard,
        ),
      ),
    );
    steadinessNoter(videoOf(mounted))(2);
    await act(async () => {
      videoOf(mounted!).dispatchEvent(new Event("playing"));
      await settle();
    });
    const sent = heard.find((request) => request.methodAndPath === "POST /playback/steady/report");
    expect(sent?.body).toMatchObject({ steadiness: 2 });
  });

  it("tells, with its start, what the film page's preload did before it (the owner's test, 2026-10-18)", async () => {
    const heard: Heard[] = [];
    const askedMs = performance.now() - 1000;
    noteDecisionTimes("preloaded", {
      askedMs,
      decidedMs: askedMs + 10,
      preloadSteps: "level 4; error bufferAppendNoProgress; level 0",
    });
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "preloaded" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy(
          {
            "POST /playback/preloaded/report": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "PUT /playback/preloaded/interest": [204, null],
            "DELETE /playback/preloaded": [204, null],
          },
          heard,
        ),
      ),
    );
    const video = videoOf(mounted);
    Object.defineProperty(video, "readyState", { value: 4 });
    await act(async () => {
      video.dispatchEvent(new Event("playing"));
      await settle();
    });
    const sent = heard.find(
      (request) => request.methodAndPath === "POST /playback/preloaded/report",
    );
    expect(sent?.body).toMatchObject({
      startup: { preloadSteps: "level 4; error bufferAppendNoProgress; level 0" },
    });
  });

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
    expect(events.find((event) => event.kind === "seeked")?.context).toMatchObject({
      skippedBy: "hole",
    });
    expect(bodies.flatMap((body) => body.pathMoves ?? []).map((move) => move.to)).toEqual([
      "WebRTC",
    ]);
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

describe("a quality the player cannot play here (D266)", () => {
  it("is reported at once, with where the video stood, and not once the reporting stops", () => {
    const video = new EventTarget();
    const recorded: [unknown, number][] = [];
    const stop = tellUnplayable(video, {
      recordUnplayable: (unplayable, positionMs) => recorded.push([unplayable, positionMs]),
    });
    const found = { qualityKbps: 36_208, positionMs: 420_500 };
    video.dispatchEvent(new CustomEvent(playerFoundUnplayableEvent, { detail: found }));
    stop();
    video.dispatchEvent(new CustomEvent(playerFoundUnplayableEvent, { detail: found }));
    expect(recorded).toEqual([[found, 420_500]]);
  });

  it("tells the page where the video stood when the film's own picture is given up, and only then (D323 §2)", () => {
    const video = new EventTarget();
    const told: unknown[] = [];
    const stop = whenOwnPictureGivenUp(video, (at) => told.push(at));
    const at = (detail: object): void => {
      video.dispatchEvent(new CustomEvent(playerFoundUnplayableEvent, { detail }));
    };
    at({ qualityKbps: 2000, positionMs: 100_000, isFilmsOwnPicture: false });
    at({ qualityKbps: 3096, positionMs: 420_500, isPaused: false, isFilmsOwnPicture: true });
    stop();
    at({ qualityKbps: 3096, positionMs: 500_000, isFilmsOwnPicture: true });
    expect(told).toEqual([{ positionMs: 420_500, isPaused: false }]);
  });

  it("keeps the decision times of the newest playbacks alone, as warm-ups never played go (the M4 closing audit)", () => {
    for (let index = 0; index <= decisionTimesKept; index++) {
      noteDecisionTimes(`kept-${String(index)}`, { askedMs: index, decidedMs: index + 1 });
    }
    expect(decisionTimesOf("kept-0")).toBeUndefined();
    expect(decisionTimesOf(`kept-${String(decisionTimesKept)}`)).toEqual({
      askedMs: decisionTimesKept,
      decidedMs: decisionTimesKept + 1,
    });
  });
});
