// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { playerFoundUnplayableEvent, ReportRefused, steadinessNoter } from "@orpheum/player";
import { startGateOf } from "@orpheum/sdk";
import { act, createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Play, PlayView } from "./play.tsx";
import { directDecision } from "./test-fixtures.ts";
import { sendReport, tellUnplayable, whenOwnPictureGivenUp } from "./play-state.ts";
import {
  decisionTimesKept,
  decisionTimesOf,
  noteDecisionTimes,
} from "./play-state-video-events.ts";
import { ApiProblem } from "./problem.ts";
import { testServices } from "./test-services.tsx";
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

  it("sends the report of a leaving page with keepalive, and a body too big for it without (L1)", async () => {
    const kept: boolean[] = [];
    const { api } = testServices(undefined, "/films", (request) => {
      kept.push(request.keepalive);
      return Promise.resolve(new Response(null, { status: 204 }));
    });
    const to = { mediaToken: "abc" };
    await sendReport(api, { ...to, isLeaving: true }, report);
    await sendReport(api, { ...to, isLeaving: false }, report);
    await sendReport(api, to, report);
    const huge = { ...report, linkNotes: ["x".repeat(60_000)] };
    await sendReport(api, { ...to, isLeaving: true }, huge);
    expect(kept).toEqual([true, false, false, false]);
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
    const video = document.createElement("video");
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

  it("opens the twin playing when the own picture is given up before its first frame, which reads as paused (the owner's test, 2026-11-03)", () => {
    const video = document.createElement("video");
    const told: unknown[] = [];
    const stop = whenOwnPictureGivenUp(video, (at) => told.push(at));
    video.dispatchEvent(
      new CustomEvent(playerFoundUnplayableEvent, {
        detail: {
          qualityKbps: 9412,
          positionMs: 0,
          readyState: 1,
          isPaused: true,
          isFilmsOwnPicture: true,
        },
      }),
    );
    stop();
    expect(told).toEqual([{ positionMs: 0, isPaused: false }]);
  });

  it("opens the twin paused when the person paused the own picture before it was given up", () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "readyState", { value: 4 });
    const told: unknown[] = [];
    const stop = whenOwnPictureGivenUp(video, (at) => told.push(at));
    video.dispatchEvent(new Event("pause"));
    video.dispatchEvent(
      new CustomEvent(playerFoundUnplayableEvent, {
        detail: { qualityKbps: 9412, positionMs: 61_000, isPaused: true, isFilmsOwnPicture: true },
      }),
    );
    stop();
    expect(told).toEqual([{ positionMs: 61_000, isPaused: true }]);
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
