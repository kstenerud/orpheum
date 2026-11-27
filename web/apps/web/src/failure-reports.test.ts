// SPDX-License-Identifier: MPL-2.0

import { noteForTheLog, reportFailure } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { sendFailuresToCore } from "./failure-reports.ts";
import { failOnUnexpectedRequests, servedBy, type Heard } from "./test-dom.tsx";

describe("the page's failures", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    // This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
    // test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
    failOnUnexpectedRequests();
  });

  it("reach the core's log with the page's path while signed in, and not otherwise", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const heard: Heard[] = [];
    const { api, session } = servedBy({ "POST /client-problems": [204, undefined] }, heard);
    sendFailuresToCore(api, session, () => "/play/17");
    reportFailure(new TypeError("Failed to fetch"));
    session.signIn("token", { deviceId: 5 });
    reportFailure(new TypeError("Failed to fetch"));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(heard).toEqual([
      {
        methodAndPath: "POST /client-problems",
        body: { message: "TypeError: Failed to fetch", page: "/play/17" },
      },
    ]);
  });

  // A Fire TV's sign-in submitted on a checkbox's OK (the owner, 2026-10-30): the note of what its
  // keys did is made signed out, and reaches the core's log once signed in.
  it("send a note at the debug level, one made signed out once the page signs in", async () => {
    vi.spyOn(console, "debug").mockImplementation(() => undefined);
    const heard: Heard[] = [];
    const { api, session } = servedBy({ "POST /client-problems": [204, undefined] }, heard);
    sendFailuresToCore(api, session, () => "/");
    noteForTheLog("a form's OK on a checkbox: keydown Enter/13");
    expect(heard).toEqual([]);
    session.signIn("token", { deviceId: 5 });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(heard).toEqual([
      {
        methodAndPath: "POST /client-problems",
        body: { message: "a form's OK on a checkbox: keydown Enter/13", page: "/", level: "debug" },
      },
    ]);
  });
});
