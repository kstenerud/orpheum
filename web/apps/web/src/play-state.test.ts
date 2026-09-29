// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { sendReport } from "./play-state.ts";
import { servedBy, type Heard } from "./test-dom.tsx";

describe("play state", () => {
  it("reports on the playback its media token names", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy({ "POST /playback/abc/report": [204, undefined] }, heard);
    const report = {
      reportSeq: 1,
      sentMonoMs: 5000,
      positionMs: 1200,
      positionReadMonoMs: 5000,
      events: [{ kind: "paused" as const, positionMs: 1200, happenedMonoMs: 5000 }],
      spans: [],
    };
    await sendReport(api, "abc", report);
    expect(heard).toEqual([{ methodAndPath: "POST /playback/abc/report", body: report }]);
  });
});
