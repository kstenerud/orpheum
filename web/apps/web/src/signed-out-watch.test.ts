// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import { createSession } from "./session.ts";
import { signedOutReason, watchForSignOut } from "./signed-out-watch.ts";

/** A storage kept in memory. */
function memory(): {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
} {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

describe("a session the server ended", () => {
  it("reads the reason from an auth_required answer, and none from anything else", () => {
    expect(
      signedOutReason({ error: { code: "auth_required", detail: { reason: "session_ended" } } }),
    ).toBe("session_ended");
    expect(signedOutReason({ error: { code: "auth_required" } })).toBe("no_session");
    expect(signedOutReason("not json")).toBe("no_session");
  });

  it("signs out with the reason and goes home, and the device stays remembered", async () => {
    const storage = memory();
    const session = createSession(storage);
    session.signIn("token", 42);
    const went: Route[] = [];
    const navigation: Navigation = {
      route: () => ({ screen: "play", fileId: 2 }),
      go: (route) => {
        went.push(route);
      },
      subscribe: () => () => undefined,
    };
    // Answered by a stand-in for fetch, so the answer passes every watcher: tests never touch
    // the network.
    const answer = {
      error: { code: "auth_required", message: "x", detail: { reason: "session_ended" } },
    };
    const api = createApi({
      origin: "http://orpheum.test",
      getToken: session.token,
      fetch: () =>
        Promise.resolve(
          new Response(JSON.stringify(answer), {
            status: 401,
            headers: { "Content-Type": "application/json" },
          }),
        ),
    });
    api.use(watchForSignOut(session, navigation));
    await api.GET("/me");
    expect(session.token()).toBeUndefined();
    expect(session.lastEndReason()).toBe("session_ended");
    expect(went).toEqual([{ screen: "films" }]);
    expect(createSession(storage).deviceId()).toBe(42);
    session.signIn("next");
    expect(session.lastEndReason()).toBeUndefined();
  });
});
