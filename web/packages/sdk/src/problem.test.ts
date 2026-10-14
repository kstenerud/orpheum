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
    reportFailure(new ApiProblem({ code: "not_found", message: "no such film" }));
    expect(sent).toEqual(["TypeError: Failed to fetch", "not_found: no such film"]);
    for (let count = 0; count < failuresSentMost + 5; count += 1) {
      reportFailure(new Error("again"));
    }
    expect(sent).toHaveLength(failuresSentMost);
    expect(failureText("x".repeat(600))).toHaveLength(500);
  });
});
