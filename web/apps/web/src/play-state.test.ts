// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { sendReport } from "./play-state.ts";

describe("play state", () => {
  it("reports on the playback its media token names", async () => {
    let sent: { url: string; method: string; body: unknown } | undefined;
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      async onRequest({ request }) {
        sent = { url: request.url, method: request.method, body: await request.json() };
        return new Response(null, { status: 204 });
      },
    });
    const report = {
      reportSeq: 1,
      sentMonoMs: 5000,
      positionMs: 1200,
      positionReadMonoMs: 5000,
      events: [{ kind: "paused" as const, positionMs: 1200, happenedMonoMs: 5000 }],
      spans: [],
    };
    await sendReport(api, "abc", report);
    expect(sent).toEqual({
      url: "http://orpheum.test/api/v1/playback/abc/report",
      method: "POST",
      body: report,
    });
  });
});
