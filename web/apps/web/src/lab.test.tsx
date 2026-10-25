// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App, Screen } from "./app.tsx";
import { LabDevice } from "./lab-device.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { createNavigation } from "./navigation.ts";
import { parseRoute } from "./router.ts";
import { bodyField, buttonWithText, click, mount, servedBy, type Heard } from "./test-dom.tsx";
import { fakePlace, testServices, withServices } from "./test-services.tsx";

/** The markup the app shows at path, signed in or not, before it has read anything. */
function appAt(path: string, isSignedIn = true): string {
  return renderToStaticMarkup(
    withServices(<App />, testServices(isSignedIn ? "token" : undefined, path)),
  );
}

/** The markup of the screen at path, on a server that offers the device tests. */
function screenAt(path: string): string {
  return renderToStaticMarkup(
    withServices(
      <DeviceTestsOfferedContext value={true}>
        <Screen route={parseRoute(path, "")} />
      </DeviceTestsOfferedContext>,
      testServices("token", path),
    ),
  );
}

/** The app mounted at path, signed in, against a server whose device tests are on or off. */
async function mountedAt(
  path: string,
  isOffered: boolean,
): Promise<Awaited<ReturnType<typeof mount>>> {
  const services = servedBy(
    {
      "GET /me": [200, { id: 1, name: "tester", role: "user", isAdmin: false }],
      "GET /client-settings": [
        200,
        {
          screenRefreshFirstWaitSeconds: 2,
          screenRefreshLongestWaitSeconds: 30,
          screenRefreshFailedWaitSeconds: 5,
          interestSaidEverySeconds: 15,
          shouldShowDeveloperPages: false,
          shouldOfferDeviceTests: isOffered,
        },
      ],
    },
    [],
    "token",
  );
  return await mount(
    withServices(<App />, { ...services, navigation: createNavigation(fakePlace(path)) }),
  );
}

describe("the device lab", () => {
  it("is at /lab: a menu of its probes, each a big link, the cast probe's opened by its own load", () => {
    const markup = screenAt("/lab");
    expect(markup).toContain('id="lab-heading">Device lab<');
    for (const probe of ["device", "input", "remote", "extend", "cast"]) {
      expect(markup).toContain(`href="/lab/${probe}"`);
    }
    // No key shows untranslated.
    expect(markup).not.toMatch(/>lab\./u);
  });

  it("shows each probe at its own address, within the signed-in app", () => {
    expect(screenAt("/lab/device")).toContain('id="lab-device-heading"');
    expect(screenAt("/lab/input")).toContain('id="lab-input-heading"');
    expect(screenAt("/lab/remote")).toContain('id="lab-remote-heading"');
    expect(screenAt("/lab/extend")).toContain('id="lab-extend-heading"');
    expect(screenAt("/lab/cast")).toContain('id="lab-cast-heading"');
    expect(appAt("/lab/input")).toContain('class="app-shell"');
    for (const path of ["/lab/input", "/lab/extend", "/lab/cast"]) {
      expect(screenAt(path)).not.toMatch(/>lab\./u);
    }
  });

  it("asks to sign in first, but the extend probe's player, which needs no server, plays bare", () => {
    expect(appAt("/lab/device", false)).toContain('<main class="credentials-form">');
    const player = appAt("/lab/extend/player", false);
    expect(player).toContain('class="lab-player"');
    expect(player).not.toContain("app-shell");
  });

  it("is reached by Device tests in the menu, and its pages say the tests are off while the server has them off (the owner, 2026-10-25)", async () => {
    const on = await mountedAt("/lab", true);
    expect(on.container.querySelector("#lab-heading")?.textContent).toBe("Device lab");
    expect(on.container.querySelector('.app-shell__watching a[href="/lab"]')?.textContent).toBe(
      "Device tests",
    );
    expect(on.container.querySelector('a[href="/lab/device"]')).not.toBeNull();
    on.unmount();
    for (const path of ["/lab", "/lab/device", "/lab/cast"]) {
      const off = await mountedAt(path, false);
      expect(off.container.querySelector("main")?.textContent).toContain(
        "Device tests are turned off on this server.",
      );
      expect(off.container.querySelector('a[href="/lab"]')).toBeNull();
      expect(off.container.querySelector('a[href="/lab/device"]')).toBeNull();
      expect(off.container.querySelector("#lab-device-heading, #lab-cast-heading")).toBeNull();
      off.unmount();
    }
  });

  it("sends a probe's results to the server's log and shows the name the log keeps them by", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /lab-results": [200, { id: "K3J9QW2A" }] }, heard, "token");
    const mounted = await mount(withServices(<LabDevice />, services));
    await click(buttonWithText(mounted.container, "Send to server"));
    const sent = heard.find((request) => request.methodAndPath === "POST /lab-results");
    expect(sent === undefined ? undefined : bodyField(sent, "probe")).toBe("device");
    const result = sent === undefined ? undefined : bodyField(sent, "result");
    expect(Array.isArray(Reflect.get(Object(result), "codecs"))).toBe(true);
    expect(Reflect.get(Object(result), "userAgent")).toBe(navigator.userAgent);
    expect(mounted.container.textContent).toContain("Sent as K3J9QW2A");
    mounted.unmount();
  });
});
