// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { sendFailuresToCore } from "./failure-reports.ts";
import { servedBy, type Heard } from "./test-dom.tsx";

describe("the page's failures", () => {
  afterEach(() => {
    vi.restoreAllMocks();
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
});
