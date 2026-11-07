// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { failOnUnexpectedRequests, refusal, servedBy, unreachable } from "./test-dom.tsx";
import {
  haveBackMySessions,
  moveHostingHere,
  noteHostedElsewhere,
  readHostedElsewhere,
} from "./together-mine.ts";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  noteHostedElsewhere(undefined);
});

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

describe("having back the sessions a signed-in person is in (D267 §2)", () => {
  it("keeps the one they joined in the browser, and none for the one they host on another device, noted as on there, by its name (D442 §1)", async () => {
    const { api } = servedBy(
      {
        "GET /me/together": [
          200,
          {
            sessions: [
              { sessionId: 3, memberId: 1, isHost: true, hostDeviceName: "the Mac" },
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
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(readHostedElsewhere()).toEqual({ sessionId: 3, deviceName: "the Mac" });
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("joined");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("joined-locator");
  });

  it("has back a session the person hosts on another device as a guest's neither, though another device of theirs joined it as a guest (the owner's Chrome, 2026-10-29; D442 §1)", async () => {
    const { api } = servedBy(
      {
        "GET /me/together": [
          200,
          {
            sessions: [
              { sessionId: 45, memberId: 46, isHost: true },
              { sessionId: 45, memberId: 50, isHost: false, membershipCredential: "the-phone's" },
            ],
          },
        ],
      },
      [],
      "token",
    );
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(readHostedElsewhere()).toEqual({ sessionId: 45, deviceName: undefined });
  });

  it("keeps hosting on the device that started the session, its page reloaded, noting it hosted nowhere else (D442 §1)", async () => {
    const kept = JSON.stringify({ sessionId: 3, credential: "this-device" });
    window.sessionStorage.setItem("orpheum.together.host", kept);
    const { api } = servedBy(
      {
        "GET /me/together": [
          200,
          { sessions: [{ sessionId: 3, memberId: 1, isHost: true, hostDeviceName: "this one" }] },
        ],
      },
      [],
      "token",
    );
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(kept);
    expect(readHostedElsewhere()).toBeUndefined();
  });

  it("moves the session hosted on another device here, this tab its host from then on (D442 §2)", async () => {
    noteHostedElsewhere({ sessionId: 3, deviceName: "the Mac" });
    const { api } = servedBy(
      { "POST /together/3/host-here": [200, { sessionId: 3, membershipCredential: "moved-here" }] },
      [],
      "token",
    );
    await moveHostingHere(api, 3, "key");
    expect(window.sessionStorage.getItem("orpheum.together.host")).toContain("moved-here");
    expect(readHostedElsewhere()).toBeUndefined();
  });

  it("forgets a session hosted on another device that the server no longer has, as it is moved here, so a new one may start", async () => {
    noteHostedElsewhere({ sessionId: 3, deviceName: "the Mac" });
    const { api } = servedBy(
      { "POST /together/3/host-here": refusal(404, "not_found") },
      [],
      "token",
    );
    await moveHostingHere(api, 3, "key");
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(readHostedElsewhere()).toBeUndefined();
  });

  it("forgets a session this tab hosts that the server no longer has, so its sign goes (the owner, 2026-10-11)", async () => {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "before-the-restart" }),
    );
    const viewing = JSON.stringify({ sessionId: 7, credential: "a-guest" });
    window.localStorage.setItem("orpheum.together.viewer", viewing);
    const { api } = servedBy({ "GET /me/together": [200, { sessions: [] }] }, [], "token");
    await haveBackMySessions(api);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    // A guest's membership is its page's to end: it hears the session is gone itself.
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBe(viewing);
  });

  it("keeps a hosted session when the server cannot be asked", async () => {
    const kept = JSON.stringify({ sessionId: 3, credential: "this-device" });
    window.sessionStorage.setItem("orpheum.together.host", kept);
    const { api } = servedBy({ "GET /me/together": unreachable }, [], "token");
    await haveBackMySessions(api).catch(() => undefined);
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(kept);
  });

  it("keeps a session started while the server answered", async () => {
    const { api } = servedBy({ "GET /me/together": [200, { sessions: [] }] }, [], "token");
    const asking = haveBackMySessions(api);
    const started = JSON.stringify({ sessionId: 4, credential: "just-started" });
    window.sessionStorage.setItem("orpheum.together.host", started);
    await asking;
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBe(started);
  });
});
