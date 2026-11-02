// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { createNavigation, type Navigation, type Place } from "./navigation.ts";
import type { Route } from "./router.ts";
import {
  checkSessionWhileShown,
  sessionCheckEveryMs,
  signingOutWithNoneHeld,
  watchForSignOut,
} from "./signed-out-watch.ts";
import { failOnUnexpectedRequests, servedBy, type Answer } from "./test-dom.tsx";
import { fakePlace, steppingLater, testOrigin } from "./test-services.tsx";

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
    noteInPlace: () => undefined,
    notedInPlace: () => undefined,
    onBase: () => () => undefined,
    onPersonsStep: () => () => undefined,
    holdBack: () => () => undefined,
    whenNoneHeld: (action) => {
      action();
    },
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

describe("a sign-out with a question held over the page (the owner's Fire TV, 2026-11-02)", () => {
  it("ends the session only once the held entry is stepped off, so no step follows to cancel its leaving", () => {
    const later = steppingLater(fakePlace("/"));
    let steps = 0;
    const counted: Place = {
      ...later.place,
      history: {
        get state() {
          return later.place.history.state;
        },
        back: () => {
          steps += 1;
          later.place.history.back();
        },
        go: (delta) => {
          steps += 1;
          later.place.history.go(delta);
        },
        pushState: (data, unused, url) => {
          later.place.history.pushState(data, unused, url);
        },
        replaceState: (data, unused, url) => {
          later.place.history.replaceState(data, unused, url);
        },
      },
    };
    const navigation = createNavigation(counted, false);
    navigation.go({ screen: "films" });
    let closed = 0;
    const release = navigation.holdBack(() => {
      closed += 1;
    });
    const given = servedBy({}, [], "token").session;
    let stepsAtSignOut: number | undefined;
    given.subscribe(() => {
      stepsAtSignOut ??= steps;
    });
    signingOutWithNoneHeld(given, navigation).signOut();
    expect(given.token()).toBe("token");
    later.settle();
    expect(given.token()).toBeUndefined();
    // The question closes as the signed-out page replaces it: its release steps nothing more.
    release();
    later.settle();
    expect(steps).toBe(stepsAtSignOut);
    expect(closed).toBe(0);
    expect(navigation.route()).toEqual({ screen: "films" });
  });

  it("signs out because the server said so in the same way, with its reason", () => {
    const later = steppingLater(fakePlace("/"));
    const navigation = createNavigation(later.place, false);
    navigation.holdBack(() => undefined);
    const given = servedBy({}, [], "token").session;
    signingOutWithNoneHeld(given, navigation).signOutBecause("session_ended");
    expect(given.token()).toBe("token");
    later.settle();
    expect(given.token()).toBeUndefined();
    expect(given.lastEndReason()).toBe("session_ended");
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
