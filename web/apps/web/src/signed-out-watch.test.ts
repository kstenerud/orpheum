// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import {
  checkSessionWhileShown,
  sessionCheckEveryMs,
  watchForSignOut,
} from "./signed-out-watch.ts";
import { failOnUnexpectedRequests, servedBy, type Answer } from "./test-dom.tsx";
import { testOrigin } from "./test-services.tsx";

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

/** A navigation that keeps where it was told to go. */
function recordingNavigation(went: Route[]): Navigation {
  return {
    route: () => ({ screen: "play", fileId: 2 }),
    go: (route) => {
      went.push(route);
    },
    replace: (route) => {
      went.push(route);
    },
    back: (up) => {
      if (up !== undefined) {
        went.push(up);
      }
      return up !== undefined;
    },
    up: (parent) => {
      went.push(parent);
    },
    behind: () => [],
    origin: () => testOrigin,
    fragment: () => "",
    subscribe: () => () => undefined,
    beforeLeaving: () => () => undefined,
    noteOnEntry: () => undefined,
    entryNote: () => undefined,
    onBase: () => () => undefined,
  };
}

/** The reason a session signed in with a token is signed out with, when GET /me answers answer. */
async function reasonSignedOutWith(answer: Answer): Promise<string | undefined> {
  const services = servedBy({ "GET /me": answer }, [], "token");
  services.api.use(watchForSignOut(services.session, recordingNavigation([])));
  await services.api.GET("/me");
  return services.session.lastEndReason();
}

describe("a session the server ended", () => {
  it("reads the reason from an auth_required answer, and none from anything else", async () => {
    expect(
      await reasonSignedOutWith([
        401,
        { error: { code: "auth_required", message: "x", detail: { reason: "session_ended" } } },
      ]),
    ).toBe("session_ended");
    expect(
      await reasonSignedOutWith([401, { error: { code: "auth_required", message: "x" } }]),
    ).toBe("no_session");
    expect(await reasonSignedOutWith([401, "not our error shape"])).toBe("no_session");
  });

  it("believes a reason only in the server's error shape, read as the SDK reads every refusal", async () => {
    // No code or message: not the server's refusal, whatever it carries.
    expect(
      await reasonSignedOutWith([401, { error: { detail: { reason: "session_ended" } } }]),
    ).toBe("no_session");
  });

  it("signs out with the reason and goes home, and the device stays remembered", async () => {
    const services = servedBy(
      {
        "GET /me": [
          401,
          { error: { code: "auth_required", message: "x", detail: { reason: "session_ended" } } },
        ],
      },
      [],
      "token",
    );
    const went: Route[] = [];
    services.session.signIn("token", { deviceId: 42 });
    services.api.use(watchForSignOut(services.session, recordingNavigation(went)));
    await services.api.GET("/me");
    expect(services.session.token()).toBeUndefined();
    expect(services.session.lastEndReason()).toBe("session_ended");
    expect(went).toEqual([{ screen: "home" }]);
    expect(services.session.deviceId()).toBe(42);
    services.session.signIn("next");
    expect(services.session.lastEndReason()).toBeUndefined();
  });
});

describe("a session checked while its page is shown", () => {
  /** A page a test shows and hides, telling its listeners. */
  class FakePage {
    visibilityState: DocumentVisibilityState = "visible";
    readonly #listeners = new Set<() => void>();
    addEventListener(_type: "visibilitychange", listener: () => void): void {
      this.#listeners.add(listener);
    }
    removeEventListener(_type: "visibilitychange", listener: () => void): void {
      this.#listeners.delete(listener);
    }
    show(state: DocumentVisibilityState): void {
      this.visibilityState = state;
      for (const listener of this.#listeners) {
        listener();
      }
    }
  }

  it("is asked about as the page is shown again and every minute while visible, never hidden or signed out", () => {
    vi.useFakeTimers();
    let token: string | undefined = "token";
    let checks = 0;
    const page = new FakePage();
    const stop = checkSessionWhileShown(
      async () => {
        checks += 1;
        await Promise.resolve();
      },
      { token: () => token },
      page,
    );
    vi.advanceTimersByTime(sessionCheckEveryMs);
    expect(checks).toBe(1);
    page.show("hidden");
    vi.advanceTimersByTime(sessionCheckEveryMs);
    expect(checks).toBe(1);
    page.show("visible");
    expect(checks).toBe(2);
    token = undefined;
    vi.advanceTimersByTime(sessionCheckEveryMs);
    expect(checks).toBe(2);
    stop();
    vi.useRealTimers();
  });
});
