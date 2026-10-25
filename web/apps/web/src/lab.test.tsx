// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { LabDevice } from "./lab-device.tsx";
import { bodyField, buttonWithText, click, mount, servedBy, type Heard } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** The markup the app shows at path, signed in or not. */
function appAt(path: string, isSignedIn = true): string {
  return renderToStaticMarkup(
    withServices(<App />, testServices(isSignedIn ? "token" : undefined, path)),
  );
}

describe("the device lab", () => {
  it("is at /lab: a menu of its probes, each a big link, the cast probe's opened by its own load", () => {
    const markup = appAt("/lab");
    expect(markup).toContain('id="lab-heading">Device lab<');
    for (const probe of ["device", "input", "remote", "extend", "cast"]) {
      expect(markup).toContain(`href="/lab/${probe}"`);
    }
    // No key shows untranslated.
    expect(markup).not.toMatch(/>lab\./u);
  });

  it("shows each probe at its own address, within the signed-in app", () => {
    expect(appAt("/lab/device")).toContain('id="lab-device-heading"');
    expect(appAt("/lab/input")).toContain('id="lab-input-heading"');
    expect(appAt("/lab/remote")).toContain('id="lab-remote-heading"');
    expect(appAt("/lab/extend")).toContain('id="lab-extend-heading"');
    expect(appAt("/lab/cast")).toContain('id="lab-cast-heading"');
    expect(appAt("/lab/input")).toContain('class="app-shell"');
    for (const path of ["/lab/input", "/lab/extend", "/lab/cast"]) {
      expect(appAt(path)).not.toMatch(/>lab\./u);
    }
  });

  it("asks to sign in first, but the extend probe's player, which needs no server, plays bare", () => {
    expect(appAt("/lab/device", false)).toContain('<main class="credentials-form">');
    const player = appAt("/lab/extend/player", false);
    expect(player).toContain('class="lab-player"');
    expect(player).not.toContain("app-shell");
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
