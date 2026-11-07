// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { use, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { App } from "./app.tsx";
import { mount, servedBy, settleUntil, type Answers } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { CanStartTogetherContext } from "./together-can-start.ts";

// Every screen is drawn in a LaterScreen, under what the signed-in app gives it: this one tells
// whether the overlay's Watch Together sign may offer a session to start, and draws nothing else.
vi.mock("./screen-load.tsx", async (original) => ({
  ...(await original<typeof import("./screen-load.tsx")>()),
  LaterScreen: (props: { readonly children: ReactNode }): ReactNode => {
    void props.children;
    return <output data-can-start={String(use(CanStartTogetherContext))} />;
  },
}));

const answers: Answers = {
  "GET /libraries": [200, { libraries: [] }],
  "GET /items/letters": [200, { letters: [] }],
  "GET /system/update": [200, { version: "v0.3.1", checkedAt: 0 }],
  "GET /client-settings": [
    200,
    {
      screenRefreshFirstWaitSeconds: 2,
      screenRefreshLongestWaitSeconds: 30,
      screenRefreshFailedWaitSeconds: 5,
      interestSaidEverySeconds: 30,
      shouldShowDeveloperPages: false,
      shouldOfferDeviceTests: false,
    },
  ],
  "GET /me/together": [200, { sessions: [] }],
};

/** What the screen is told of starting a session, for a person whose role may host or not. */
async function canStartFor(canHostWatchTogether: boolean): Promise<string | null | undefined> {
  const services = servedBy(
    {
      ...answers,
      "GET /me": [
        200,
        { id: 1, name: "owner", role: "user", isAdmin: false, canHostWatchTogether },
      ],
    },
    [],
    "token",
  );
  const mounted = await mount(withServices(<App />, services));
  const told = (): string | null | undefined =>
    mounted.container.querySelector("[data-can-start]")?.getAttribute("data-can-start");
  await settleUntil(() => told() === String(canHostWatchTogether));
  const value = told();
  mounted.unmount();
  return value;
}

describe("the signed-in app tells its screens who may start Watch Together (D440 §6)", () => {
  it("tells a person whose role may host that they may start one from the film", async () => {
    expect(await canStartFor(true)).toBe("true");
  });

  it("tells a person whose role may not that they may not", async () => {
    expect(await canStartFor(false)).toBe("false");
  });
});
