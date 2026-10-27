// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { LiveProvider } from "./live.tsx";
import { watchForSignOut } from "./signed-out-watch.ts";
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
});
