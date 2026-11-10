// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { SystemNetwork } from "./system-network.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const home = { httpsPort: 8443, webrtcPort: 8478, router: "192.168.1.1" };
const steady = {
  kept: home,
  running: home,
  fellBack: [],
  pending: false,
  appliedBy: "restart",
} as const;

async function mountNetwork(
  answers: Parameters<typeof servedBy>[0],
  heard: Heard[] = [],
): Promise<Mounted> {
  mounted = await mount(withServices(<SystemNetwork />, servedBy(answers, heard)));
  return mounted;
}

/** Types value into the field labelled label, as a person does. */
async function typeInto(shown: Mounted, label: string, value: string): Promise<void> {
  await enter(
    [...shown.container.querySelectorAll("label")]
      .find((each) => each.textContent.startsWith(label))
      ?.querySelector("input"),
    value,
  );
}

const saveButton = (shown: Mounted): HTMLButtonElement | undefined =>
  buttonWithText(shown.container, "Save");

// D462 §2, §10.11: the house's network settings are the app's; the System page shows what runs,
// keeps new ones for the next start, says a kept port that would not bind and the default taken in
// its place, and offers what takes the settings: its restart, or the install script's next run.
describe("The house's network", () => {
  it("shows the ports and router the server runs on, the form filled with those kept", async () => {
    const shown = await mountNetwork({ "GET /system/network": [200, steady] });
    await settleUntil(() => saveButton(shown) !== undefined);
    expect(shown.container.textContent).toContain(
      "The server runs on HTTPS port 8443 and WebRTC port 8478. It asks the router at 192.168.1.1 to forward the HTTPS port.",
    );
    expect([...shown.container.querySelectorAll("input")].map((each) => each.value)).toEqual([
      "8443",
      "8478",
      "192.168.1.1",
    ]);
    // Nothing kept waits for a start: no restart offered.
    expect(buttonWithText(shown.container, "Restart the server")).toBeUndefined();
  });

  it("is no section on a server that keeps no network settings", async () => {
    const heard: Heard[] = [];
    const shown = await mountNetwork(
      {
        "GET /system/network": [
          409,
          { error: { code: "conflict", message: "no network settings" } },
        ],
      },
      heard,
    );
    await settleUntil(() => heard.length === 1);
    await settleUntil(() => true);
    expect(shown.container.textContent).toBe("");
  });

  it("keeps new settings, then offers the restart that takes them", async () => {
    const heard: Heard[] = [];
    const moved = { ...home, httpsPort: 9443 };
    const shown = await mountNetwork(
      {
        "GET /system/network": [
          [200, steady],
          [200, { ...steady, kept: moved, pending: true }],
        ],
        "PUT /system/network": [200, { ...steady, kept: moved, pending: true }],
        "POST /system/restart": [202, null],
      },
      heard,
    );
    await settleUntil(() => saveButton(shown) !== undefined);
    await typeInto(shown, "HTTPS port", "9443");
    await click(saveButton(shown));
    await settleUntil(() =>
      shown.container.textContent.includes("take effect when the server restarts"),
    );
    const put = heard.find((each) => each.methodAndPath === "PUT /system/network");
    expect(put === undefined ? undefined : bodyField(put, "httpsPort")).toBe(9443);
    expect(put === undefined ? undefined : bodyField(put, "router")).toBe("192.168.1.1");
    await click(buttonWithText(shown.container, "Restart the server"));
    await click(buttonWithText(shown.container, "Restart"));
    await settleUntil(() => shown.container.textContent.includes("The server is restarting."));
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /system/restart");
  });

  it("says the install script's next run takes them on the script's way, with no restart here", async () => {
    const shown = await mountNetwork({
      "GET /system/network": [200, { ...steady, pending: true, appliedBy: "script" }],
    });
    await settleUntil(() => saveButton(shown) !== undefined);
    expect(shown.container.textContent).toContain("take effect when the install script runs next");
    expect(buttonWithText(shown.container, "Restart the server")).toBeUndefined();
  });

  it("says a kept port that would not bind at the start, and the default the server took", async () => {
    const shown = await mountNetwork({
      "GET /system/network": [
        200,
        {
          ...steady,
          kept: { ...home, httpsPort: 443 },
          running: { ...home, httpsPort: 8443 },
          fellBack: ["httpsPort"],
        },
      ],
    });
    await settleUntil(() => saveButton(shown) !== undefined);
    expect(shown.container.querySelector('[role="alert"]')?.textContent).toBe(
      "The HTTPS port 443 was taken by something else when the server started, so it runs on 8443. Choose a free port, or free that one, then restart.",
    );
    expect(buttonWithText(shown.container, "Restart the server")).toBeDefined();
  });

  it("names the setting the server refused, in plain words", async () => {
    const shown = await mountNetwork({
      "GET /system/network": [200, steady],
      "PUT /system/network": [
        400,
        {
          error: {
            code: "invalidRequest",
            message: "the network settings; webrtcPort: address already in use",
            detail: {
              problems: [{ location: "body.webrtcPort", message: "address already in use" }],
            },
          },
        },
      ],
    });
    await settleUntil(() => saveButton(shown) !== undefined);
    await typeInto(shown, "WebRTC port", "53");
    await click(saveButton(shown));
    await settleUntil(() => shown.container.querySelector('[role="alert"]') !== null);
    expect(shown.container.querySelector('[role="alert"]')?.textContent).toBe(
      "The server cannot take that WebRTC port: give one from 1 to 65535 that nothing else on the machine uses.",
    );
  });
});
