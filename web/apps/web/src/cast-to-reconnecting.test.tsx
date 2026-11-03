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

describe("the device list while the live connection is down", () => {
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
    expect(buttonWithText(mounted.container, "Fire TV")?.disabled).toBe(false);
    expect(mounted.container.textContent).not.toContain("Reconnecting…");
    await server.drop();
    expect(buttonWithText(mounted.container, "Fire TV")?.disabled).toBe(true);
    expect(mounted.container.textContent).toContain("Reconnecting…");
  });
});
