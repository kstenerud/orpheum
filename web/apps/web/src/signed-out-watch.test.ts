// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import { signedOutReason, watchForSignOut } from "./signed-out-watch.ts";
import { servedBy } from "./test-dom.tsx";

describe("a session the server ended", () => {
  it("reads the reason from an auth_required answer, and none from anything else", () => {
    expect(
      signedOutReason({ error: { code: "auth_required", detail: { reason: "session_ended" } } }),
    ).toBe("session_ended");
    expect(signedOutReason({ error: { code: "auth_required" } })).toBe("no_session");
    expect(signedOutReason("not json")).toBe("no_session");
  });

  it("signs out with the reason and goes home", async () => {
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
    const navigation: Navigation = {
      route: () => ({ screen: "play", fileId: 2 }),
      go: (route) => {
        went.push(route);
      },
      subscribe: () => () => undefined,
    };
    services.session.signIn("token", 42);
    services.api.use(watchForSignOut(services.session, navigation));
    await services.api.GET("/me");
    expect(services.session.token()).toBeUndefined();
    expect(services.session.lastEndReason()).toBe("session_ended");
    expect(went).toEqual([{ screen: "films" }]);
    expect(services.session.deviceId()).toBe(42);
    services.session.signIn("next");
    expect(services.session.lastEndReason()).toBeUndefined();
  });
});
