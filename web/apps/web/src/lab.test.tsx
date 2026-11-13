// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { ReactNode } from "react";
import { prerender } from "react-dom/static";
import { describe, expect, it } from "vitest";

import { App, Screen } from "./app.tsx";
import { LabDevice } from "./lab-device.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { createNavigation } from "./navigation.ts";
import { parseRoute } from "./router.ts";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
} from "./test-dom.tsx";
import { signedInMarkupAt } from "./test-app.tsx";
import { fakePlace, testServices, withServices } from "./test-services.tsx";

/**
 * The markup of tree once its screens' code has loaded (later-screens.tsx), before it has read
 * anything.
 */
async function markupOf(tree: ReactNode): Promise<string> {
  const { prelude } = await prerender(tree);
  return await new Response(prelude).text();
}

/** The markup the app shows at path: signed in, once it settles (test-app.tsx); else before it has read anything. */
async function appAt(path: string, isSignedIn = true): Promise<string> {
  if (isSignedIn) {
    return await signedInMarkupAt(path);
  }
  return await markupOf(withServices(<App />, testServices(undefined, path)));
}

/** The markup of the screen at path, on a server that offers the device tests. */
async function screenAt(path: string): Promise<string> {
  return await markupOf(
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
      "GET /me/together": [200, { sessions: [] }],
      "GET /libraries": [200, { libraries: [] }],
    },
    [],
    "token",
  );
  const mounted = await mount(
    withServices(<App />, { ...services, navigation: createNavigation(fakePlace(path)) }),
  );
  // The lab's code loads as it is first shown (later-screens.tsx).
  await settleUntil(() => mounted.container.querySelector("main .lab") !== null);
  return mounted;
}

describe("the device lab", () => {
  it("is at /lab: a menu of its probes, each a big link, the cast probe's opened by its own load", async () => {
    const markup = await screenAt("/lab");
    expect(markup).toContain('id="lab-heading">Device lab<');
    for (const probe of ["device", "input", "sound", "remote", "extend", "throughput", "cast"]) {
      expect(markup).toContain(`href="/lab/${probe}"`);
    }
    // The UI gallery too, which is a pain to type on a TV (the owner, 2026-11-03).
    expect(markup).toContain('href="/ui-gallery"');
    // No key shows untranslated.
    expect(markup).not.toMatch(/>lab\./u);
  });

  it("shows each probe at its own address, within the signed-in app", async () => {
    expect(await screenAt("/lab/device")).toContain('id="lab-device-heading"');
    expect(await screenAt("/lab/input")).toContain('id="lab-input-heading"');
    expect(await screenAt("/lab/sound")).toContain('id="lab-sound-heading"');
    expect(await screenAt("/lab/remote")).toContain('id="lab-remote-heading"');
    expect(await screenAt("/lab/extend")).toContain('id="lab-extend-heading"');
    expect(await screenAt("/lab/throughput")).toContain('id="lab-throughput-heading"');
    expect(await screenAt("/lab/cast")).toContain('id="lab-cast-heading"');
    expect(await appAt("/lab/input")).toContain('class="app-shell"');
    for (const path of [
      "/lab/input",
      "/lab/sound",
      "/lab/extend",
      "/lab/throughput",
      "/lab/cast",
    ]) {
      expect(await screenAt(path)).not.toMatch(/>lab\./u);
    }
  });

  it("asks to sign in first, but the extend probe's player, which needs no server, plays bare", async () => {
    expect(await appAt("/lab/device", false)).toContain('<main class="credentials-form">');
    const player = await appAt("/lab/extend/player", false);
    expect(player).toContain('class="lab-player"');
    expect(player).not.toContain("app-shell");
  });

  it("is reached from This device, not the menu, and its pages say the tests are off while the server has them off (the owner, 2026-10-25)", async () => {
    const on = await mountedAt("/lab", true);
    expect(on.container.querySelector("#lab-heading")?.textContent).toBe("Device lab");
    // Behind "For testers" on This device (this-device.test.tsx), no longer the menu's
    // (ui-audit-2026-11-04.md Q8). The menu's places are drawn while it is open (drop-down.tsx).
    await click(on.container.querySelector(".app-shell__menu-button"));
    expect(on.container.querySelector("dialog")).not.toBeNull();
    expect(on.container.querySelector('.app-shell__menu a[href="/lab"]')).toBeNull();
    expect(on.container.querySelector('a[href="/lab/device"]')).not.toBeNull();
    on.unmount();
    for (const path of ["/lab", "/lab/device", "/lab/cast"]) {
      const off = await mountedAt(path, false);
      expect(off.container.querySelector("main")?.textContent).toContain(
        "Device tests are turned off on this server.",
      );
      await click(off.container.querySelector(".app-shell__menu-button"));
      expect(off.container.querySelector("dialog")).not.toBeNull();
      expect(off.container.querySelector('a[href="/lab"]')).toBeNull();
      expect(off.container.querySelector('a[href="/lab/device"]')).toBeNull();
      expect(off.container.querySelector("#lab-device-heading, #lab-cast-heading")).toBeNull();
      off.unmount();
    }
  });

  it("sends a probe's results to the server's log and shows the name the log keeps them by", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /lab/results": [200, { id: "K3J9QW2A" }] }, heard, "token");
    const mounted = await mount(withServices(<LabDevice />, services));
    await click(buttonWithText(mounted.container, "Send to server"));
    const sent = heard.find((request) => request.methodAndPath === "POST /lab/results");
    expect(sent === undefined ? undefined : bodyField(sent, "probe")).toBe("device");
    const result = sent === undefined ? undefined : bodyField(sent, "result");
    expect(Array.isArray(Reflect.get(Object(result), "codecs"))).toBe(true);
    expect(Reflect.get(Object(result), "userAgent")).toBe(navigator.userAgent);
    expect(mounted.container.textContent).toContain("Sent as K3J9QW2A");
    mounted.unmount();
  });
});
