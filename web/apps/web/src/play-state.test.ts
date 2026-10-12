// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { playerFoundUnplayableEvent, ReportRefused } from "@orpheum/player";
import { act, createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import { directDecision } from "./test-fixtures.ts";
import {
  decisionTimesKept,
  decisionTimesOf,
  noteDecisionTimes,
  sendReport,
  tellUnplayable,
} from "./play-state.ts";
import { ApiProblem } from "./problem.ts";
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
        servedBy({ "POST /playback/gone/report": refusal(404, "not_found") }, heard),
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
    expect(heard.map((request) => request.methodAndPath)).toEqual(["POST /playback/gone/report"]);
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "Where you are is no longer being kept for next time. That is no longer here",
    );
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
    };
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "slow" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(
        playView,
        servedBy({ "POST /playback/slow/report": [200, { lowerQuality: offer }] }),
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
    noteDecisionTimes("timed", askedMs, askedMs + 200);
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "timed" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(playView, servedBy({ "POST /playback/timed/report": [204, undefined] }, heard)),
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
    const { startup } = sent?.body as {
      startup: { firstDataAfterMs: number; playingAfterMs: number };
    };
    expect(startup.firstDataAfterMs).toBeGreaterThanOrEqual(3000);
    expect(startup.playingAfterMs).toBeGreaterThanOrEqual(startup.firstDataAfterMs);
  });

  it("tells how long each of the person's seeks took to play again, from its last seeking (the owner, 2026-10-12)", async () => {
    const heard: Heard[] = [];
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "sought" }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(
      withServices(playView, servedBy({ "POST /playback/sought/report": [204, undefined] }, heard)),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 4 });
    Object.defineProperty(video, "paused", { value: false, configurable: true });
    await act(async () => {
      video.currentTime = 30;
      video.dispatchEvent(new Event("timeupdate"));
      video.currentTime = 600;
      video.dispatchEvent(new Event("seeking"));
      await new Promise((resolve) => setTimeout(resolve, 20));
      video.dispatchEvent(new Event("playing"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    const seeks = heard
      .filter((request) => request.methodAndPath === "POST /playback/sought/report")
      .flatMap((request) => (request.body as { seeks?: Record<string, number>[] }).seeks ?? []);
    expect(seeks).toHaveLength(1);
    expect(seeks[0]).toMatchObject({ fromPositionMs: 30_000, toPositionMs: 600_000 });
    expect(seeks[0]?.["readyAfterMs"]).toBeGreaterThanOrEqual(15);
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
    };
    const playView = createElement(PlayView, {
      fileId: 2,
      decision: directDecision({ mediaToken: "remembered", onLadder }),
      onDecideAgain: () => undefined,
    });
    mounted = await mount(withServices(playView, servedBy({})));
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

  it("keeps the decision times of the newest playbacks alone, as warm-ups never played go (the M4 closing audit)", () => {
    for (let index = 0; index <= decisionTimesKept; index++) {
      noteDecisionTimes(`kept-${String(index)}`, index, index + 1);
    }
    expect(decisionTimesOf("kept-0")).toBeUndefined();
    expect(decisionTimesOf(`kept-${String(decisionTimesKept)}`)).toEqual({
      askedMs: decisionTimesKept,
      decidedMs: decisionTimesKept + 1,
    });
  });
});
