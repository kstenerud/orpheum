// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { beforeEach, describe, expect, it } from "vitest";

import { servedBy } from "./test-dom.tsx";
import { haveBackMySessions } from "./together-mine.ts";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("having back the sessions a signed-in person is in (D267 §2)", () => {
  it("keeps the one they host in the tab and the one they joined in the browser", async () => {
    const { api } = servedBy(
      {
        "GET /together/mine": [
          200,
          {
            sessions: [
              { sessionId: 3, memberId: 1, isHost: true, membershipCredential: "hosting" },
              { sessionId: 7, memberId: 9, isHost: false, membershipCredential: "joined" },
            ],
          },
        ],
      },
      [],
      "token",
    );
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toContain("hosting");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("joined");
  });

  it("leaves a membership this device keeps for the same session as it is", async () => {
    const kept = JSON.stringify({ sessionId: 3, credential: "this-device" });
    window.sessionStorage.setItem("orpheum.together.host", kept);
    const { api } = servedBy(
      {
        "GET /together/mine": [
          200,
          { sessions: [{ sessionId: 3, memberId: 1, isHost: true, membershipCredential: "new" }] },
        ],
      },
      [],
      "token",
    );
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(kept);
  });
});
