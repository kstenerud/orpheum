// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { CastTargets } from "./cast-to.tsx";
import { LiveProvider } from "./live.tsx";
import { buttonWithText, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** A device of the person's, as GET /me/devices lists it. */
function device(id: number, name: string, isThisDevice = false): Record<string, unknown> {
  return {
    id,
    name,
    platform: "web",
    addedAt: 0,
    lastUsedAt: Date.UTC(2026, 9, 25, 20, 0),
    isSignedIn: true,
    isByCode: false,
    isThisDevice,
  };
}

describe("the device list as the live connection says what is open", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("offers no device, saying it reconnects, while the live connection is down (stale lists)", async () => {
    const services = servedBy(
      { "GET /me/devices": [200, { devices: [device(1, "Phone", true), device(3, "Fire TV")] }] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 3], inFrontDeviceIds: [1, 3] });
    expect(buttonWithText(mounted.container, "Fire TV")?.matches(":disabled")).toBe(false);
    expect(mounted.container.textContent).not.toContain("Reconnecting…");
    await server.drop();
    expect(buttonWithText(mounted.container, "Fire TV")?.matches(":disabled")).toBe(true);
    expect(mounted.container.textContent).toContain("Reconnecting…");
  });

  it("lists a device signed in while the list is open, as it connects (the owner, staging, 2026-11-07)", async () => {
    const routes: Record<string, Parameters<typeof servedBy>[0][string]> = {
      "GET /me/devices": [200, { devices: [device(1, "Mac", true)] }],
    };
    const services = servedBy(routes, [], "token");
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastTargets film={() => undefined} />
        </LiveProvider>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1] });
    expect(mounted.container.textContent).toContain("None of your other devices");
    routes["GET /me/devices"] = [200, { devices: [device(1, "Mac", true), device(2, "Phone")] }];
    await server.say({ type: "devices", deviceIds: [1, 2], inFrontDeviceIds: [2] });
    expect(mounted.container.textContent).toContain("Phone");
  });
});
