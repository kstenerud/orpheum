// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import { watchForSignOut } from "./signed-out-watch.ts";
import { servedBy, type Answer } from "./test-dom.tsx";
import { testOrigin } from "./test-services.tsx";

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
    origin: () => testOrigin,
    fragment: () => "",
    subscribe: () => () => undefined,
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
    services.session.signIn("token", 42);
    services.api.use(watchForSignOut(services.session, recordingNavigation(went)));
    await services.api.GET("/me");
    expect(services.session.token()).toBeUndefined();
    expect(services.session.lastEndReason()).toBe("session_ended");
    expect(went).toEqual([{ screen: "films" }]);
    expect(services.session.deviceId()).toBe(42);
    services.session.signIn("next");
    expect(services.session.lastEndReason()).toBeUndefined();
  });
});
