// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { LiveProvider } from "./live.tsx";
import { createLiveOpenMark, watchForSignOut } from "./signed-out-watch.ts";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

describe("the page's live connection (D337 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("signs the page out at once when the server says another device removed this one", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "GET /me": [401, { title: "Unauthorized", status: 401, detail: "session ended" }] },
      heard,
      "token",
    );
    services.api.use(watchForSignOut(services.session, services.navigation));
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <p>the films</p>
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    expect(heard.map((request) => request.methodAndPath)).toEqual([]);
    await server.say({ type: "signedOut" });
    await expect.poll(() => services.session.token()).toBeUndefined();
    expect(heard.map((request) => request.methodAndPath)).toEqual(["GET /me"]);
  });

  it("marks the live connection open while it is, so the session's minute check rests, and not once it drops", async () => {
    const liveOpen = createLiveOpenMark();
    const server = fakeTogetherServer();
    const services = servedBy({}, [], "token");
    mounted = await mount(
      withServices(
        <LiveProvider>
          <p>the films</p>
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket, liveOpen },
      ),
    );
    expect(liveOpen.isOpen()).toBe(false);
    await server.accept();
    await server.say({
      type: "devices",
      deviceIds: [1],
      inFrontDeviceIds: [1],
      receivingDeviceIds: [],
    });
    expect(liveOpen.isOpen()).toBe(true);
    await server.drop();
    expect(liveOpen.isOpen()).toBe(false);
  });
});
