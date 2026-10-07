// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { ReportRefused } from "@orpheum/player";
import { act, createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import { directDecision } from "./test-fixtures.ts";
import { sendReport } from "./play-state.ts";
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
});
