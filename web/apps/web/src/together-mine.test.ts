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
              {
                sessionId: 7,
                memberId: 9,
                isHost: false,
                membershipCredential: "joined",
                locator: "joined-locator",
              },
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
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("joined-locator");
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

  it("forgets a session this tab hosts that the server no longer has, so its sign goes (the owner, 2026-10-11)", async () => {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "before-the-restart" }),
    );
    const viewing = JSON.stringify({ sessionId: 7, credential: "a-guest" });
    window.localStorage.setItem("orpheum.together.viewer", viewing);
    const { api } = servedBy({ "GET /together/mine": [200, { sessions: [] }] }, [], "token");
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    // A guest's membership is its page's to end: it hears the session is gone itself.
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBe(viewing);
  });

  it("keeps a hosted session when the server cannot be asked", async () => {
    const kept = JSON.stringify({ sessionId: 3, credential: "this-device" });
    window.sessionStorage.setItem("orpheum.together.host", kept);
    const { api } = servedBy({}, [], "token");
    await haveBackMySessions(api).catch(() => undefined);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(kept);
  });

  it("keeps a session started while the server answered", async () => {
    const { api } = servedBy({ "GET /together/mine": [200, { sessions: [] }] }, [], "token");
    const asking = haveBackMySessions(api);
    const started = JSON.stringify({ sessionId: 4, credential: "just-started" });
    window.sessionStorage.setItem("orpheum.together.host", started);
    await asking;
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(started);
  });
});
