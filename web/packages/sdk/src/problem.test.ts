// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ApiProblem,
  failuresSentMost,
  failureText,
  reportFailure,
  sendFailuresTo,
} from "./problem.ts";

describe("reportFailure", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    // What a test reported before any sink is held for the next one's: sent nowhere, here.
    sendFailuresTo(() => undefined);
  });

  it("writes the failure's cause itself to the console, so what the words left out can be read", () => {
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const cause = new SyntaxError("JSON.parse: unexpected end of data");
    reportFailure(cause);
    expect(written).toHaveBeenCalledOnce();
    expect(written.mock.calls[0]).toContain(cause);
  });

  it("sends each failure on to where the page sends them, its kind and its words, a few per page", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    reportFailure(new TypeError("Failed to fetch"));
    reportFailure(new ApiProblem({ code: "notFound", message: "no such film" }));
    expect(sent).toEqual(["TypeError: Failed to fetch", "notFound: no such film"]);
    for (let count = 0; count < failuresSentMost + 5; count += 1) {
      reportFailure(new Error("again"));
    }
    expect(sent).toHaveLength(failuresSentMost);
    expect(failureText("x".repeat(600))).toHaveLength(500);
  });

  // A sign-in's own connections are made before the page has anywhere to send failures, and one that
  // took half a minute said so nowhere (staging, 2026-10-23).
  it("holds what fails before the page sends failures anywhere, and sends it once it does", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.resetModules();
    const fresh = await import("./problem.ts");
    fresh.reportFailure(new Error("before the sign-in"));
    const sent: string[] = [];
    fresh.sendFailuresTo((text) => sent.push(text));
    fresh.reportFailure(new Error("after it"));
    expect(sent).toEqual(["Error: before the sign-in", "Error: after it"]);
  });

  // A Fire TV's sign-in submitted on a checkbox's OK, and nothing said what the remote's keys had
  // done (the owner, 2026-10-30).
  it("sends a note for the log at the debug level, a failure as a failure, held alike until there is a sink", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.spyOn(console, "debug").mockImplementation(() => undefined);
    vi.resetModules();
    const fresh = await import("./problem.ts");
    fresh.noteForTheLog("a form's OK, held");
    const sent: [string, string][] = [];
    fresh.sendFailuresTo((text, level) => sent.push([text, level]));
    fresh.noteForTheLog("x".repeat(600));
    fresh.reportFailure(new Error("a failure"));
    expect(sent).toEqual([
      ["a form's OK, held", "debug"],
      ["x".repeat(500), "debug"],
      ["Error: a failure", "failure"],
    ]);
  });
});
